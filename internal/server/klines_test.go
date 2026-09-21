package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/server"
)

type mockKlineDownloader struct {
	klines []market.HistoricalCandle
	err    error
}

func (m *mockKlineDownloader) FetchHistoricalKlines(ctx context.Context, symbol, interval string, limit int) ([]market.HistoricalCandle, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.klines, nil
}

func TestKlinesEndpoint(t *testing.T) {
	cfg := &config.Config{
		InitialCapital: 100000.0,
		CoreTargetPct:  0.50,
		AlphaTargetPct: 0.50,
	}

	baseTime := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	mockCandles := []market.HistoricalCandle{
		{
			OpenTime:  baseTime,
			Open:      65000.0,
			High:      65500.0,
			Low:       64800.0,
			Close:     65300.0,
			Volume:    1250.5,
			CloseTime: baseTime.Add(time.Hour),
		},
		{
			OpenTime:  baseTime.Add(time.Hour),
			Open:      65300.0,
			High:      66000.0,
			Low:       65200.0,
			Close:     65900.0,
			Volume:    1800.2,
			CloseTime: baseTime.Add(2 * time.Hour),
		},
	}

	downloader := &mockKlineDownloader{
		klines: mockCandles,
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	srv.SetCandleDownloader(downloader)
	r := srv.Router()

	t.Run("successful kline fetch with query params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/klines?symbol=BTCUSDT&interval=1h&limit=2", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		type FormattedCandle struct {
			Time   int64   `json:"time"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		}

		var resCandles []FormattedCandle
		if err := json.Unmarshal(rec.Body.Bytes(), &resCandles); err != nil {
			t.Fatalf("failed to parse json response: %v", err)
		}

		if len(resCandles) != 2 {
			t.Fatalf("expected 2 candles, got %d", len(resCandles))
		}

		if resCandles[0].Close != 65300.0 || resCandles[1].Close != 65900.0 {
			t.Errorf("unexpected candle close prices: %+v", resCandles)
		}
		if resCandles[0].Time != baseTime.Unix() {
			t.Errorf("expected time %d, got %d", baseTime.Unix(), resCandles[0].Time)
		}
	})

	t.Run("downloader error returns 502 Bad Gateway", func(t *testing.T) {
		errorDownloader := &mockKlineDownloader{
			err: errors.New("exchange rate limit exceeded"),
		}
		errSrv := server.NewServer(cfg, nil, nil, nil, nil, nil)
		errSrv.SetCandleDownloader(errorDownloader)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/klines?symbol=ETHUSDT", nil)
		rec := httptest.NewRecorder()
		errSrv.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected status 502, got %d", rec.Code)
		}
	})

	t.Run("uninitialized downloader returns 500", func(t *testing.T) {
		nilSrv := server.NewServer(cfg, nil, nil, nil, nil, nil)
		nilSrv.SetCandleDownloader(nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/klines?symbol=ETHUSDT", nil)
		rec := httptest.NewRecorder()
		nilSrv.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", rec.Code)
		}
	})
}
