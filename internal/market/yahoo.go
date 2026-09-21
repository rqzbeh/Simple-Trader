package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// YahooFinanceFetcher retrieves commodity prices from Yahoo Finance Chart API.
type YahooFinanceFetcher struct {
	baseURL    string
	httpClient *http.Client
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol               string  `json:"symbol"`
				RegularMarketPrice   float64 `json:"regularMarketPrice"`
				ChartPreviousClose   float64 `json:"chartPreviousClose"`
				RegularMarketDayHigh float64 `json:"regularMarketDayHigh"`
				RegularMarketDayLow  float64 `json:"regularMarketDayLow"`
				RegularMarketVolume  float64 `json:"regularMarketVolume"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

// NewYahooFinanceFetcher creates a default Yahoo Finance quote fetcher.
func NewYahooFinanceFetcher() *YahooFinanceFetcher {
	return NewYahooFinanceFetcherWithBaseURL("https://query1.finance.yahoo.com")
}

// NewYahooFinanceFetcherWithBaseURL creates a Yahoo Finance fetcher with a custom base URL.
func NewYahooFinanceFetcherWithBaseURL(baseURL string) *YahooFinanceFetcher {
	return &YahooFinanceFetcher{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
}

// mapToYahooSymbol normalizes symbols to Yahoo Finance contract tickers.
func mapToYahooSymbol(symbol string) string {
	clean := strings.ToUpper(symbol)
	switch clean {
	case "OIL/USDT", "OILUSDT", "WTI/USDT", "WTI":
		return "CL=F"
	case "BRENT/USDT", "BRENT":
		return "BZ=F"
	case "ALU/USDT", "ALUUSDT", "ALUMINUM":
		return "ALI=F"
	case "COPPER/USDT", "COPPERUSDT":
		return "HG=F"
	case "XAU/USDT", "XAUUSDT", "GOLD":
		return "GC=F"
	case "XAG/USDT", "XAGUSDT", "SILVER":
		return "SI=F"
	case "XPT/USDT", "XPTUSDT", "PLATINUM":
		return "PL=F"
	case "XPD/USDT", "XPDUSDT", "PALLADIUM":
		return "PA=F"
	default:
		return symbol
	}
}

// FetchTicker implements TickerFetcher interface for Yahoo Finance.
func (y *YahooFinanceFetcher) FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	return y.FetchQuote(ctx, symbol)
}

// FetchQuote fetches commodity futures quotes (e.g. "GC=F" for Gold, "SI=F" for Silver, "CL=F" for Crude Oil, "ALI=F" for Aluminum).
func (y *YahooFinanceFetcher) FetchQuote(ctx context.Context, symbol string) (*cache.TickerQuote, error) {
	yahooSym := mapToYahooSymbol(symbol)
	url := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=1d", y.baseURL, yahooSym)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create yahoo finance request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Simple-Trader/1.0")

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo finance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance returned status: %d", resp.StatusCode)
	}

	var raw yahooChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode yahoo finance response: %w", err)
	}

	if len(raw.Chart.Result) == 0 {
		return nil, fmt.Errorf("empty chart result returned for %s", symbol)
	}

	meta := raw.Chart.Result[0].Meta
	var change24h float64
	if meta.ChartPreviousClose > 0 {
		change24h = ((meta.RegularMarketPrice - meta.ChartPreviousClose) / meta.ChartPreviousClose) * 100.0
	}

	return &cache.TickerQuote{
		Symbol:    symbol,
		Price:     meta.RegularMarketPrice,
		Change24h: change24h,
		High24h:   meta.RegularMarketDayHigh,
		Low24h:    meta.RegularMarketDayLow,
		Volume:    meta.RegularMarketVolume,
		UpdatedAt: time.Now().Unix(),
	}, nil
}
