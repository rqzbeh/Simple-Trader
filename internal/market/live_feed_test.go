package market

import (
	"context"
	"testing"
	"time"
)

func TestLiveMarketFeedOnline(t *testing.T) {
	feed := NewLiveMarketFeed(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	quotes, err := feed.FetchAllLiveTicks(ctx)
	if err != nil {
		t.Skipf("Live exchange API call skipped (might be restricted or timed out from runner): %v", err)
		return
	}

	if len(quotes) == 0 {
		t.Fatalf("expected at least 1 live quote, got 0")
	}

	for _, q := range quotes {
		if q.Price <= 0 {
			t.Errorf("expected positive price for %s, got %f", q.Symbol, q.Price)
		}
		t.Logf("Live tick from exchange: %s = $%.4f (24h change: %.2f%%)", q.Symbol, q.Price, q.Change24h)
	}
}
