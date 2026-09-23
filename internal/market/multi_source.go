package market

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// TickerFetcher is the standardized interface for exchange quote providers.
type TickerFetcher interface {
	FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error)
}

// KuCoinFetcher retrieves crypto and commodity prices from KuCoin REST APIs (Spot & Futures).
type KuCoinFetcher struct {
	spotBaseURL    string
	futuresBaseURL string
	httpClient     *http.Client
}

// NewKuCoinFetcher creates a default KuCoin fetcher.
func NewKuCoinFetcher() *KuCoinFetcher {
	return NewKuCoinFetcherWithBaseURLs("https://api.kucoin.com", "https://api-futures.kucoin.com")
}

// NewKuCoinFetcherWithBaseURLs creates a KuCoin fetcher targeting specific base URLs.
func NewKuCoinFetcherWithBaseURLs(spotBaseURL, futuresBaseURL string) *KuCoinFetcher {
	return &KuCoinFetcher{
		spotBaseURL:    strings.TrimRight(spotBaseURL, "/"),
		futuresBaseURL: strings.TrimRight(futuresBaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

type kucoinSpotLevel1Response struct {
	Code string `json:"code"`
	Data struct {
		Time        int64  `json:"time"`
		Sequence    string `json:"sequence"`
		Price       string `json:"price"`
		Size        string `json:"size"`
		BestBid     string `json:"bestBid"`
		BestBidSize string `json:"bestBidSize"`
		BestAsk     string `json:"bestAsk"`
		BestAskSize string `json:"bestAskSize"`
	} `json:"data"`
}

type kucoinFuturesTickerResponse struct {
	Code string `json:"code"`
	Data struct {
		Sequence     int64   `json:"sequence"`
		Symbol       string  `json:"symbol"`
		Price        float64 `json:"price"`
		Size         float64 `json:"size"`
		BestBidPrice float64 `json:"bestBidPrice"`
		BestBidSize  float64 `json:"bestBidSize"`
		BestAskPrice float64 `json:"bestAskPrice"`
		BestAskSize  float64 `json:"bestAskSize"`
	} `json:"data"`
}

// FetchTicker retrieves the latest quote for a symbol from KuCoin Spot or Futures.
func (k *KuCoinFetcher) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	cleanSym := strings.ToUpper(symbol)
	base := cleanSym
	quote := "USDT"
	if strings.Contains(cleanSym, "/") {
		parts := strings.Split(cleanSym, "/")
		base = parts[0]
		quote = parts[1]
	} else if strings.HasSuffix(cleanSym, "USDT") {
		base = strings.TrimSuffix(cleanSym, "USDT")
	}

	// 1. Try Spot endpoint: /api/v1/market/orderbook/level1?symbol={BASE}-{QUOTE}
	spotPair := fmt.Sprintf("%s-%s", base, quote)
	url := fmt.Sprintf("%s/api/v1/market/orderbook/level1?symbol=%s", k.spotBaseURL, spotPair)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Simple-Trader/KuCoin-Client")
		resp, errDo := k.httpClient.Do(req)
		if errDo == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var raw kucoinSpotLevel1Response
			if errDec := json.NewDecoder(resp.Body).Decode(&raw); errDec == nil && raw.Code == "200000" && raw.Data.Price != "" {
				price, _ := strconv.ParseFloat(raw.Data.Price, 64)
				bid, _ := strconv.ParseFloat(raw.Data.BestBid, 64)
				ask, _ := strconv.ParseFloat(raw.Data.BestAsk, 64)
				if price > 0 {
					return &cache.TickerQuote{
						Symbol:    symbol,
						Price:     price,
						High24h:   ask,
						Low24h:    bid,
						UpdatedAt: time.Now().Unix(),
					}, nil
				}
			}
		} else if resp != nil {
			resp.Body.Close()
		}
	}

	// 2. Fall back to KuCoin Futures: /api/v1/ticker?symbol={BASE}USDTM
	futSymbol := fmt.Sprintf("%sUSDTM", base)
	futURL := fmt.Sprintf("%s/api/v1/ticker?symbol=%s", k.futuresBaseURL, futSymbol)
	reqF, errF := http.NewRequestWithContext(ctx, http.MethodGet, futURL, nil)
	if errF != nil {
		return nil, fmt.Errorf("kucoin futures request creation failed: %w", errF)
	}
	reqF.Header.Set("User-Agent", "Simple-Trader/KuCoin-Client")
	respF, errDoF := k.httpClient.Do(reqF)
	if errDoF != nil {
		return nil, fmt.Errorf("kucoin spot and futures requests failed: %w", errDoF)
	}
	defer respF.Body.Close()

	if respF.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kucoin returned status %d for %s", respF.StatusCode, symbol)
	}

	var rawF kucoinFuturesTickerResponse
	if err := json.NewDecoder(respF.Body).Decode(&rawF); err != nil {
		return nil, fmt.Errorf("failed to decode kucoin futures response: %w", err)
	}
	if rawF.Code != "200000" || rawF.Data.Price <= 0 {
		return nil, fmt.Errorf("kucoin futures returned invalid data for %s", symbol)
	}

	return &cache.TickerQuote{
		Symbol:    symbol,
		Price:     rawF.Data.Price,
		High24h:   rawF.Data.BestAskPrice,
		Low24h:    rawF.Data.BestBidPrice,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// Get24hStats implements MarketStatsProvider for KuCoin.
func (k *KuCoinFetcher) Get24hStats(symbol string) (price float64, volume24h float64, spreadBps float64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quote, err := k.FetchTicker(ctx, symbol)
	if err != nil {
		return 0, 0, 0, err
	}

	spreadBps = 3.5
	if quote.High24h > 0 && quote.Low24h > 0 && quote.High24h >= quote.Low24h {
		spreadBps = ((quote.High24h - quote.Low24h) / quote.Low24h) * 10000.0
	}

	return quote.Price, quote.Volume, spreadBps, nil
}

// CoinExFetcher retrieves quotes from CoinEx v2 Public REST APIs.
type CoinExFetcher struct {
	baseURL    string
	httpClient *http.Client
}

// NewCoinExFetcher creates a default CoinEx fetcher.
func NewCoinExFetcher() *CoinExFetcher {
	return NewCoinExFetcherWithBaseURL("https://api.coinex.com")
}

// NewCoinExFetcherWithBaseURL creates a CoinEx fetcher with custom base URL.
func NewCoinExFetcherWithBaseURL(baseURL string) *CoinExFetcher {
	return &CoinExFetcher{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

type coinexV2TickerResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []struct {
		Market    string `json:"market"`
		Last      string `json:"last"`
		Open      string `json:"open"`
		Close     string `json:"close"`
		High      string `json:"high"`
		Low       string `json:"low"`
		Volume    string `json:"volume"`
		Value     string `json:"value"`
		Period    int64  `json:"period"`
	} `json:"data"`
}

// FetchTicker retrieves a quote from CoinEx v2 Spot Ticker API.
func (c *CoinExFetcher) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	cleanSym := strings.ToUpper(symbol)
	cleanSym = strings.ReplaceAll(cleanSym, "/", "")
	cleanSym = strings.ReplaceAll(cleanSym, "-", "")
	cleanSym = strings.ReplaceAll(cleanSym, "_", "")

	url := fmt.Sprintf("%s/v2/spot/ticker?market=%s", c.baseURL, cleanSym)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create coinex request: %w", err)
	}
	req.Header.Set("User-Agent", "Simple-Trader/CoinEx-Client")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coinex request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coinex returned status: %d", resp.StatusCode)
	}

	var raw coinexV2TickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode coinex response: %w", err)
	}

	if raw.Code != 0 || len(raw.Data) == 0 {
		return nil, fmt.Errorf("coinex returned no ticker data for %s", symbol)
	}

	item := raw.Data[0]
	lastPrice, _ := strconv.ParseFloat(item.Last, 64)
	high, _ := strconv.ParseFloat(item.High, 64)
	low, _ := strconv.ParseFloat(item.Low, 64)
	val, _ := strconv.ParseFloat(item.Value, 64)

	var change24h float64
	openPrice, _ := strconv.ParseFloat(item.Open, 64)
	if openPrice > 0 {
		change24h = ((lastPrice - openPrice) / openPrice) * 100.0
	}

	return &cache.TickerQuote{
		Symbol:    symbol,
		Price:     lastPrice,
		Change24h: change24h,
		High24h:   high,
		Low24h:    low,
		Volume:    val,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// Get24hStats implements MarketStatsProvider for CoinEx.
func (c *CoinExFetcher) Get24hStats(symbol string) (price float64, volume24h float64, spreadBps float64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quote, err := c.FetchTicker(ctx, symbol)
	if err != nil {
		return 0, 0, 0, err
	}

	return quote.Price, quote.Volume, 4.0, nil
}

// TradingViewFetcher retrieves institutional market quotes from TradingView's public scanner APIs.
type TradingViewFetcher struct {
	baseURL    string
	httpClient *http.Client
}

// NewTradingViewFetcher creates a default TradingView fetcher.
func NewTradingViewFetcher() *TradingViewFetcher {
	return NewTradingViewFetcherWithBaseURL("https://scanner.tradingview.com")
}

// NewTradingViewFetcherWithBaseURL creates a TradingView fetcher with custom base URL.
func NewTradingViewFetcherWithBaseURL(baseURL string) *TradingViewFetcher {
	return &TradingViewFetcher{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

type tvScannerResponse struct {
	TotalCount int `json:"totalCount"`
	Data       []struct {
		S string        `json:"s"`
		D []interface{} `json:"d"`
	} `json:"data"`
}

// GetLatestPrice implements PriceProvider for TradingView.
func (t *TradingViewFetcher) GetLatestPrice(ctx context.Context, symbol string) (float64, error) {
	quote, err := t.FetchTicker(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return quote.Price, nil
}

// FetchTicker queries TradingView Crypto or CFD scanner API.
func (t *TradingViewFetcher) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	clean := strings.ToUpper(symbol)
	clean = strings.ReplaceAll(clean, "/", "")

	// Determine scanner domain (crypto or cfd)
	endpoint := "crypto"
	tvSymbol := fmt.Sprintf("BINANCE:%s", clean)

	// Check commodity mappings
	switch symbol {
	case "OIL/USDT", "WTI/USDT", "CL=F":
		endpoint = "cfd"
		tvSymbol = "TVC:USOIL"
	case "BRENT/USDT", "BZ=F":
		endpoint = "cfd"
		tvSymbol = "TVC:UKOIL"
	case "ALU/USDT", "ALI=F":
		endpoint = "cfd"
		tvSymbol = "TVC:ALUMINUM"
	case "COPPER/USDT", "HG=F":
		endpoint = "cfd"
		tvSymbol = "COMEX:HG1!"
	case "XPT/USDT", "PL=F":
		endpoint = "cfd"
		tvSymbol = "TVC:PLATINUM"
	case "XPD/USDT", "PA=F":
		endpoint = "cfd"
		tvSymbol = "TVC:PALLADIUM"
	case "XAU/USDT", "PAXG/USDT":
		endpoint = "crypto"
		if symbol != "PAXG/USDT" {
			endpoint = "cfd"
			tvSymbol = "TVC:GOLD"
		}
	}

	url := fmt.Sprintf("%s/%s/scan", t.baseURL, endpoint)
	payload := map[string]interface{}{
		"symbols": map[string]interface{}{
			"tickers": []string{tvSymbol},
		},
		"columns": []string{"close", "change", "high", "low", "volume"},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tradingview payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create tradingview request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Simple-Trader/TV-Scanner")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tradingview scanner request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tradingview returned status: %d", resp.StatusCode)
	}

	var raw tvScannerResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode tradingview response: %w", err)
	}

	if len(raw.Data) == 0 || len(raw.Data[0].D) == 0 {
		return nil, fmt.Errorf("tradingview scanner returned no data for %s (%s)", symbol, tvSymbol)
	}

	d := raw.Data[0].D
	var price, change, high, low, vol float64
	if len(d) > 0 {
		price = toFloat(d[0])
	}
	if len(d) > 1 {
		change = toFloat(d[1])
	}
	if len(d) > 2 {
		high = toFloat(d[2])
	}
	if len(d) > 3 {
		low = toFloat(d[3])
	}
	if len(d) > 4 {
		vol = toFloat(d[4])
	}

	if price <= 0 {
		return nil, fmt.Errorf("tradingview scanner returned zero or negative price for %s", symbol)
	}

	return &cache.TickerQuote{
		Symbol:    symbol,
		Price:     price,
		Change24h: change,
		High24h:   high,
		Low24h:    low,
		Volume:    vol,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

func toFloat(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

// MultiSourcePriceAggregator implements resilient price resolution across prioritized exchanges.
type MultiSourcePriceAggregator struct {
	fetchers []TickerFetcher
}

// NewMultiSourcePriceAggregator creates an aggregator that cascades through providers until a valid quote is found.
func NewMultiSourcePriceAggregator(fetchers []TickerFetcher) *MultiSourcePriceAggregator {
	return &MultiSourcePriceAggregator{
		fetchers: fetchers,
	}
}

// NewDefaultMultiSourceAggregator creates a fully integrated aggregator (Binance -> KuCoin -> CoinEx -> TradingView -> Yahoo).
func NewDefaultMultiSourceAggregator() *MultiSourcePriceAggregator {
	return NewMultiSourcePriceAggregator([]TickerFetcher{
		NewBinanceFetcher(),
		NewKuCoinFetcher(),
		NewCoinExFetcher(),
		NewTradingViewFetcher(),
		NewYahooFinanceFetcher(),
	})
}

// FetchTicker queries fetchers in sequence until a positive quote is returned.
func (m *MultiSourcePriceAggregator) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	var lastErr error
	for _, f := range m.fetchers {
		quote, err := f.FetchTicker(ctx, symbol)
		if err == nil && quote != nil && quote.Price > 0 {
			return quote, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all exchange fetchers failed for %s: %w", symbol, lastErr)
	}
	return nil, fmt.Errorf("no exchange fetcher returned a valid price for %s", symbol)
}

// GetLatestPrice implements PriceProvider interface.
func (m *MultiSourcePriceAggregator) GetLatestPrice(ctx context.Context, symbol string) (float64, error) {
	quote, err := m.FetchTicker(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return quote.Price, nil
}
