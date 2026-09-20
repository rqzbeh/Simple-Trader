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
