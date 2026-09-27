package trader

import (
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestBuildStateHappyPath(t *testing.T) {
	st, err := BuildState("BTC/USDT", "2026-09-27T00:00:00Z",
		map[string]float64{"rsi": 62, "macd": 0.4}, 1.5, 3.0,
		market.NewsSentimentReport{Polarity: market.PolarityBullish, Score: 0.6, HeadlineCount: 3},
		FeedState{Value: "rsi success 63%"}, map[string]string{"fast_ema": "passed"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Symbol != "BTC/USDT" || st.ATR["stop_loss"] != 1.5 {
		t.Fatalf("bad state: %+v", st)
	}
}

func TestBuildStateExplicitErrors(t *testing.T) {
	if _, err := BuildState("", "t", map[string]float64{"rsi": 1}, 1, 2, market.NewsSentimentReport{}, FeedState{}, nil); err == nil || !strings.Contains(err.Error(), "component=") {
		t.Fatalf("missing symbol must error explicitly: %v", err)
	}
	if _, err := BuildState("BTC", "t", nil, 1, 2, market.NewsSentimentReport{}, FeedState{}, nil); err == nil || !strings.Contains(err.Error(), "missing indicator") {
		t.Fatalf("empty indicators must error: %v", err)
	}
	if _, err := BuildState("BTC", "t", map[string]float64{"rsi": 1}, 0, 0, market.NewsSentimentReport{}, FeedState{}, nil); err == nil || !strings.Contains(err.Error(), "ATR") {
		t.Fatalf("missing ATR must error: %v", err)
	}
}
