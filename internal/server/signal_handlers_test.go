package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/server"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func TestFuturesSignalsEndpoints(t *testing.T) {
	cfg := &config.Config{
		AIModelID: "test-model",
	}

	allocCfg := trader.Default3TierConfig(100000.0)
	allocator := trader.NewAllocator(allocCfg)

	srv := server.NewServer(cfg, nil, nil, nil, allocator, nil)
	r := srv.Router()

	// 1. Test GET /api/v1/signals/futures when dbStore is nil (should return empty list)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/signals/futures", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var signals []db.FuturesTradeSignal
	if err := json.Unmarshal(rec.Body.Bytes(), &signals); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(signals))
	}

	// 2. Test POST /api/v1/signals/futures/decide when aiClient is nil (service unavailable)
	decReq := map[string]string{
		"symbol": "BTC/USD",
		"bucket": "ALPHA",
	}
	bodyBytes, _ := json.Marshal(decReq)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/signals/futures/decide", bytes.NewReader(bodyBytes))
	reqPost.Header.Set("Content-Type", "application/json")
	recPost := httptest.NewRecorder()
	r.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for unconfigured AI client, got %d", recPost.Code)
	}

	// 3. Test POST /api/v1/signals/futures/{id}/close when dbStore is nil (service unavailable)
	reqClose := httptest.NewRequest(http.MethodPost, "/api/v1/signals/futures/101/close", bytes.NewReader([]byte(`{"exit_price": 64000.0}`)))
	reqClose.Header.Set("Content-Type", "application/json")
	recClose := httptest.NewRecorder()
	r.ServeHTTP(recClose, reqClose)

	if recClose.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for nil dbStore on close, got %d", recClose.Code)
	}

	// 4. Test POST /api/v1/signals/futures/decide-all when aiClient is nil (service unavailable)
	reqDecideAll := httptest.NewRequest(http.MethodPost, "/api/v1/signals/futures/decide-all", bytes.NewReader([]byte(`{}`)))
	reqDecideAll.Header.Set("Content-Type", "application/json")
	recDecideAll := httptest.NewRecorder()
	r.ServeHTTP(recDecideAll, reqDecideAll)

	if recDecideAll.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for unconfigured AI client on decide-all, got %d", recDecideAll.Code)
	}
}

func TestGenerateAllFuturesSignalsDecideAllNilStore(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `{"decision":"HOLD","confidence":0.7,"reasoning":"test","timeframe":"4h"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	aiClient := ai.NewClient(ai.ClientConfig{
		BaseURL:     mockServer.URL,
		APIKey:      "test-api-key",
		ModelID:     "test-model",
		Temperature: 0.2,
		TimeoutSec:  5,
	})

	cfg := &config.Config{
		AIModelID: "test-model",
	}
	allocCfg := trader.Default3TierConfig(100000.0)
	allocator := trader.NewAllocator(allocCfg)

	// Server initialized with nil dbStore, nil redisClient
	srv := server.NewServer(cfg, nil, nil, aiClient, allocator, nil)
	r := srv.Router()

	reqBody := `{"symbols":["BTC/USDT"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/futures/decide-all", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ScannedCount int                      `json:"scanned_count"`
		SignalsCount int                      `json:"signals_count"`
		Results      []server.AssetScanResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ScannedCount != 1 {
		t.Fatalf("expected scanned_count=1, got %d", resp.ScannedCount)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

func TestGenerateAllFuturesSignalsBatchDeadlinePartial(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	aiClient := ai.NewClient(ai.ClientConfig{
		BaseURL:     mockServer.URL,
		APIKey:      "test-api-key",
		ModelID:     "test-model",
		Temperature: 0.2,
		TimeoutSec:  5,
	})

	cfg := &config.Config{
		AIModelID: "test-model",
	}
	allocCfg := trader.Default3TierConfig(100000.0)
	allocator := trader.NewAllocator(allocCfg)

	srv := server.NewServer(cfg, nil, nil, aiClient, allocator, nil)
	r := srv.Router()

	// Pre-cancelled context should trigger batchCtx early exit and return SKIPPED results immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/futures/decide-all", bytes.NewReader([]byte(`{"symbols":["BTC/USDT","ETH/USDT"]}`))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		ScannedCount int                      `json:"scanned_count"`
		Results      []server.AssetScanResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ScannedCount != 2 {
		t.Fatalf("expected scanned_count=2, got %d", resp.ScannedCount)
	}
	for _, res := range resp.Results {
		if res.Status != "SKIPPED" {
			t.Errorf("expected SKIPPED on pre-cancelled context, got %s", res.Status)
		}
	}
}
