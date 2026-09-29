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

// T016: Live smoke test for batched managed parameters judgment (SC-303, quickstart V7).
// Run with: go test -tags=liveapi -v ./internal/trader -run TestManagedParamsLiveSmoke
func TestManagedParamsLiveSmoke(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set; skipping liveapi test")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	j := ai.NewJevClient("", key, 12*time.Second)
	state := map[string]interface{}{
		"symbol":     "BTCUSDT",
		"confluence": 0.75,
		"atr_pct":    1.8,
		"regime":     "EXPANSION",
		"note":       "live managed params batch smoke test",
	}

	cycleID := "live-managed-params-smoke"
	questions := EntryQuestions("ALPHA")
	managedQs := BuildManagedEntryQuestions(cfg, state)
	for qk, q := range managedQs {
		questions[qk] = q
	}
	if decayQ, ok := BuildManagedNewsDecayQuestion(cfg, state); ok {
		questions["decay"] = decayQ
	}

	start := time.Now()
	answers, usage, err := j.Evaluate(context.Background(), cycleID, state, questions)
	batchLatency := time.Since(start)
	if err != nil {
		t.Fatalf("batch evaluate failed: %v", err)
	}

	reg := NewParamRegistry(cfg)
	resolved, err := reg.ResolveAll(answers, cycleID)
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	for k, res := range resolved {
		t.Logf("param %s: mode=%s value=%v clamped=%v", k, res.Mode, res.Value, res.Clamped)
	}

	t.Logf("batch success: %d questions answered, latency=%v (usage latency=%v)",
		len(answers), batchLatency, usage.Latency)
}
