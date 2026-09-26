package ai_test

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

func TestThompsonSamplingWeights(t *testing.T) {
	sampler := ai.NewThompsonSampler(42)

	weights := sampler.SampleWeights()

	indicators := []string{"RSI", "MACD", "SUPERTREND", "MICROSTRUCTURE"}
	for _, ind := range indicators {
		w, ok := weights[ind]
		if !ok {
			t.Fatalf("expected weight for %s", ind)
		}
		// Verification according to FR-006: clamped strictly in [0.20, 3.00x]
		if w < 0.20 || w > 3.00 {
			t.Errorf("indicator %s weight %f out of bounds [0.20, 3.00]", ind, w)
		}
	}

	// Record 20 consecutive wins for SUPERTREND
	for i := 0; i < 20; i++ {
		sampler.RecordOutcome(ai.TradeOutcome{
			HasSnapshot:     true,
			Side:            "BUY",
			SuperTrendTrend: "BULL",
			Pnl:             150.0,
		})
	}

	stats := sampler.GetPosteriorStats()
	stStats := stats["SUPERTREND"]
	if stStats["alpha"] <= 2.0 {
		t.Errorf("expected alpha to increase after wins, got %f", stStats["alpha"])
	}
	if stStats["mean"] <= 0.50 {
		t.Errorf("expected posterior expected value > 0.50 after wins, got %f", stStats["mean"])
	}
}

// --- Spec 012 US7: outcome attribution (FR-022/023) ---

func statsOf(t *testing.T, s *ai.ThompsonSampler, name string) map[string]float64 {
	t.Helper()
	all := s.GetPosteriorStats()
	st, ok := all[name]
	if !ok {
		t.Fatalf("no posterior for %s", name)
	}
	return st
}

// A winning LONG whose snapshot was bullish/aligned must lift every aligned
// indicator's alpha and leave unaligned ones untouched.
func TestRecordOutcomeAlignedWinUpdatesOnlyAligned(t *testing.T) {
	s := ai.NewThompsonSampler(7)
	before := map[string]map[string]float64{}
	for _, n := range []string{"SUPERTREND", "MACD", "RSI", "MICROSTRUCTURE", "CMF", "KER"} {
		st := statsOf(t, s, n)
		before[n] = map[string]float64{"alpha": st["alpha"], "beta": st["beta"]}
	}

	s.RecordOutcome(ai.TradeOutcome{
		HasSnapshot:     true,
		Side:            "BUY",
		Pnl:             120,
		SuperTrendTrend: "BULL",
		RSI:             62,
		MACDHistogram:   0.4,
		CMF:             0.15,
		KaufmanER:       0.55,
		OBI:             0.30, // bullish flow, agrees with BUY
		Divergence:      "NONE",
	})

	// All six were aligned for this long entry.
	for _, n := range []string{"SUPERTREND", "MACD", "RSI", "MICROSTRUCTURE", "CMF", "KER"} {
		after := statsOf(t, s, n)
		if after["alpha"] <= before[n]["alpha"] {
			t.Errorf("%s alpha = %v, want > %v (aligned win must add evidence)", n, after["alpha"], before[n]["alpha"])
		}
		if after["beta"] != before[n]["beta"] {
			t.Errorf("%s beta = %v, want unchanged %v", n, after["beta"], before[n]["beta"])
		}
	}
}

