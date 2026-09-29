package trader_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// runTargetTape drives the full entry pipeline with a direction-correct
// market tape through the real mocked Jev batch.
func runTargetTape(t *testing.T, answers map[string]interface{}, natr, price float64, dir string) (*db.FuturesTradeSignal, *ai.DecisionResponse) {
	t.Helper()
	merged := map[string]interface{}{
		"timeframe": map[string]interface{}{
			"type": "choice", "choice": "1h", "confidence": 0.9,
			"probabilities": map[string]float64{"15m": 0.05, "1h": 0.9, "4h": 0.05},
		},
	}
	for k, v := range answers {
		merged[k] = v
	}
	srv := mockJevServerWithBatch(merged)
	t.Cleanup(srv.Close)
	router := &trader.DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "k", time.Second),
		Threshold: 0.70,
	}
	svc := trader.NewSignalService(nil, nil)
	svc.SetDecisionRouter(router)
	svc.SetNewsClassifier(stubClassifier())
	quote := cache.TickerQuote{Symbol: "BTC/USD", Price: price}
	snap := cache.IndicatorSnapshot{
		Symbol: "BTC/USD", RSI: 50, SuperTrend: "BULL", VWAP: price * 0.99,
		VolumeRatio: 3.0, ConfluenceScore: 0.41, NATR: natr,
	}
	if dir == "SHORT" {
		snap.SuperTrend = "BEAR"
		snap.VWAP = price * 1.01
	}
	sig, resp, err := svc.EvaluateMarketSignal(
		context.Background(), "BTC/USD", "ALPHA", quote, snap, nil,
		[]string{"BlackRock ETF inflows hit weekly record"}, 100000.0, 40000.0, nil,
	)
	if err != nil {
		t.Fatalf("EvaluateMarketSignal: %v", err)
	}
	return sig, resp
}

func runTarget(t *testing.T, answers map[string]interface{}, natr, price float64) (*db.FuturesTradeSignal, *ai.DecisionResponse) {
	t.Helper()
	return runTargetTape(t, answers, natr, price, "LONG")
}

// TestAtrUnitsFromNatrFraction: snapshot.NATR is a fraction (ATR/Price) —
// absolute ATR = NATR × price. The old ÷100 undersized targets 100×
// (TP2 landed at entry — spec-020 FR-701).
func TestAtrUnitsFromNatrFraction(t *testing.T) {
	sig, _ := runTarget(t, nil, 0.007, 64000.0)
	if sig == nil {
		t.Fatalf("expected signal")
	}
	if sig.ATRAtEntry == nil {
		t.Fatalf("ATRAtEntry missing")
	}
	want := 0.007 * 64000.0 // 448
	if diff := *sig.ATRAtEntry - want; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("ATRAtEntry = %v, want %v (100× unit defect if ~4.48)", *sig.ATRAtEntry, want)
	}
}

// TestStagedTargetsRegimeOrdering: with Jev picking WIDE, the override must
// keep TP1 strictly below TP2 (regime tp_atr_mult describes the runner).
func TestStagedTargetsRegimeOrdering(t *testing.T) {
	answers := map[string]interface{}{
		"atr_regime": map[string]interface{}{
			"type": "choice", "choice": "WIDE",
			"probabilities": map[string]float64{"WIDE": 1.0},
		},
	}
	sig, resp := runTarget(t, answers, 0.004, 64000.0)
	if sig == nil {
		t.Fatalf("expected signal; gate=%s detail=%v", resp.GateRejected, resp.GateRejectedDetail)
	}
	if sig.TakeProfit2 == nil {
		t.Fatalf("TP2 must exist for this sizing")
	}
	if *sig.TakeProfit2 <= sig.TakeProfit1 {
		t.Errorf("TP2 (%v) must be strictly above TP1 (%v) for LONG", *sig.TakeProfit2, sig.TakeProfit1)
	}
}

// TestTakeProfitBoundsClamp: MIN/MAX_TAKE_PROFIT_PCT finally enforced
// (FR-703); runner omitted (nil) when unreachable inside the cap (FR-704);
// bounded target below min_rr rejects explicitly instead of lying.
func TestTakeProfitBoundsClamp(t *testing.T) {
	// Extreme volatility: TP2 wants +20%, cap is 8% → clamped, ordering kept.
	sig, resp := runTarget(t, nil, 0.05, 64000.0)
	if sig == nil {
		t.Fatalf("expected signal (rr 8/2.5=3.2 >= managed min_rr ~2.7): %+v", resp)
	}
	hi := 64000.0 * 1.08
	if *sig.TakeProfit2 > hi+1e-6 || sig.TakeProfit1 > hi+1e-6 {
		t.Errorf("TP1=%v TP2=%v exceed MAX_TAKE_PROFIT_PCT cap %v", sig.TakeProfit1, *sig.TakeProfit2, hi)
	}
	if sig.TakeProfit2 != nil && *sig.TakeProfit2 < sig.TakeProfit1 {
		t.Errorf("post-clamp TP2 (%v) below TP1 (%v)", *sig.TakeProfit2, sig.TakeProfit1)
	}
	var clamps map[string]interface{}
	if len(sig.ParameterClamps) > 0 {
		if err := json.Unmarshal(sig.ParameterClamps, &clamps); err != nil {
			t.Fatalf("clamps json: %v", err)
		}
		if _, ok := clamps["take_profit_2"]; !ok {
			if _, omit := clamps["take_profit_2_omitted"]; !omit {
				t.Errorf("expected take_profit_2 clamp record, got %v", clamps)
			}
		}
	}

	// Bounded target cannot meet min_rr (Jev demands Strong RR 4.5, cap gives 3.2) → explicit gate.
	answers := map[string]interface{}{
		"min_rr_accept": map[string]interface{}{
			"type": "score", "score": 3.0, "confidence": 0.9,
			"probabilities": map[string]float64{"3": 1.0},
			"legend":        map[string]string{"0": "Reject <1.5", "1": "Marginal 1.5-2.5", "2": "Accept 2.5-4", "3": "Strong >4"},
		},
	}
	sig2, resp2 := runTarget(t, answers, 0.5, 64000.0)
	if sig2 != nil {
		t.Fatalf("bounded TP must reject when below min_rr, got signal TP1=%v", sig2.TakeProfit1)
	}
	if resp2 == nil || !strings.Contains(resp2.GateRejected, "take_profit_bound") {
		t.Errorf("expected take_profit_bound rejection, got %+v", resp2)
	}
}
