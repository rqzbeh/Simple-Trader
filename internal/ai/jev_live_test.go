//go:build liveapi

package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

// T032: live smoke — run: go test -tags=liveapi ./internal/ai/ -run TestJevLive
func TestJevLive(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Fatal("TYPESAFE_API_KEY not set")
	}
	j := NewJevClient("", key, 5*time.Second)
	answers, usage, err := j.Evaluate(context.Background(), "live-smoke",
		map[string]interface{}{"symbol": "BTCUSDT", "note": "test state"},
		map[string]JevQuestion{"entry": {Type: "choice",
			Instructions: "Position intent",
			Criteria:     map[string]string{"LONG": "up", "SHORT": "down", "NO_TRADE": "no edge"}}})
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"LONG": true, "SHORT": true, "NO_TRADE": true}
	if err := ValidateChoice(answers["entry"], allowed, "live-smoke"); err != nil {
		t.Fatal(err)
	}
	if usage.Latency > time.Second {
		t.Fatalf("latency %v > 1s", usage.Latency)
	}
	t.Logf("model=%s choice=%s conf=%.2f latency=%v", usage.Model, answers["entry"].Choice, answers["entry"].Confidence, usage.Latency)
}
