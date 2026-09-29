//go:build liveapi

package trader

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// T016: Live smoke test for batched entry + timeframe judgment (SC-205, quickstart V7).
// Run with: go test -tags=liveapi -v ./internal/trader -run TestTimeframeLiveSmoke
func TestTimeframeLiveSmoke(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set; skipping liveapi test")
	}

	j := ai.NewJevClient("", key, 5*time.Second)
	state := map[string]interface{}{
		"symbol": "BTCUSDT",
		"note":   "live batch smoke test",
	}

	cycleID := "live-batch-smoke"
	questions := EntryQuestions("ALPHA")

	start := time.Now()
	answers, usage, err := j.Evaluate(context.Background(), cycleID, state, questions)
	batchLatency := time.Since(start)
	if err != nil {
		t.Fatalf("batch evaluate failed: %v", err)
	}

	// 1. Validate direction answer (spec-018 split questions)
	dirAns, ok := answers["direction"]
	if !ok {
		t.Fatalf("missing direction answer in batch response")
	}
	if err := ai.ValidateChoice(dirAns, directionVocab, cycleID); err != nil {
		t.Fatalf("direction validation failed: %v", err)
	}
	if _, hasEdge := answers["edge"]; !hasEdge {
		t.Fatalf("missing edge noul answer in batch response")
	}

	// 2. Validate timeframe answer
	tfAns, ok := answers["timeframe"]
	if !ok {
		t.Fatalf("missing timeframe answer in batch response")
	}
	alphaSet := config.GetBucketTimeframeSet("ALPHA")
	if err := ValidateTimeframe(tfAns, alphaSet, cycleID); err != nil {
		t.Fatalf("timeframe validation failed: %v", err)
	}

	t.Logf("batch success: entry=%s conf=%.2f, timeframe=%s conf=%.2f, latency=%v (usage latency=%v)",
		dirAns.Choice, dirAns.Confidence, tfAns.Choice, tfAns.Confidence, batchLatency, usage.Latency)
}
