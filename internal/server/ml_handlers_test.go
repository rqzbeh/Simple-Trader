package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

func TestMLEndpoints(t *testing.T) {
	cfg := &config.Config{
		AdminPassword: "test-password-123",
	}
	s := NewServer(cfg, nil, nil, nil, nil, nil)
	r := s.Router()

	t.Run("GET /api/v1/ml/status returns initial status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ml/status", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp["hardware"] == nil {
			t.Errorf("expected hardware field in status response")
		}
		if resp["bayesian_posteriors"] == nil {
			t.Errorf("expected bayesian_posteriors in status response")
		}
	})

	t.Run("GET /api/v1/ml/runs returns empty list when db is nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ml/runs", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var runs []interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(runs) != 0 {
			t.Errorf("expected 0 runs, got %d", len(runs))
		}
	})

	t.Run("POST /api/v1/ml/train validates request input", func(t *testing.T) {
		body := bytes.NewBufferString(`{"symbol":"BTCUSDT","mode":"statistical","candles":10}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ml/train", body)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		// With candles=10, TrainOnAuthenticData should require minimum 50 candles or fail gracefully
		if rec.Code == http.StatusOK {
			t.Errorf("expected failure when candles < 50, got 200")
		}
	})
}
