package market

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKuCoinFetcher(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/market/orderbook/level1":
			// Spot KuCoin level 1 response
			w.Write([]byte(`{
				"code": "200000",
				"data": {
					"time": 1710000000000,
					"sequence": "123456",
					"price": "67500.50",
					"size": "0.15",
					"bestBid": "67500.00",
					"bestBidSize": "1.2",
					"bestAsk": "67501.00",
					"bestAskSize": "0.8"
				}
			}`))
		case "/api/v1/ticker":
			// Futures KuCoin ticker response
			w.Write([]byte(`{
				"code": "200000",
				"data": {
					"sequence": 1234567,
					"symbol": "COPPERUSDTM",
					"price": "6.85",
					"size": 10,
					"bestBidPrice": "6.84",
					"bestBidSize": 50,
					"bestAskPrice": "6.86",
					"bestAskSize": 45
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fetcher := NewKuCoinFetcherWithBaseURLs(ts.URL, ts.URL)

	t.Run("Fetch Spot Ticker for BTC/USDT", func(t *testing.T) {
		quote, err := fetcher.FetchTicker(context.Background(), "BTC/USDT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quote.Price != 67500.50 {
			t.Errorf("expected price 67500.50, got %f", quote.Price)
		}
	})

	t.Run("Get24hStats for BTC/USDT", func(t *testing.T) {
		price, _, spreadBps, err := fetcher.Get24hStats("BTC/USDT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if price != 67500.50 {
			t.Errorf("expected price 67500.50, got %f", price)
		}
		if spreadBps <= 0 {
			t.Errorf("expected positive spread bps, got %f", spreadBps)
		}
	})
}

func TestCoinExFetcher(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/spot/ticker":
			w.Write([]byte(`{
				"code": 0,
				"message": "OK",
				"data": [
					{
						"market": "BTCUSDT",
						"last": "68100.25",
						"open": "67000.00",
						"close": "68100.25",
						"high": "68500.00",
						"low": "66800.00",
						"volume": "1542.5",
						"volume_sell": "700.0",
						"volume_buy": "842.5",
						"value": "104890000.0",
						"period": 86400
					}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fetcher := NewCoinExFetcherWithBaseURL(ts.URL)

	t.Run("Fetch Spot Ticker for BTC/USDT", func(t *testing.T) {
		quote, err := fetcher.FetchTicker(context.Background(), "BTC/USDT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quote.Price != 68100.25 {
			t.Errorf("expected price 68100.25, got %f", quote.Price)
		}
		if quote.High24h != 68500.00 {
			t.Errorf("expected high 68500.00, got %f", quote.High24h)
		}
	})
}

func TestTradingViewFetcher(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"totalCount": 1,
			"data": [
				{
					"s": "BINANCE:BTCUSDT",
					"d": [67950.0, 2.35, 68500.0, 66400.0, 150000000.0]
				}
			]
		}`))
	}))
	defer ts.Close()

	fetcher := NewTradingViewFetcherWithBaseURL(ts.URL)

	t.Run("Fetch Scan Price for BTC/USDT", func(t *testing.T) {
		price, err := fetcher.GetLatestPrice(context.Background(), "BTC/USDT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if price != 67950.0 {
			t.Errorf("expected price 67950.0, got %f", price)
		}
	})
}

func TestMultiSourcePriceAggregator(t *testing.T) {
	// Primary mock server fails with 500 error
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
	}))
	defer failingServer.Close()

	// Secondary mock server succeeds
	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"code": 0,
			"message": "OK",
			"data": [
				{
					"market": "BTCUSDT",
					"last": "67890.00",
					"open": "66000.00",
					"close": "67890.00",
					"high": "68000.00",
					"low": "65900.00",
					"volume": "100.0",
					"value": "6789000.0",
					"period": 86400
				}
			]
		}`))
	}))
	defer backupServer.Close()

	primaryFetcher := NewCoinExFetcherWithBaseURL(failingServer.URL)
	backupFetcher := NewCoinExFetcherWithBaseURL(backupServer.URL)

	aggregator := NewMultiSourcePriceAggregator([]TickerFetcher{primaryFetcher, backupFetcher})

	price, err := aggregator.GetLatestPrice(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("expected aggregator to seamlessly failover to backup fetcher, got error: %v", err)
	}
	if price != 67890.00 {
		t.Errorf("expected failover price 67890.00, got %f", price)
	}
}
