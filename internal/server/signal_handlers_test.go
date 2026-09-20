package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
}

type mockAIClient struct {
	response *ai.DecisionResponse
	err      error
}

func (m *mockAIClient) Analyze(ctx context.Context, req ai.DecisionRequest) (*ai.DecisionResponse, error) {
	return m.response, m.err
}
