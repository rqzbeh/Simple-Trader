package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// BinanceFetcher retrieves real-time crypto prices from Binance public REST endpoints.
type BinanceFetcher struct {
	baseURL    string
	httpClient *http.Client
}

type binance24hrResponse struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	BidPrice           string `json:"bidPrice"`
	AskPrice           string `json:"askPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	CloseTime          int64  `json:"closeTime"`
}

// NewBinanceFetcher creates a default Binance market fetcher.
func NewBinanceFetcher() *BinanceFetcher {
	return NewBinanceFetcherWithBaseURL("https://api.binance.com")
}

// NewBinanceFetcherWithBaseURL creates a Binance fetcher targeting a specific base URL (for testing or proxying).
func NewBinanceFetcherWithBaseURL(baseURL string) *BinanceFetcher {
	return &BinanceFetcher{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Get24hStats implements MarketStatsProvider for DynamicCryptoScreener with real live metrics.
func (b *BinanceFetcher) Get24hStats(symbol string) (price float64, volume24h float64, spreadBps float64, err error) {
	formatted := strings.ReplaceAll(symbol, "/", "")
	if strings.HasSuffix(formatted, "USD") && !strings.HasSuffix(formatted, "USDT") && !strings.HasSuffix(formatted, "USDC") {
		formatted += "T"
	}

	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, formatted)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to create binance request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("binance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("binance returned non-200 status: %d", resp.StatusCode)
	}

	var raw binance24hrResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to decode binance response: %w", err)
	}

	price, _ = strconv.ParseFloat(raw.LastPrice, 64)
	bidPrice, _ := strconv.ParseFloat(raw.BidPrice, 64)
	askPrice, _ := strconv.ParseFloat(raw.AskPrice, 64)
	quoteVol, _ := strconv.ParseFloat(raw.QuoteVolume, 64)

	spread := 2.5
	if bidPrice > 0 && askPrice >= bidPrice {
		spread = ((askPrice - bidPrice) / bidPrice) * 10000.0
	}

	return price, quoteVol, spread, nil
}

// FetchTicker fetches the latest 24hr ticker for a symbol (e.g. "BTCUSDT").
func (b *BinanceFetcher) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create binance request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance returned non-200 status: %d", resp.StatusCode)
	}

	var raw binance24hrResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode binance response: %w", err)
	}

	price, _ := strconv.ParseFloat(raw.LastPrice, 64)
	change, _ := strconv.ParseFloat(raw.PriceChangePercent, 64)
	high, _ := strconv.ParseFloat(raw.HighPrice, 64)
	low, _ := strconv.ParseFloat(raw.LowPrice, 64)
	vol, _ := strconv.ParseFloat(raw.Volume, 64)

	return &cache.TickerQuote{
		Symbol:    raw.Symbol,
		Price:     price,
		Change24h: change,
		High24h:   high,
		Low24h:    low,
		Volume:    vol,
		UpdatedAt: time.Now().Unix(),
	}, nil
}
