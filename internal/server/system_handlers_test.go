package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestSystemConfigEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	envPath := tempDir + "/.env"
	_ = os.WriteFile(envPath, []byte("ROUTING_CONFIDENCE_THRESHOLD=0.75\nDEFAULT_LEVERAGE=8\n"), 0600)
	t.Setenv("ENV_FILE", envPath)
	t.Setenv("TYPESAFE_API_KEY", "apikey_typesafe_secret_123456789")

	cfg := &config.Config{
		AdminPassword:              "AdminSecret2026!",
		RoutingConfidenceThreshold: "0.75",
		DefaultLeverage:            8,
		TypesafeAPIKey:             "apikey_typesafe_secret_123456789",
		AIAPIKey:                   "sk-ai-secret-key-123456789",
		AIModelID:                  "test-model",
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	router := srv.Router()

	// 1. GET /api/v1/system/config without auth (should work, masked telemetry)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", rec.Code)
	}

	var getResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode GET response failed: %v", err)
	}

	if getResp["typesafe_api_key_configured"] != true {
		t.Errorf("expected typesafe_api_key_configured=true")
	}
	if getResp["typesafe_base_url"] != "https://api.typesafe.ai" {
		t.Errorf("expected typesafe_base_url=https://api.typesafe.ai, got %v", getResp["typesafe_base_url"])
	}
	if getResp["env_file"] != envPath {
		t.Errorf("expected env_file=%s, got %v", envPath, getResp["env_file"])
	}
	// Verify keys are masked
	if strings.Contains(fmt.Sprintf("%v", getResp["typesafe_api_key_masked"]), "secret") {
		t.Errorf("typesafe key leaked in response: %v", getResp["typesafe_api_key_masked"])
	}
	if strings.Contains(fmt.Sprintf("%v", getResp["ai_api_key_masked"]), "secret") {
		t.Errorf("ai key leaked in response: %v", getResp["ai_api_key_masked"])
	}

	// 2. PUT /api/v1/system/config without auth -> expect 401
	putPayload, _ := json.Marshal(map[string]string{"ROUTING_CONFIDENCE_THRESHOLD": "0.85"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(putPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected PUT without auth to return 401, got %d", rec.Code)
	}

	// 3. Login to obtain session token
	loginPayload, _ := json.Marshal(map[string]string{"password": "AdminSecret2026!"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", rec.Code)
	}
	var loginResp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&loginResp)
	token, _ := loginResp["token"].(string)
	if token == "" {
		t.Fatalf("expected token from login, got empty")
	}

	// 4. PUT with invalid key -> expect 400
	disallowedPayload, _ := json.Marshal(map[string]string{"INVALID_KEY": "foo"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(disallowedPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disallowed key, got %d", rec.Code)
	}

	// 5. PUT with invalid range -> expect 400
	invalidRangePayload, _ := json.Marshal(map[string]interface{}{"ROUTING_CONFIDENCE_THRESHOLD": 1.5})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(invalidRangePayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out of range threshold, got %d", rec.Code)
	}

	// 6. PUT with valid authorized payload
	validPayload, _ := json.Marshal(map[string]interface{}{
		"ROUTING_CONFIDENCE_THRESHOLD": "0.85",
		"DEFAULT_LEVERAGE":             12,
		"TYPESAFE_API_KEY":             "***", // masked placeholder, should not overwrite
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(validPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid PUT, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var putResp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&putResp)
	if putResp["default_leverage"] != float64(12) {
		t.Errorf("expected default_leverage=12, got %v", putResp["default_leverage"])
	}
	if putResp["routing_confidence_threshold"] != 0.85 {
		t.Errorf("expected routing_confidence_threshold=0.85, got %v", putResp["routing_confidence_threshold"])
	}

	// Verify .env file on disk was updated
	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("failed reading env file: %v", err)
	}
	envContent := string(envBytes)
	if !strings.Contains(envContent, "ROUTING_CONFIDENCE_THRESHOLD=0.85") {
		t.Errorf("expected ROUTING_CONFIDENCE_THRESHOLD=0.85 in file, got:\n%s", envContent)
	}
	if !strings.Contains(envContent, "DEFAULT_LEVERAGE=12") {
		t.Errorf("expected DEFAULT_LEVERAGE=12 in file, got:\n%s", envContent)
	}
	// Verify TYPESAFE_API_KEY was not overwritten with "***"
	if strings.Contains(envContent, "TYPESAFE_API_KEY=***") {
		t.Errorf("TYPESAFE_API_KEY was overwritten with masked placeholder!")
	}
	// Verify in-memory config updated
	if cfg.DefaultLeverage != 12 {
		t.Errorf("in-memory cfg.DefaultLeverage not updated, got %d", cfg.DefaultLeverage)
	}
	if cfg.RoutingConfidenceThreshold != "0.85" {
		t.Errorf("in-memory cfg.RoutingConfidenceThreshold not updated, got %s", cfg.RoutingConfidenceThreshold)
	}
}
