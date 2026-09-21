package market_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestNewsDeduplication(t *testing.T) {
	dedup := market.NewNewsDeduplicator()

	n1 := market.NewsArticle{
		Title:       "Federal Reserve keeps rates unchanged at 5.25%",
		URL:         "https://reuters.com/markets/fed-rates-01",
		Source:      "Reuters",
		PublishedAt: time.Now(),
	}

	n2 := market.NewsArticle{
		Title:       "Federal Reserve keeps rates unchanged at 5.25%",
		URL:         "https://bloomberg.com/news/fed-rates-02",
		Source:      "Bloomberg",
		PublishedAt: time.Now(),
	}

	n3 := market.NewsArticle{
		Title:       "Gold rallies towards all-time highs amid geopolitical tension",
		URL:         "https://cnbc.com/gold-rally",
		Source:      "CNBC",
		PublishedAt: time.Now(),
	}

	if !dedup.IsNew(n1) {
		t.Errorf("expected first article to be new")
	}

	// Identical title or normalized title should be flagged as duplicate
	if dedup.IsNew(n2) {
		t.Errorf("expected duplicate title to be rejected")
	}

	// Different article should be accepted
	if !dedup.IsNew(n3) {
		t.Errorf("expected distinct article to be accepted")
	}
}

func TestBinanceMarketFetcher(t *testing.T) {
	// Mock Binance 24hr ticker response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"symbol": "BTCUSDT",
			"lastPrice": "68500.50",
			"priceChangePercent": "2.45",
			"highPrice": "69200.00",
			"lowPrice": "66800.00",
			"volume": "15230.12",
			"closeTime": 1726800000000
		}`))
	}))
	defer mockServer.Close()

	fetcher := market.NewBinanceFetcherWithBaseURL(mockServer.URL)
	quote, err := fetcher.FetchTicker(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}

	if quote.Symbol != "BTCUSDT" || quote.Price != 68500.50 {
		t.Errorf("unexpected quote data: %+v", quote)
	}
	if quote.Change24h != 2.45 {
		t.Errorf("expected change 2.45, got %f", quote.Change24h)
	}
}

func TestYahooFinanceFetcher(t *testing.T) {
	// Mock Yahoo Finance chart API response for GC=F (Gold)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"chart": {
				"result": [{
					"meta": {
						"symbol": "GC=F",
						"regularMarketPrice": 2685.40,
						"chartPreviousClose": 2660.00,
						"regularMarketDayHigh": 2690.00,
						"regularMarketDayLow": 2655.00,
						"regularMarketVolume": 142000
					}
				}]
			}
		}`))
	}))
	defer mockServer.Close()

	fetcher := market.NewYahooFinanceFetcherWithBaseURL(mockServer.URL)
	quote, err := fetcher.FetchQuote(context.Background(), "GC=F")
	if err != nil {
		t.Fatalf("unexpected yahoo fetch error: %v", err)
	}

	if quote.Symbol != "GC=F" || quote.Price != 2685.40 {
		t.Errorf("unexpected quote data: %+v", quote)
	}
	expectedChange := ((2685.40 - 2660.00) / 2660.00) * 100
	if quote.Change24h < expectedChange-0.1 || quote.Change24h > expectedChange+0.1 {
		t.Errorf("expected change ~%f, got %f", expectedChange, quote.Change24h)
	}
}

func TestAssetUniverse(t *testing.T) {
	assets := market.GetSupportedAssets()
	if len(assets) == 0 {
		t.Fatal("expected non-empty supported assets list")
	}

	hasCore := false
	hasAlpha := false
	for _, a := range assets {
		if a.Bucket == "CORE" && (a.Symbol == "PAXG/USDT" || a.Symbol == "BNB/USDT") {
			hasCore = true
		}
		if a.Bucket == "ALPHA" && a.Symbol == "BTC/USDT" {
			hasAlpha = true
		}
		// Confirm zero Iranian assets exist
		if a.Bucket == "IRAN" || a.Symbol == "IRO1FOLD0001" {
			t.Errorf("illegal Iranian asset remaining: %s", a.Symbol)
		}
	}

	if !hasCore || !hasAlpha {
		t.Errorf("expected both Core and Alpha assets present")
	}
}

func TestSimulatedFeed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	feed := market.NewSimulatedFeed()
	ch := feed.Subscribe(ctx)

	select {
	case tick := <-ch:
		if tick.Symbol == "" || tick.Price <= 0 {
			t.Errorf("invalid tick received: %+v", tick)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for simulated tick")
	}
}

