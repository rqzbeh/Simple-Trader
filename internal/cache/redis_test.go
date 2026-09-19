package cache_test

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

func TestCacheKeys(t *testing.T) {
	tickerKey := cache.TickerKey("BTC/USD")
	expectedTicker := "ticker:BTC/USD"
	if tickerKey != expectedTicker {
		t.Errorf("expected %s, got %s", expectedTicker, tickerKey)
	}

	candleKey := cache.CandleKey("ETH/USD", "1h")
	expectedCandle := "candles:ETH/USD:1h"
	if candleKey != expectedCandle {
		t.Errorf("expected %s, got %s", expectedCandle, candleKey)
	}

	weightsKey := cache.WeightsKey("SOL/USD", "VOLATILE")
	expectedWeights := "weights:SOL/USD:VOLATILE"
	if weightsKey != expectedWeights {
		t.Errorf("expected %s, got %s", expectedWeights, weightsKey)
	}
}

func TestTickerSerialization(t *testing.T) {
	orig := cache.TickerQuote{
		Symbol:    "BTC/USD",
		Price:     67450.25,
		Change24h: 3.42,
		High24h:   68100.0,
		Low24h:    65200.0,
		Volume:    18450.5,
		UpdatedAt: time.Now().Unix(),
	}

	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal ticker: %v", err)
	}

	var parsed cache.TickerQuote
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatalf("failed to unmarshal ticker: %v", err)
	}

	if parsed.Symbol != orig.Symbol || parsed.Price != orig.Price {
		t.Errorf("mismatch in serialized ticker: expected %+v, got %+v", orig, parsed)
	}
}

func TestIndicatorSnapshotSerialization(t *testing.T) {
	snap := cache.IndicatorSnapshot{
		Symbol:          "XAU/USD",
		RSI:             62.5,
		MACD:            1.8,
		Signal:          1.2,
		Histogram:       0.6,
		UpperBand:       2680.0,
		MiddleBand:      2650.0,
		LowerBand:       2620.0,
		SuperTrend:      "BULL",
		ConfluenceScore: 0.85,
		Regime:          "BULL",
		UpdatedAt:       time.Now().Unix(),
	}

	data, err := snap.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	var parsed cache.IndicatorSnapshot
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	if parsed.Symbol != snap.Symbol || parsed.SuperTrend != "BULL" {
		t.Errorf("snapshot unmarshal mismatch: %+v", parsed)
	}
}