// A losing SHORT whose snapshot agreed with the entry must add beta
// (negative evidence) to the aligned indicators only.
func TestRecordOutcomeAlignedLossAddsBeta(t *testing.T) {
	s := ai.NewThompsonSampler(7)
	before := statsOf(t, s, "SUPERTREND")["beta"]

	s.RecordOutcome(ai.TradeOutcome{
		HasSnapshot:     true,
		Side:            "SELL",
		Pnl:             -80,
		SuperTrendTrend: "BEAR", // aligned with SELL
		RSI:             40,     // aligned (<=50)
		MACDHistogram:   -0.2,   // aligned
		CMF:             -0.1,   // aligned
		KaufmanER:       0.50,
		OBI:             -0.25, // bearish flow, agrees with SELL
	})

	if got := statsOf(t, s, "SUPERTREND")["beta"]; got <= before {
		t.Errorf("SUPERTREND beta = %v, want > %v (aligned loss must add negative evidence)", got, before)
	}
	// Unaligned for this trade: SuperTrend said BULL against a SELL entry →
	// no update at all.
	bullAgainstSell := ai.TradeOutcome{
		HasSnapshot:     true,
		Side:            "SELL",
		Pnl:             -50,
		SuperTrendTrend: "BULL", // opposed the entry
		RSI:             60,     // opposed too
		MACDHistogram:   0.3,    // opposed
		CMF:             0.2,    // opposed
		KaufmanER:       0.10,   // below threshold
		OBI:             0.40,   // opposed (bullish vs SELL)
	}
	stBefore := statsOf(t, s, "SUPERTREND")
	s.RecordOutcome(bullAgainstSell)
	stAfter := statsOf(t, s, "SUPERTREND")
	if stAfter["alpha"] != stBefore["alpha"] || stAfter["beta"] != stBefore["beta"] {
		t.Errorf("unaligned outcome moved SUPERTREND posterior: %+v -> %+v", stBefore, stAfter)
	}
}

// Regression: outcomes without a recorded snapshot (legacy rows) used to
// update MICROSTRUCTURE unconditionally and RSI on any SELL. FR-023:
// unknown data must move nothing.
func TestRecordOutcomeMissingSnapshotUpdatesNothing(t *testing.T) {
	s := ai.NewThompsonSampler(7)
	before := s.GetPosteriorStats()

	s.RecordOutcome(ai.TradeOutcome{
		HasSnapshot: false,
		Side:        "SELL",
		Pnl:         -100,
		// zero-valued indicator fields, exactly what call sites used to send
	})

	after := s.GetPosteriorStats()
	for name, st := range before {
		if st["alpha"] != after[name]["alpha"] || st["beta"] != after[name]["beta"] {
			t.Errorf("%s posterior moved without a snapshot: %+v -> %+v", name, st, after[name])
		}
	}
}

// Microstructure must be direction-aware: bullish flow counts for BUY only.
func TestRecordOutcomeMicrostructureDirectionAware(t *testing.T) {
	s := ai.NewThompsonSampler(7)
	before := statsOf(t, s, "MICROSTRUCTURE")["alpha"]

	// BUY against bearish flow → no update.
	s.RecordOutcome(ai.TradeOutcome{
		HasSnapshot: true,
		Side:        "BUY",
		Pnl:         100,
		OBI:         -0.5, // bearish, disagrees
	})
	if got := statsOf(t, s, "MICROSTRUCTURE")["alpha"]; got != before {
		t.Errorf("bearish flow updated on BUY: alpha %v -> %v", before, got)
	}

	// BUY with bullish flow → update.
	s.RecordOutcome(ai.TradeOutcome{
		HasSnapshot: true,
		Side:        "BUY",
		Pnl:         100,
		OBI:         0.5,
	})
	if got := statsOf(t, s, "MICROSTRUCTURE")["alpha"]; got <= before {
		t.Errorf("bullish flow did not update on BUY: alpha %v -> %v", before, got)
	}
}

// FR-024: operator slider targets must be exactly what GetWeights serves.
func TestSetWeightsRoundTrip(t *testing.T) {
	s := ai.NewThompsonSampler(7)
	targets := map[string]float64{
		"RSI":            1.45,
		"MACD":           0.55,
		"SUPERTREND":     2.10,
		"MICROSTRUCTURE": 1.00,
		"CMF":            0.20,
		"KER":            3.00,
	}
	s.SetWeights(targets)
	got := s.GetWeights()
	for name, want := range targets {
		if diff := got[name] - want; diff > 0.001 || diff < -0.001 {
			t.Errorf("%s served weight = %v, want %v", name, got[name], want)
		}
	}

	// Out-of-range targets clamp to the FR-006 band.
	s.SetWeights(map[string]float64{"RSI": 99, "MACD": -5})
	got = s.GetWeights()
	if got["RSI"] != 3.0 {
		t.Errorf("RSI = %v, want clamp to 3.0", got["RSI"])
	}
	if got["MACD"] != 0.2 {
		t.Errorf("MACD = %v, want clamp to 0.2", got["MACD"])
	}
}
