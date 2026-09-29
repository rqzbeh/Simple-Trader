package trader

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

// TestDirectionVocabRejectsNoTrade: NO_TRADE must be impossible inside the
// relative direction Choice — it is composed in code (spec-018 FR-502/503).
func TestDirectionVocabRejectsNoTrade(t *testing.T) {
	ans := ai.JevAnswer{
		Type: "choice", Choice: "NO_TRADE", Confidence: 0.99,
		Probabilities: map[string]float64{"NO_TRADE": 0.99, "LONG": 0.01},
	}
	if err := ai.ValidateChoice(ans, directionVocab, "vocab-check"); err == nil {
		t.Fatalf("NO_TRADE must be rejected by direction vocab")
	}
	long := ai.JevAnswer{
		Type: "choice", Choice: "LONG", Confidence: 0.8,
		Probabilities: map[string]float64{"LONG": 0.8, "SHORT": 0.2},
	}
	if err := ai.ValidateChoice(long, directionVocab, "vocab-check"); err != nil {
		t.Fatalf("LONG must be accepted: %v", err)
	}
}
