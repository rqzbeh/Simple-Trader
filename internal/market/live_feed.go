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

// LiveMarketFeed polls live prices and order book ticks directly from online exchange APIs
// (Binance Spot, Binance Futures, Yahoo Finance) providing continuous, real-time market fluctuations
// with zero static hardcoded values.
type LiveMarketFeed struct {
	httpClient   *http.Client
	spotBaseURL  string
	fapiBaseURL  string
	yahooFetcher *YahooFinanceFetcher
	assets       []AssetDefinition
	mu           sync.RWMutex
	lastQuotes   map[string]cache.TickerQuote
}

// NewLiveMarketFeed creates a new live market feed connecting to real multi-source exchange endpoints.
func NewLiveMarketFeed(assets []AssetDefinition) *LiveMarketFeed {
	if len(assets) == 0 {
		assets = GetSupportedAssets()
	}
	return &LiveMarketFeed{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		spotBaseURL:  "https://api.binance.com",
		fapiBaseURL:  "https://fapi.binance.com",
		yahooFetcher: NewYahooFinanceFetcher(),
		assets:       assets,
		lastQuotes:   make(map[string]cache.TickerQuote),
	}
}

// binanceRawTicker models the JSON response from Binance Spot and Futures 24hr ticker endpoints.
type binanceRawTicker struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	PriceChangePercent string `json:"priceChangePercent"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
}

// parseTickerQuote converts a raw ticker response into a normalized domain TickerQuote.
func parseTickerQuote(symbol string, raw binanceRawTicker, now int64) (*cache.TickerQuote, error) {
	price, err := strconv.ParseFloat(raw.LastPrice, 64)
	if err != nil || price <= 0 {
		return nil, fmt.Errorf("invalid price %q for %s", raw.LastPrice, symbol)
	}

	change, _ := strconv.ParseFloat(raw.PriceChangePercent, 64)
	high, _ := strconv.ParseFloat(raw.HighPrice, 64)
	low, _ := strconv.ParseFloat(raw.LowPrice, 64)
	vol, _ := strconv.ParseFloat(raw.Volume, 64)

	return &cache.TickerQuote{
		Symbol:    symbol,
		Price:     price,
		Change24h: change,
		High24h:   high,
		Low24h:    low,
		Volume:    vol,
		UpdatedAt: now,
	}, nil
}

// FetchLiveTick fetches a single live ticker from its appropriate feed source.
func (f *LiveMarketFeed) FetchLiveTick(ctx context.Context, asset AssetDefinition) (*cache.TickerQuote, error) {
	now := time.Now().Unix()

	switch asset.FeedSource {
	case "YAHOO":
		q, err := f.yahooFetcher.FetchQuote(ctx, asset.Symbol)
		if err != nil {
			return nil, err
		}
		f.mu.Lock()
		f.lastQuotes[asset.Symbol] = *q
		f.mu.Unlock()
		return q, nil

	case "BINANCE_FUTURES":
		param := asset.SourceParam
		if param == "" {
			param = strings.ReplaceAll(asset.Symbol, "/", "")
		}
		apiURL := fmt.Sprintf("%s/fapi/v1/ticker/24hr?symbol=%s", f.fapiBaseURL, param)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "SimpleTrader/1.0")

		resp, err := f.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("binance futures returned HTTP %d for %s", resp.StatusCode, param)
		}

		var raw binanceRawTicker
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, err
		}

		q, err := parseTickerQuote(asset.Symbol, raw, now)
		if err != nil {
			return nil, err
		}

		f.mu.Lock()
		f.lastQuotes[asset.Symbol] = *q
		f.mu.Unlock()
		return q, nil

	default: // BINANCE_SPOT
		param := asset.SourceParam
		if param == "" {
			param = strings.ReplaceAll(asset.Symbol, "/", "")
		}
		apiURL := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", f.spotBaseURL, param)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "SimpleTrader/1.0")

		resp, err := f.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("binance spot returned HTTP %d for %s", resp.StatusCode, param)
		}

		var raw binanceRawTicker
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, err
		}

		q, err := parseTickerQuote(asset.Symbol, raw, now)
		if err != nil {
			return nil, err
		}

		f.mu.Lock()
		f.lastQuotes[asset.Symbol] = *q
		f.mu.Unlock()
		return q, nil
	}
}

// FetchAllLiveTicks queries all exchange endpoints concurrently and aggregates live quotes for all assets.
func (f *LiveMarketFeed) FetchAllLiveTicks(ctx context.Context) ([]cache.TickerQuote, error) {
	var (
		spotAssets    []AssetDefinition
		futuresAssets []AssetDefinition
		yahooAssets   []AssetDefinition
	)

	for _, a := range f.assets {
		switch a.FeedSource {
		case "YAHOO":
			yahooAssets = append(yahooAssets, a)
		case "BINANCE_FUTURES":
			futuresAssets = append(futuresAssets, a)
		default:
			spotAssets = append(spotAssets, a)
		}
	}

	var (
		quotesMu sync.Mutex
		quotes   []cache.TickerQuote
		wg       sync.WaitGroup
		now      = time.Now().Unix()
	)

	// 1. Fetch Binance Spot Batch
	if len(spotAssets) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var symParams []string
			paramToAsset := make(map[string]AssetDefinition)
			for _, a := range spotAssets {
				p := a.SourceParam
				if p == "" {
					p = strings.ReplaceAll(a.Symbol, "/", "")
				}
				symParams = append(symParams, p)
				paramToAsset[p] = a
			}

			// Format compact JSON array without spaces: ["BTCUSDT","ETHUSDT"]
			queryParam := `["` + strings.Join(symParams, `","`) + `"]`
			apiURL := fmt.Sprintf("%s/api/v3/ticker/24hr?symbols=%s", f.spotBaseURL, url.QueryEscape(queryParam))

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if err != nil {
				log.Printf("[LiveMarketFeed] Spot batch req build error: %v", err)
				return
			}
			req.Header.Set("User-Agent", "SimpleTrader/1.0")

			resp, err := f.httpClient.Do(req)
			if err != nil {
				log.Printf("[LiveMarketFeed] Spot batch HTTP error: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("[LiveMarketFeed] Spot batch returned HTTP %d", resp.StatusCode)
				return
			}

			var rawList []binanceRawTicker
			if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
				log.Printf("[LiveMarketFeed] Spot batch decode error: %v", err)
				return
			}

			quotesMu.Lock()
			for _, raw := range rawList {
				if asset, ok := paramToAsset[raw.Symbol]; ok {
					if q, err := parseTickerQuote(asset.Symbol, raw, now); err == nil {
						quotes = append(quotes, *q)
					}
				}
			}
			quotesMu.Unlock()
		}()
	}

	// 2. Fetch Binance Futures Batch (Metals)
	if len(futuresAssets) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			paramToAsset := make(map[string]AssetDefinition)
			for _, a := range futuresAssets {
				p := a.SourceParam
				if p == "" {
					p = strings.ReplaceAll(a.Symbol, "/", "")
				}
				paramToAsset[p] = a
			}

			apiURL := fmt.Sprintf("%s/fapi/v1/ticker/24hr", f.fapiBaseURL)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if err != nil {
				log.Printf("[LiveMarketFeed] Futures batch req build error: %v", err)
				return
			}
			req.Header.Set("User-Agent", "SimpleTrader/1.0")

			resp, err := f.httpClient.Do(req)
			if err != nil {
				log.Printf("[LiveMarketFeed] Futures batch HTTP error: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("[LiveMarketFeed] Futures batch returned HTTP %d", resp.StatusCode)
				return
			}

			var rawList []binanceRawTicker
			if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
				log.Printf("[LiveMarketFeed] Futures batch decode error: %v", err)
				return
			}

			quotesMu.Lock()
			for _, raw := range rawList {
				if asset, ok := paramToAsset[raw.Symbol]; ok {
					if q, err := parseTickerQuote(asset.Symbol, raw, now); err == nil {
						quotes = append(quotes, *q)
					}
				}
			}
			quotesMu.Unlock()
		}()
	}

	// 3. Fetch Yahoo Commodities Concurrently
	for _, a := range yahooAssets {
		asset := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, err := f.yahooFetcher.FetchQuote(ctx, asset.Symbol)
			if err != nil {
				log.Printf("[LiveMarketFeed] Yahoo fetch error for %s: %v", asset.Symbol, err)
				return
			}
			quotesMu.Lock()
			quotes = append(quotes, *q)
			quotesMu.Unlock()
		}()
	}

	wg.Wait()

	// Update in-memory quote cache
	f.mu.Lock()
	for _, q := range quotes {
		f.lastQuotes[q.Symbol] = q
	}
	// If any asset failed in this pass, backfill from latest cached quote if present
	if len(quotes) < len(f.assets) {
		present := make(map[string]bool)
		for _, q := range quotes {
			present[q.Symbol] = true
		}
		for _, a := range f.assets {
			if !present[a.Symbol] {
				if cached, ok := f.lastQuotes[a.Symbol]; ok {
					quotes = append(quotes, cached)
				}
			}
		}
	}
	f.mu.Unlock()

	if len(quotes) == 0 {
		return nil, fmt.Errorf("failed to fetch any live quotes from multi-source feed")
	}

	return quotes, nil
}

// GetLastQuote retrieves the most recent quote for a symbol.
func (f *LiveMarketFeed) GetLastQuote(symbol string) (cache.TickerQuote, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	q, ok := f.lastQuotes[symbol]
	return q, ok
}

// GetLastQuotes returns all cached quotes.
func (f *LiveMarketFeed) GetLastQuotes() []cache.TickerQuote {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var list []cache.TickerQuote
	for _, q := range f.lastQuotes {
		list = append(list, q)
	}
	return list
}

// Subscribe returns a channel that streams real-time live market ticks directly from the exchange.
func (f *LiveMarketFeed) Subscribe(ctx context.Context, interval time.Duration) <-chan cache.TickerQuote {
	if interval < 500*time.Millisecond {
		interval = 1 * time.Second
	}

	ch := make(chan cache.TickerQuote, 250)

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
