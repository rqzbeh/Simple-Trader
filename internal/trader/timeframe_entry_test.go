package trader_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func mockJevServerWithBatch(answers map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fullAnswers := map[string]interface{}{
			// Docs-shaped Score answers: probability-weighted LEVEL INDEX via
			// probabilities keyed by level index (docs.typesafe.ai/api).
			"min_rr_accept": map[string]interface{}{"type": "score", "score": 2.3, "confidence": 0.8,
				"probabilities": map[string]float64{"1": 0.3, "2": 0.7},
				"legend":        map[string]string{"0": "Reject <1.5", "1": "Marginal 1.5-2.5", "2": "Accept 2.5-4", "3": "Strong >4"}},
			"leverage": map[string]interface{}{"type": "choice", "choice": "8x", "probabilities": map[string]float64{"8x": 1.0}},
			"conviction": map[string]interface{}{"type": "score", "score": 1.7, "confidence": 0.75,
				"probabilities": map[string]float64{"1": 0.3, "2": 0.7},
				"legend":        map[string]string{"0": "Weak", "1": "Moderate", "2": "High", "3": "Very high"}},
			"atr_regime": map[string]interface{}{"type": "choice", "choice": "NORMAL", "probabilities": map[string]float64{"NORMAL": 1.0}},
			"confluence": map[string]interface{}{"type": "score", "score": 1.5, "confidence": 0.7,
				"probabilities": map[string]float64{"1": 0.5, "2": 0.5},
				"legend":        map[string]string{"0": "<0.25", "1": "0.25-0.35", "2": "0.35-0.50", "3": ">0.50"}},
			"decay": map[string]interface{}{"type": "choice", "choice": "FAST_BREAKING", "probabilities": map[string]float64{"FAST_BREAKING": 1.0}},
		}
		for k, v := range answers {
			fullAnswers[k] = v
		}
		// Answer ONLY the questions actually asked (mirrors the real API).
		// Pre-spec-015 this mock answered every key blindly, which masked the
		// missing-decay defect: production batches never request "decay".
		var req struct {
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && len(req.Questions) > 0 {
			for k := range fullAnswers {
				if _, asked := req.Questions[k]; !asked {
					delete(fullAnswers, k)
				}
			}
		}
		resp := map[string]interface{}{
			"model":   "jev-test",
			"answers": fullAnswers,
			"usage":   map[string]int{"input_tokens": 100, "output_tokens": 10},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestEntryPath_TimeframeValid(t *testing.T) {
	ctx := context.Background()

	answers := map[string]interface{}{
		"entry": map[string]interface{}{
			"type":       "choice",
			"choice":     "LONG",
			"confidence": 0.90,
			"probabilities": map[string]float64{
				"LONG": 0.90, "SHORT": 0.05, "NO_TRADE": 0.05,
			},
		},
		"timeframe": map[string]interface{}{
			"type":       "choice",
			"choice":     "1h",
			"confidence": 0.88,
			"probabilities": map[string]float64{
				"15m": 0.10, "1h": 0.80, "4h": 0.10,
			},
		},
	}
	srv := mockJevServerWithBatch(answers)
	defer srv.Close()

	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "test-key", time.Second),
		Threshold: 0.70,
	}

	service := trader.NewSignalService(nil, nil)
	service.SetDecisionRouter(router)
	service.SetNewsClassifier(stubClassifier())

	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: 60000.0}
	snap := cache.IndicatorSnapshot{Symbol: "BTC/USD", RSI: 55.0, SuperTrend: "BULL"}
	headlines := []string{"Bullish catalyst breaking news"}

	sig, resp, err := service.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, headlines, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatalf("expected active signal, got nil")
	}

	// Verify timeframe persisted on signal
	if sig.Timeframe == nil || *sig.Timeframe != "1h" {
		t.Errorf("expected signal timeframe '1h', got %v", sig.Timeframe)
	}
	if sig.TimeframeConfidence == nil || *sig.TimeframeConfidence != 0.88 {
		t.Errorf("expected signal timeframe confidence 0.88, got %v", sig.TimeframeConfidence)
	}
	if len(sig.TimeframeDistribution) == 0 {
		t.Errorf("expected signal timeframe distribution JSON to be present")
	}

	// Verify horizon from profile map: 1h profile is 120m, not static 60m
	tfProf, ok := config.GetTimeframeProfile("1h")
	if !ok {
		t.Fatalf("timeframe profile for 1h not found")
	}
	if tfProf.HorizonMin != 120 {
		t.Errorf("expected 1h profile horizon 120m, got %d", tfProf.HorizonMin)
	}

	// Effective profile for signal must match 1h profile horizon (120m, not 60m static)
	effProf, err := trader.EffectiveProfileWithTimeframe("CRYPTO", *sig.Timeframe)
	if err != nil {
		t.Fatalf("unexpected error resolving effective profile: %v", err)
	}
	if effProf.HorizonMin != 120 {
		t.Errorf("expected signal effective horizon 120, got %d (static 60 was not overridden)", effProf.HorizonMin)
	}
	if effProf.DecayBreakevenAtMin != 30 {
		t.Errorf("expected decay breakeven 30, got %d", effProf.DecayBreakevenAtMin)
	}
	if effProf.DecayFlatAtMin != 60 {
		t.Errorf("expected decay flat 60, got %d", effProf.DecayFlatAtMin)
	}
	_ = resp
}

