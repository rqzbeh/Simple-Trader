package trader_test

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestSignalContract_Invariants (spec-020 US-C1) is the property suite that
// would have caught the live TP2-below-TP1 bug: across every regime,
// volatility, direction and R:R demand, an emitted signal MUST satisfy the
// full contract — bounded targets, staged ordering, bounded stop, honest
// R:R, sane leverage, correct ATR units, valid clamp records. Rejections are
// allowed ONLY as explicit take_profit_bound gates.
func TestSignalContract_Invariants(t *testing.T) {
	regimes := map[string]interface{}{
		"TIGHT":  map[string]interface{}{"type": "choice", "choice": "TIGHT", "probabilities": map[string]float64{"TIGHT": 1.0}},
		"NORMAL": map[string]interface{}{"type": "choice", "choice": "NORMAL", "probabilities": map[string]float64{"NORMAL": 1.0}},
		"WIDE":   map[string]interface{}{"type": "choice", "choice": "WIDE", "probabilities": map[string]float64{"WIDE": 1.0}},
	}
	minRRs := map[string]map[string]interface{}{
		"2.0": {"min_rr_accept": map[string]interface{}{"type": "score", "score": 1.0,
			"probabilities": map[string]float64{"1": 1.0},
			"legend":        map[string]string{"0": "Reject <1.5", "1": "Marginal 1.5-2.5", "2": "Accept 2.5-4", "3": "Strong >4"}}},
		"4.5": {"min_rr_accept": map[string]interface{}{"type": "score", "score": 3.0,
			"probabilities": map[string]float64{"3": 1.0},
			"legend":        map[string]string{"0": "Reject <1.5", "1": "Marginal 1.5-2.5", "2": "Accept 2.5-4", "3": "Strong >4"}}},
	}
	minStop, maxStop := 0.6, 2.5 // mirrors SampleSignalConfig (.env samples)
	minTP, maxTP := 1.5, 8.0
	price := 64000.0

	for _, dir := range []string{"LONG", "SHORT"} {
		for regimeName, regimeAns := range regimes {
			for _, natr := range []float64{0.001, 0.004, 0.05, 0.5} {
				for rrName, rrAns := range minRRs {
					name := fmt.Sprintf("%s/%s/natr%v/rr%s", dir, regimeName, natr, rrName)
					t.Run(name, func(t *testing.T) {
						answers := map[string]interface{}{
							"direction": map[string]interface{}{
								"type": "choice", "choice": dir, "confidence": 0.9,
								"probabilities": probPair(dir),
							},
							"atr_regime": regimeAns,
						}
						for k, v := range rrAns {
							answers[k] = v
						}
						sig, resp := runTargetTape(t, answers, natr, price, dir)
						if sig == nil {
							if resp == nil || resp.GateRejected != "take_profit_bound" {
								t.Fatalf("nil signal must be an explicit take_profit_bound rejection, got gate=%q", resp.GateRejected)
							}
							return
						}
						entry := sig.EntryPrice
						if sig.ATRAtEntry == nil {
							t.Fatalf("ATRAtEntry missing")
						}
						if d := *sig.ATRAtEntry - natr*price; d > 1e-6 || d < -1e-6 {
							t.Errorf("ATRAtEntry=%v want %v (100x unit defect)", *sig.ATRAtEntry, natr*price)
						}
						var lo, hi float64
						if dir == "LONG" {
							lo, hi = entry*(1+minTP/100), entry*(1+maxTP/100)
						} else {
							lo, hi = entry*(1-maxTP/100), entry*(1-minTP/100)
						}
						if sig.TakeProfit1 < lo-1e-6 || sig.TakeProfit1 > hi+1e-6 {
							t.Errorf("TP1=%v outside [%v,%v]", sig.TakeProfit1, lo, hi)
						}
						if sig.TakeProfit2 != nil {
							if dir == "LONG" && *sig.TakeProfit2 < sig.TakeProfit1 {
								t.Errorf("TP2=%v below TP1=%v (LONG)", *sig.TakeProfit2, sig.TakeProfit1)
							}
							if dir == "SHORT" && *sig.TakeProfit2 > sig.TakeProfit1 {
								t.Errorf("TP2=%v above TP1=%v (SHORT)", *sig.TakeProfit2, sig.TakeProfit1)
							}
							if *sig.TakeProfit2 < lo-1e-6 || *sig.TakeProfit2 > hi+1e-6 {
								t.Errorf("TP2=%v outside [%v,%v]", *sig.TakeProfit2, lo, hi)
							}
						}
						var slPct float64
						if dir == "LONG" {
							slPct = (entry - sig.StopLoss) / entry * 100
						} else {
							slPct = (sig.StopLoss - entry) / entry * 100
						}
						if slPct < minStop-1e-6 || slPct > maxStop+1e-6 {
							t.Errorf("%s stop pct=%v outside env bounds [%v,%v]", dir, slPct, minStop, maxStop)
						}
						risk, reward := 0.0, 0.0
						if dir == "LONG" {
							risk, reward = entry-sig.StopLoss, sig.TakeProfit1-entry
						} else {
							risk, reward = sig.StopLoss-entry, entry-sig.TakeProfit1
						}
						wantMinRR := map[string]float64{"2.0": 2.0, "4.5": 4.5}[rrName]
						if risk <= 0 || reward <= 0 {
							t.Fatalf("non-positive risk/reward: risk=%v reward=%v", risk, reward)
						}
						if got := reward / risk; got+1e-6 < wantMinRR {
							t.Errorf("rr=%v below demanded min_rr=%v", got, wantMinRR)
						}
						if sig.Leverage < 1 {
							t.Errorf("leverage=%d < 1", sig.Leverage)
						}
						if len(sig.ParameterClamps) > 0 {
							var m map[string]interface{}
							if err := json.Unmarshal(sig.ParameterClamps, &m); err != nil {
								t.Errorf("parameter_clamps invalid JSON: %v", err)
							}
						}
					})
				}
			}
		}
	}
}

func probPair(dir string) map[string]float64 {
	if dir == "LONG" {
		return map[string]float64{"LONG": 0.9, "SHORT": 0.1}
	}
	return map[string]float64{"LONG": 0.1, "SHORT": 0.9}
}
