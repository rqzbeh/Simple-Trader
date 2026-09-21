package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HistoricalCandle represents an authentic Binance candlestick.
type HistoricalCandle struct {
	OpenTime         time.Time `json:"open_time"`
	Open             float64   `json:"open"`
	High             float64   `json:"high"`
	Low              float64   `json:"low"`
	Close            float64   `json:"close"`
	Volume           float64   `json:"volume"`
	CloseTime        time.Time `json:"close_time"`
	QuoteAssetVolume float64   `json:"quote_asset_volume"`
	NumberOfTrades   int       `json:"number_of_trades"`
	TakerBuyBaseVol  float64   `json:"taker_buy_base_vol"`
	TakerBuyQuoteVol float64   `json:"taker_buy_quote_vol"`
}

// HistoricalKlineProvider defines an authentic candlestick history interface.
type HistoricalKlineProvider interface {
	FetchHistoricalKlines(ctx context.Context, symbol string, interval string, targetCount int) ([]HistoricalCandle, error)
}

// BinanceHistoricalDownloader fetches authentic continuous historical klines from Binance Public API (Spot & Futures).
type BinanceHistoricalDownloader struct {
	baseURL        string
	futuresBaseURL string
	httpClient     *http.Client
}

// NewBinanceHistoricalDownloader creates a historical downloader with default endpoints.
func NewBinanceHistoricalDownloader() *BinanceHistoricalDownloader {
	return NewBinanceHistoricalDownloaderWithBaseURLs("https://api.binance.com", "https://fapi.binance.com")
}

// NewBinanceHistoricalDownloaderWithBaseURL creates a downloader with custom base URL.
func NewBinanceHistoricalDownloaderWithBaseURL(baseURL string) *BinanceHistoricalDownloader {
	return NewBinanceHistoricalDownloaderWithBaseURLs(baseURL, "https://fapi.binance.com")
}

// NewBinanceHistoricalDownloaderWithBaseURLs creates a downloader with custom Spot and Futures endpoints.
func NewBinanceHistoricalDownloaderWithBaseURLs(baseURL, futuresBaseURL string) *BinanceHistoricalDownloader {
	return &BinanceHistoricalDownloader{
		baseURL:        strings.TrimRight(baseURL, "/"),
		futuresBaseURL: strings.TrimRight(futuresBaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// FetchHistoricalKlines pulls continuous authentic historical candles backward from endTime.
func (d *BinanceHistoricalDownloader) FetchHistoricalKlines(
	ctx context.Context,
	symbol string,
	interval string,
	targetCount int,
) ([]HistoricalCandle, error) {
	if symbol == "" {
		return nil, errors.New("symbol cannot be empty")
	}
	if targetCount <= 0 {
		targetCount = 1000
	}
	if interval == "" {
		interval = "1h"
	}

	cleanSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	cleanSymbol = strings.ReplaceAll(cleanSymbol, "/", "")
	cleanSymbol = strings.ReplaceAll(cleanSymbol, "-", "")
	cleanSymbol = strings.ReplaceAll(cleanSymbol, "_", "")
	if strings.HasSuffix(cleanSymbol, "USD") && !strings.HasSuffix(cleanSymbol, "USDT") {
		cleanSymbol = cleanSymbol + "T"
	}

	var allCandles []HistoricalCandle
	endTime := time.Now().UnixMilli()

	for len(allCandles) < targetCount {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		batchLimit := 1000
		remaining := targetCount - len(allCandles)
		if remaining < batchLimit {
			batchLimit = remaining
		}

		reqURL := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d&endTime=%d",
			d.baseURL, cleanSymbol, interval, batchLimit, endTime)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", "SimpleTrader-GoHistorical/2.0")

		resp, err := d.httpClient.Do(req)
		var raw [][]interface{}

		// If Spot request fails (e.g. commodities like XAUUSDT, XAGUSDT only on Futures), fall back to Futures API
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}

			futuresURL := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&limit=%d&endTime=%d",
				d.futuresBaseURL, cleanSymbol, interval, batchLimit, endTime)
			reqF, errF := http.NewRequestWithContext(ctx, http.MethodGet, futuresURL, nil)
			if errF != nil {
				return nil, fmt.Errorf("failed to create futures request: %w", errF)
			}
			reqF.Header.Set("User-Agent", "SimpleTrader-GoHistorical/2.0")

			respF, errF := d.httpClient.Do(reqF)
			if errF != nil {
				return nil, fmt.Errorf("http request failed on spot and futures: %w", errF)
			}
			if respF.StatusCode != http.StatusOK {
				respF.Body.Close()
				return nil, fmt.Errorf("binance klines returned non-200 status on spot and futures: %d", respF.StatusCode)
			}
			if err := json.NewDecoder(respF.Body).Decode(&raw); err != nil {
				respF.Body.Close()
				return nil, fmt.Errorf("failed to decode futures klines json: %w", err)
			}
			respF.Body.Close()
		} else {
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("failed to decode klines json: %w", err)
			}
			resp.Body.Close()
		}

		if len(raw) == 0 {
			break
		}

		parsedBatch := make([]HistoricalCandle, 0, len(raw))
		for _, row := range raw {
			if len(row) < 11 {
				continue
			}
			openTimeMs, _ := row[0].(float64)
			open, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[1]), 64)
			high, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[2]), 64)
			low, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[3]), 64)
			closeVal, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[4]), 64)
			volume, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[5]), 64)
			closeTimeMs, _ := row[6].(float64)
			quoteAssetVol, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[7]), 64)
			trades, _ := strconv.Atoi(fmt.Sprintf("%v", row[8]))
			takerBuyBaseVol, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[9]), 64)
			takerBuyQuoteVol, _ := strconv.ParseFloat(fmt.Sprintf("%v", row[10]), 64)

			parsedBatch = append(parsedBatch, HistoricalCandle{
				OpenTime:         time.UnixMilli(int64(openTimeMs)),
				Open:             open,
				High:             high,
				Low:              low,
				Close:            closeVal,
				Volume:           volume,
				CloseTime:        time.UnixMilli(int64(closeTimeMs)),
				QuoteAssetVolume: quoteAssetVol,
				NumberOfTrades:   trades,
				TakerBuyBaseVol:  takerBuyBaseVol,
				TakerBuyQuoteVol: takerBuyQuoteVol,
			})
		}

		// Prepend older candles
		allCandles = append(parsedBatch, allCandles...)

		// Update endTime to 1ms before the earliest candle in this batch
		firstCandleMs := int64(raw[0][0].(float64))
		endTime = firstCandleMs - 1

		if len(raw) < batchLimit {
			break
		}
	}

	return allCandles, nil
}