func TestEntryPath_TimeframeInvalid_AbortsLoud(t *testing.T) {
	ctx := context.Background()

	// "2h" is not in ALPHA set {15m, 1h, 4h}
	answers := map[string]interface{}{
		"entry": map[string]interface{}{
			"type":       "choice",
			"choice":     "LONG",
			"confidence": 0.90,
			"probabilities": map[string]float64{
				"LONG": 0.90, "SHORT": 0.05, "NO_TRADE": 0.05,
			},
		},
		"timeframe": map[string]interface{}{
			"type":       "choice",
			"choice":     "2h",
			"confidence": 0.85,
			"probabilities": map[string]float64{
				"15m": 0.10, "1h": 0.80, "4h": 0.10,
			},
		},
	}
	srv := mockJevServerWithBatch(answers)
	defer srv.Close()

	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "test-key", time.Second),
		Threshold: 0.70,
	}

	service := trader.NewSignalService(nil, nil)
	service.SetDecisionRouter(router)
	service.SetNewsClassifier(stubClassifier())

	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: 60000.0}
	snap := cache.IndicatorSnapshot{Symbol: "BTC/USD", RSI: 55.0, SuperTrend: "BULL"}
	headlines := []string{"Bullish catalyst breaking news"}

	sig, _, err := service.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, headlines, 100000.0, 40000.0,
	)
	if err == nil {
		t.Fatalf("expected error for invalid timeframe choice, got nil")
	}
	if sig != nil {
		t.Fatalf("expected no signal row to be produced, got %+v", sig)
	}

	var decErr *ai.DecisionError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *ai.DecisionError, got %T: %v", err, err)
	}
	if decErr.Component != "timeframe" {
		t.Errorf("expected component='timeframe', got %q", decErr.Component)
	}
	if !errors.Is(err, ai.ErrJevSchema) {
		t.Errorf("expected ErrJevSchema, got %v", err)
	}
}

