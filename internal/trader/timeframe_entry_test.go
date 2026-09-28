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
		resp := map[string]interface{}{
			"model":   "jev-test",
			"answers": answers,
			"usage":   map[string]int{"input_tokens": 100, "output_tokens": 10},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestEntryPath_TimeframeValid(t *testing.T) {
	ctx := context.Background()

	answers := map[string]interface{}{
		"entry": map[string]interface{}{
			"type":   "choice",
			"choice": "LONG",
			"confidence": 0.90,
			"probabilities": map[string]float64{
				"LONG": 0.90, "SHORT": 0.05, "NO_TRADE": 0.05,
			},
		},
		"timeframe": map[string]interface{}{
			"type":   "choice",
			"choice": "1h",
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
			"type":   "choice",
			"choice": "LONG",
			"confidence": 0.90,
			"probabilities": map[string]float64{
				"LONG": 0.90, "SHORT": 0.05, "NO_TRADE": 0.05,
			},
		},
		"timeframe": map[string]interface{}{
			"type":   "choice",
			"choice": "2h",
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
			"type":   "choice",
			"choice": "LONG",
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
