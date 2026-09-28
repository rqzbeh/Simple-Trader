//go:build liveapi

package trader

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// T017: live smoke — run: go test -tags=liveapi -v ./internal/trader/ -run TestEarlyExitLiveSmoke
func TestEarlyExitLiveSmoke(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set; skipping liveapi test")
	}

	j := ai.NewJevClient("", key, 5*time.Second)
	now := time.Now()

	pos := &db.Trade{
		ID:         777,
		Symbol:     "BTC/USDT",
		Side:       "BUY",
		EntryPrice: 65000.0,
		StopLoss:   63000.0,
		TakeProfit: 69000.0,
		EntryTime:  now.Add(-45 * time.Minute),
		Status:     "OPEN",
	}

	cluster := &market.NewsCluster{
		ID:        "9999",
		Headline:  "SEC files emergency enforcement action against major exchange",
		StoryCount: 8,
		FirstSeen: now.Add(-10 * time.Minute),
	}

	questions := BuildEarlyExitQuestions([]*db.Trade{pos}, cluster, now)
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cycleID := "live-smoke-early-exit-1"
	answers, usage, err := j.Evaluate(ctx, cycleID, map[string]interface{}{
		"symbol": "BTC/USDT",
		"note":   "live smoke early exit validation",
	}, questions)
	if err != nil {
		t.Fatalf("Jev evaluation failed: %v", err)
	}

	ans, ok := answers["close_now:777"]
	if !ok {
		t.Fatalf("missing answer for close_now:777")
	}

	if ans.Type != "noul" {
		t.Fatalf("expected answer type noul, got %s", ans.Type)
	}
	if ans.Noul == nil {
		t.Fatalf("expected non-nil noul probability")
	}
	if *ans.Noul < 0 || *ans.Noul > 1 {
		t.Fatalf("expected noul in [0,1], got %f", *ans.Noul)
	}

	t.Logf("model=%s noul=%.3f conf=%.3f latency=%v", usage.Model, *ans.Noul, ans.Confidence, usage.Latency)
}
