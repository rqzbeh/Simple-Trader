package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/server"
)

func TestSystemStatsEndpoint(t *testing.T) {
	ai.ResetStatsForTest()
	ai.RecordGateway(true, 120.0, "")
	ai.RecordGateway(false, 300.0, "network timeout")
	ai.RecordJev(true, 45.0, "")

	cfg := &config.Config{
		Port:         "8080",
		AIModelID:    "test-model",
		IsProduction: false,
	}

	_ = os.Setenv("TYPESAFE_API_KEY", "test-typesafe-key")
	defer os.Unsetenv("TYPESAFE_API_KEY")

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/system/stats")
	if err != nil {
		t.Fatalf("failed calling /api/v1/system/stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var data struct {
		Version          string `json:"version"`
		UptimeSeconds    int64  `json:"uptime_seconds"`
		RoutingThreshold *float64 `json:"routing_threshold"`
		Gateway          struct {
			Total         int64   `json:"total"`
			Success       int64   `json:"success"`
			Fail          int64   `json:"fail"`
			SuccessRate   float64 `json:"success_rate"`
			EMALatencyMs  float64 `json:"ema_latency_ms"`
			LastLatencyMs float64 `json:"last_latency_ms"`
			LastError     string  `json:"last_error"`
			LastOKAt      *string `json:"last_ok_at"`
		} `json:"gateway"`
		Jev struct {
			Total         int64   `json:"total"`
			Success       int64   `json:"success"`
			Fail          int64   `json:"fail"`
			SuccessRate   float64 `json:"success_rate"`
			EMALatencyMs  float64 `json:"ema_latency_ms"`
			LastLatencyMs float64 `json:"last_latency_ms"`
			LastError     string  `json:"last_error"`
			LastOKAt      *string `json:"last_ok_at"`
			Model         string  `json:"model"`
			KeyConfigured bool    `json:"key_configured"`
		} `json:"jev"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed decoding response: %v", err)
	}

	if data.Version != "3.0.0-decision-core" {
		t.Errorf("expected version 3.0.0-decision-core, got %s", data.Version)
	}
	if data.Gateway.Total != 2 || data.Gateway.Success != 1 || data.Gateway.Fail != 1 {
		t.Errorf("expected gateway total=2, success=1, fail=1; got %+v", data.Gateway)
	}
	if data.Gateway.SuccessRate != 0.5 {
		t.Errorf("expected gateway success rate 0.5, got %f", data.Gateway.SuccessRate)
	}
	if data.Gateway.LastError != "network timeout" {
		t.Errorf("expected gateway last error 'network timeout', got %q", data.Gateway.LastError)
	}
	if data.Gateway.LastOKAt == nil {
		t.Errorf("expected gateway last_ok_at to be non-nil")
	}

	if data.Jev.Total != 1 || data.Jev.Success != 1 || data.Jev.Fail != 0 {
		t.Errorf("expected jev total=1, success=1, fail=0; got %+v", data.Jev)
	}
	if data.Jev.SuccessRate != 1.0 {
		t.Errorf("expected jev success rate 1.0, got %f", data.Jev.SuccessRate)
	}
	if !data.Jev.KeyConfigured {
		t.Errorf("expected jev key_configured=true when TYPESAFE_API_KEY is set")
	}
	if data.Jev.Model == "" {
		t.Errorf("expected non-empty jev model")
	}
}
