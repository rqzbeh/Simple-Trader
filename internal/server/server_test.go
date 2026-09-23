package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/server"
)

func TestHealthAndAssetsEndpoints(t *testing.T) {
	cfg := &config.Config{
		Port:         "8080",
		AIModelID:    "test-model",
		IsProduction: false,
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)

	// Inject deterministic mock candles to prevent test failures in georestricted CI environments
	baseTime := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	mockCandles := make([]market.HistoricalCandle, 500)
	for i := 0; i < 500; i++ {
		mockCandles[i] = market.HistoricalCandle{
			OpenTime:  baseTime.Add(time.Duration(i) * time.Hour),
			Open:      65000.0 + float64(i)*2,
			High:      65100.0 + float64(i)*2,
			Low:       64900.0 + float64(i)*2,
			Close:     65050.0 + float64(i)*2,
			Volume:    1000.0,
			CloseTime: baseTime.Add(time.Duration(i+1) * time.Hour),
		}
	}
	srv.SetCandleDownloader(&mockKlineDownloader{klines: mockCandles})

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// 1. Health endpoint
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed calling /health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// 2. Assets endpoint
	respAssets, err := http.Get(ts.URL + "/api/v1/assets")
	if err != nil {
		t.Fatalf("failed calling /api/v1/assets: %v", err)
	}
	if respAssets.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", respAssets.StatusCode)
	}

	// 3. Economic Calendar endpoint (FR-005)
	respCal, err := http.Get(ts.URL + "/api/v1/calendar")
	if err != nil {
		t.Fatalf("failed calling /api/v1/calendar: %v", err)
	}
	if respCal.StatusCode != http.StatusOK {
		t.Errorf("expected calendar status 200, got %d", respCal.StatusCode)
	}

	// 4. Backtest & Monte Carlo endpoint (FR-008, SC-004)
	payload := []byte(`{"symbol":"BTC/USD","initial_capital":100000,"bars_count":500}`)
	respBT, err := http.Post(ts.URL+"/api/v1/backtest/run", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("failed calling /api/v1/backtest/run: %v", err)
	}
	if respBT.StatusCode != http.StatusOK {
		t.Errorf("expected backtest status 200, got %d", respBT.StatusCode)
	}
	var btBody map[string]interface{}
	if err := json.NewDecoder(respBT.Body).Decode(&btBody); err != nil {
		t.Fatalf("failed decoding backtest response: %v", err)
	}
	if _, ok := btBody["backtest"]; !ok {
		t.Errorf("missing 'backtest' key in response")
	}
	if _, ok := btBody["monte_carlo"]; !ok {
		t.Errorf("missing 'monte_carlo' key in response")
	}
}

func TestSSEEventBroadcaster(t *testing.T) {
	broadcaster := server.NewSSEBroadcaster()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		broadcaster.ServeHTTP(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed connecting to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream content type, got %s", resp.Header.Get("Content-Type"))
	}

	// Broadcast test event in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		broadcaster.Broadcast("ticker", `{"symbol":"BTC/USD","price":68500.0}`)
	}()

	reader := bufio.NewReader(resp.Body)
	receivedEvent := false

	done := make(chan bool)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(line, "BTC/USD") {
				receivedEvent = true
				done <- true
				return
			}
		}
	}()

	select {
	case <-done:
		if !receivedEvent {
			t.Errorf("expected to receive SSE ticker payload")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE message")
	}
}
