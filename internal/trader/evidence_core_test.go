package trader_test

import (
	"context"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func coreAnswers(directionConf float64, edge float64) map[string]interface{} {
	return map[string]interface{}{
		"direction": map[string]interface{}{
			"type": "choice", "choice": "LONG", "confidence": directionConf,
			"probabilities": map[string]float64{"LONG": directionConf, "SHORT": 1 - directionConf},
		},
		"edge": map[string]interface{}{"type": "noul", "noul": edge},
		"timeframe": map[string]interface{}{
			"type": "choice", "choice": "1h", "confidence": 0.9,
			"probabilities": map[string]float64{"15m": 0.05, "1h": 0.9, "4h": 0.05},
		},
	}
}

func runCore(t *testing.T, answers map[string]interface{}, esc trader.DecisionOutcome) *ai.DecisionResponse {
	t.Helper()
	srv := mockJevServerWithBatch(answers)
	t.Cleanup(srv.Close)
	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "k", time.Second),
		Threshold: 0.70,
		Escalate: func(ctx context.Context, payload interface{}) (trader.DecisionOutcome, error) {
			return esc, nil
		},
	}
	svc := trader.NewSignalService(nil, nil)
	svc.SetDecisionRouter(router)
	svc.SetNewsClassifier(stubClassifier())
	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: 64000}
	snap := cache.IndicatorSnapshot{Symbol: "BTC/USD", RSI: 62, SuperTrend: "BULL", VWAP: 63000, VolumeRatio: 3.0, ConfluenceScore: 0.41}
	_, resp, err := svc.EvaluateMarketSignal(
		context.Background(), "BTC/USD", "ALPHA", quote, snap, nil,
		[]string{"BlackRock ETF inflows hit weekly record"}, 100000.0, 40000.0, nil,
	)
	if err != nil {
		t.Fatalf("EvaluateMarketSignal: %v", err)
	}
	return resp
}

// TestDirectionEdgeComposition: code composes NO_TRADE from the absolute
// edge Noul; a confirmed edge honors the relative direction (FR-503).
func TestDirectionEdgeComposition(t *testing.T) {
	resp := runCore(t, coreAnswers(0.85, 0.9), trader.DecisionOutcome{Choice: "NO_TRADE", Confidence: 0.99})
	if resp == nil {
		t.Fatalf("expected decision response")
	}
	if resp.Decision != "BUY" {
		t.Errorf("edge 0.9 + direction LONG must compose BUY, got %q (route %s)", resp.Decision, resp.Reasoning)
	}

	resp2 := runCore(t, coreAnswers(0.85, 0.2), trader.DecisionOutcome{Choice: "LONG", Confidence: 0.95})
	if resp2 == nil {
		t.Fatalf("expected decision response")
	}
	if resp2.Decision != "HOLD" {
		t.Errorf("edge 0.2 must compose NO_TRADE (HOLD) regardless of direction, got %q", resp2.Decision)
	}
}

// TestTrustArgmax: spec-018 FR-504 — the higher-confidence answer wins.
// A weak escalated HOLD must not beat a stronger composed direction.
func TestTrustArgmax(t *testing.T) {
	// composed conf = min(0.60, 0.9) = 0.60 < 0.70 → escalates;
	// escalated HOLD 0.15 < 0.60 → Jev keeps LONG (escalated_jev_kept).
	resp := runCore(t, coreAnswers(0.60, 0.9), trader.DecisionOutcome{Choice: "NO_TRADE", Confidence: 0.15})
	if resp == nil {
		t.Fatalf("expected response")
	}
	if resp.Decision != "BUY" {
		t.Errorf("weak escalated HOLD (0.15) must NOT beat composed LONG (0.60), got %q route=%s", resp.Decision, resp.Reasoning)
	}
	if resp.Reasoning != "route=escalated_jev_kept" {
		t.Errorf("route = %q, want escalated_jev_kept", resp.Reasoning)
	}

	// escalated BUY 0.95 > composed 0.60 → escalation wins.
	resp2 := runCore(t, coreAnswers(0.60, 0.9), trader.DecisionOutcome{Choice: "LONG", Confidence: 0.95})
	if resp2.Reasoning != "route=escalated" {
		t.Errorf("route = %q, want escalated", resp2.Reasoning)
	}
	if resp2.Decision != "BUY" {
		t.Errorf("escalated BUY expected, got %q", resp2.Decision)
	}
}
