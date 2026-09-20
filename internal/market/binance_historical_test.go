package market_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestBinanceHistoricalDownloaderMockServer(t *testing.T) {
	mockData := `[
		[1710000000000,"65000.0","65500.0","64800.0","65200.0","120.5",1710003599999,"7850000.0",1540,"65.2","4250000.0","0"],
		[1710003600000,"65200.0","65800.0","65100.0","65700.0","180.2",1710007199999,"11800000.0",2100,"95.4","6250000.0","0"]
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockData))
	}))
	defer server.Close()

	downloader := market.NewBinanceHistoricalDownloaderWithBaseURL(server.URL)
	candles, err := downloader.FetchHistoricalKlines(context.Background(), "BTCUSDT", "1h", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}

	if candles[0].Open != 65000.0 || candles[0].Close != 65200.0 {
		t.Errorf("candle 0 price mismatch: open=%f, close=%f", candles[0].Open, candles[0].Close)
	}
	if candles[1].High != 65800.0 || candles[1].Volume != 180.2 {
		t.Errorf("candle 1 price mismatch: high=%f, vol=%f", candles[1].High, candles[1].Volume)
	}
}

func TestBinanceHistoricalDownloaderLiveSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	downloader := market.NewBinanceHistoricalDownloader()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pull 5 authentic historical candles for BTCUSDT
	candles, err := downloader.FetchHistoricalKlines(ctx, "BTC/USDT", "1h", 5)
	if err != nil {
		t.Skipf("Live Binance network call skipped (might be restricted or timed out): %v", err)
		return
	}

	if len(candles) == 0 {
		t.Fatalf("expected at least 1 candle from live Binance")
	}

	for _, c := range candles {
		if c.Close <= 0 || c.Open <= 0 {
			t.Errorf("invalid candle prices: %+v", c)
		}
	}
}
