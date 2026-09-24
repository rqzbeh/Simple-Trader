package market

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveFeedSmokeAllAssets(t *testing.T) {
	// Live network check: opt-in so CI stays hermetic.
	// Run with: SIMPLE_TRADER_LIVE_TEST=1 go test ./internal/market/ -run TestLiveFeedSmokeAllAssets -v
	if os.Getenv("SIMPLE_TRADER_LIVE_TEST") != "1" {
		t.Skip("set SIMPLE_TRADER_LIVE_TEST=1 to run against live exchange feeds")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	assets := GetSupportedAssets()
	f := NewLiveMarketFeed(assets)
	quotes, err := f.FetchAllLiveTicks(ctx)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	got := map[string]bool{}
	for _, q := range quotes {
		if q.Price > 0 {
			got[q.Symbol] = true
		}
	}
	var missing []string
	for _, a := range assets {
		if !got[a.Symbol] {
			missing = append(missing, a.Symbol)
		}
	}
	t.Logf("priced %d/%d assets", len(got), len(assets))
	if len(missing) > 0 {
		t.Logf("MISSING (%d): %v", len(missing), missing)
	}
	if len(missing) > len(assets)/4 {
		t.Fatalf("too many unpriced assets: %d", len(missing))
	}
}