func TestEntryPath_TimeframeMissing_AbortsLoud(t *testing.T) {
	ctx := context.Background()

	// Missing timeframe answer
	answers := map[string]interface{}{
		"entry": map[string]interface{}{
			"type":       "choice",
			"choice":     "LONG",
			"confidence": 0.90,
			"probabilities": map[string]float64{
				"LONG": 0.90, "SHORT": 0.05, "NO_TRADE": 0.05,
			},
		},
	}
	srv := mockJevServerWithBatch(answers)
	defer srv.Close()

	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "test-key", time.Second),
		Threshold: 0.70,
	}

	service := trader.NewSignalService(nil, nil)
	service.SetDecisionRouter(router)
	service.SetNewsClassifier(stubClassifier())

	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: 60000.0}
	snap := cache.IndicatorSnapshot{Symbol: "BTC/USD", RSI: 55.0, SuperTrend: "BULL"}
	headlines := []string{"Bullish catalyst breaking news"}

	sig, _, err := service.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, headlines, 100000.0, 40000.0,
	)
	if err == nil {
		t.Fatalf("expected error for missing timeframe answer, got nil")
	}
	if sig != nil {
		t.Fatalf("expected no signal row to be produced, got %+v", sig)
	}

	var decErr *ai.DecisionError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *ai.DecisionError, got %T: %v", err, err)
	}
	if decErr.Component != "timeframe" {
		t.Errorf("expected component='timeframe', got %q", decErr.Component)
	}
}

// TestEntryPath_EscalationReceivesRealPayload is the 2026-09-29 no-signal
// regression: with Jev confidence below threshold, Escalate must receive the
// FULL entry request (symbol, indicators, headlines) — the production closure
// used to send Symbol:"escalated" + empty snapshot, so 9Router answered HOLD
// for every sub-threshold candidate and no trade was ever created.
func TestEntryPath_EscalationReceivesRealPayload(t *testing.T) {
	ctx := context.Background()
	answers := map[string]interface{}{
		"entry": map[string]interface{}{
			"type": "choice", "choice": "LONG", "confidence": 0.50,
			"probabilities": map[string]float64{"LONG": 0.50, "NO_TRADE": 0.30, "SHORT": 0.20},
		},
		"timeframe": map[string]interface{}{
			"type": "choice", "choice": "1h", "confidence": 0.88,
			"probabilities": map[string]float64{"15m": 0.10, "1h": 0.80, "4h": 0.10},
		},
	}
	srv := mockJevServerWithBatch(answers)
	defer srv.Close()

	var gotPayload interface{}
	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "test-key", time.Second),
		Threshold: 0.70, // 0.50 < 0.70 ⇒ escalation path fires
		Escalate: func(ctx context.Context, payload interface{}) (trader.DecisionOutcome, error) {
			gotPayload = payload
			// Judge the real request the way 9Router would: return SHORT intent.
			return trader.DecisionOutcome{Choice: "SHORT", Confidence: 0.9}, nil
		},
	}
	service := trader.NewSignalService(nil, nil)
	service.SetDecisionRouter(router)
	service.SetNewsClassifier(stubClassifier())

	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: 60000.0}
	snap := cache.IndicatorSnapshot{Symbol: "BTC/USD", RSI: 55.0, SuperTrend: "BULL"}
	headlines := []string{"US SEC follows CFTC in staff guidance for crypto"}

	sig, resp, err := service.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, headlines, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatalf("escalated candidate must still produce a signal (defect: HOLD killed every entry)")
	}

	req, ok := gotPayload.(ai.DecisionRequest)
	if !ok {
		t.Fatalf("escalation payload type = %T, want ai.DecisionRequest (empty/dummy payload regression)", gotPayload)
	}
	if req.Symbol != "BTC/USD" {
		t.Errorf("escalation symbol = %q, want BTC/USD", req.Symbol)
	}
	if req.IndicatorSnap.RSI != 55.0 {
		t.Errorf("escalation lost indicator evidence: %+v", req.IndicatorSnap)
	}
	if len(req.NewsHeadlines) != 1 || req.NewsHeadlines[0] != headlines[0] {
		t.Errorf("escalation lost headlines: %v", req.NewsHeadlines)
	}
	if resp.Reasoning != "route=escalated" {
		t.Errorf("route = %q, want route=escalated", resp.Reasoning)
	}
	if resp.Decision != "SELL" {
		t.Errorf("escalated SHORT must map to SELL intent, got %q", resp.Decision)
	}
}
