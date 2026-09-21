package market

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// LiveMarketFeed polls live prices and order book ticks directly from online exchange APIs (e.g. Binance)
// providing continuous, real-time market fluctuations with zero static hardcoded values.
type LiveMarketFeed struct {
	httpClient *http.Client
	baseURL    string
	assets     []AssetDefinition
	mu         sync.RWMutex
	lastPrices map[string]float64
}

// NewLiveMarketFeed creates a new live market feed connecting to real exchange endpoints.
func NewLiveMarketFeed(assets []AssetDefinition) *LiveMarketFeed {
	if len(assets) == 0 {
		assets = GetSupportedAssets()
	}
	return &LiveMarketFeed{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL:    "https://api.binance.com",
		assets:     assets,
		lastPrices: make(map[string]float64),
	}
}

// FetchLiveTick fetches a single live ticker from Binance.
func (f *LiveMarketFeed) FetchLiveTick(ctx context.Context, asset AssetDefinition) (*cache.TickerQuote, error) {
	symbol := asset.SourceParam
	if symbol == "" {
		symbol = strings.ReplaceAll(asset.Symbol, "/", "")
	}

	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", f.baseURL, symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange API returned HTTP %d for %s", resp.StatusCode, symbol)
	}

	var raw struct {
		Symbol             string `json:"symbol"`
		LastPrice          string `json:"lastPrice"`
		PriceChangePercent string `json:"priceChangePercent"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
		QuoteVolume        string `json:"quoteVolume"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	price, err := strconv.ParseFloat(raw.LastPrice, 64)
	if err != nil || price <= 0 {
		return nil, fmt.Errorf("invalid price %q parsed from exchange", raw.LastPrice)
	}

	change, _ := strconv.ParseFloat(raw.PriceChangePercent, 64)
	high, _ := strconv.ParseFloat(raw.HighPrice, 64)
	low, _ := strconv.ParseFloat(raw.LowPrice, 64)
	vol, _ := strconv.ParseFloat(raw.Volume, 64)

	f.mu.Lock()
	f.lastPrices[asset.Symbol] = price
	f.mu.Unlock()

	return &cache.TickerQuote{
		Symbol:    asset.Symbol,
		Price:     price,
		Change24h: change,
		High24h:   high,
		Low24h:    low,
		Volume:    vol,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// FetchAllLiveTicks queries Binance batch ticker endpoint for all pairs simultaneously.
func (f *LiveMarketFeed) FetchAllLiveTicks(ctx context.Context) ([]cache.TickerQuote, error) {
	var symbols []string
	symbolToAsset := make(map[string]AssetDefinition)

	for _, a := range f.assets {
		s := a.SourceParam
		if s == "" {
			s = strings.ReplaceAll(a.Symbol, "/", "")
		}
		symbols = append(symbols, `"`+s+`"`)
		symbolToAsset[s] = a
	}

	query := url.QueryEscape("[" + strings.Join(symbols, ",") + "]")
	apiURL := fmt.Sprintf("%s/api/v3/ticker/24hr?symbols=%s", f.baseURL, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange API returned HTTP %d", resp.StatusCode)
	}

	var rawList []struct {
		Symbol             string `json:"symbol"`
		LastPrice          string `json:"lastPrice"`
		PriceChangePercent string `json:"priceChangePercent"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, err
	}

	var quotes []cache.TickerQuote
	now := time.Now().Unix()

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, raw := range rawList {
		asset, ok := symbolToAsset[raw.Symbol]
		if !ok {
			continue
		}
		price, err := strconv.ParseFloat(raw.LastPrice, 64)
		if err != nil || price <= 0 {
			continue
		}
		change, _ := strconv.ParseFloat(raw.PriceChangePercent, 64)
		high, _ := strconv.ParseFloat(raw.HighPrice, 64)
		low, _ := strconv.ParseFloat(raw.LowPrice, 64)
		vol, _ := strconv.ParseFloat(raw.Volume, 64)

		f.lastPrices[asset.Symbol] = price

		quotes = append(quotes, cache.TickerQuote{
			Symbol:    asset.Symbol,
			Price:     price,
			Change24h: change,
			High24h:   high,
			Low24h:    low,
			Volume:    vol,
			UpdatedAt: now,
		})
	}

	return quotes, nil
}

// Subscribe returns a channel that streams real-time live market ticks directly from the exchange.
func (f *LiveMarketFeed) Subscribe(ctx context.Context, interval time.Duration) <-chan cache.TickerQuote {
	if interval < 500*time.Millisecond {
		interval = 1 * time.Second
	}

	ch := make(chan cache.TickerQuote, 100)

	go func() {
		defer close(ch)

		// Immediate first fetch
		if quotes, err := f.FetchAllLiveTicks(ctx); err == nil {
			for _, q := range quotes {
				select {
				case ch <- q:
				case <-ctx.Done():
					return
				}
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				quotes, err := f.FetchAllLiveTicks(ctx)
				if err != nil {
					log.Printf("[LiveMarketFeed] Live exchange poll error: %v", err)
					continue
				}
				for _, q := range quotes {
					select {
					case ch <- q:
					case <-ctx.Done():
						return
					default:
						// Avoid blocking if channel consumer is momentarily slow
					}
				}
			}
		}
	}()

	return ch
}
