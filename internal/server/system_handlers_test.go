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

	// 7. Test EARLY_EXIT_* keys (spec-014 contracts §5)
	// Invalid EARLY_EXIT_CONF_FLOOR -> 400
	invalidFloorPayload, _ := json.Marshal(map[string]interface{}{"EARLY_EXIT_CONF_FLOOR": 1.5})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(invalidFloorPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for EARLY_EXIT_CONF_FLOOR > 1.0, got %d", rec.Code)
	}

	// Invalid EARLY_EXIT_MAX_PER_DAY -> 400
	invalidBudgetPayload, _ := json.Marshal(map[string]interface{}{"EARLY_EXIT_MAX_PER_DAY": 0})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(invalidBudgetPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for EARLY_EXIT_MAX_PER_DAY < 1, got %d", rec.Code)
	}

	// Valid EARLY_EXIT update -> 200
	validEarlyExitPayload, _ := json.Marshal(map[string]interface{}{
		"EARLY_EXIT_ENABLED":      false,
		"EARLY_EXIT_MIN_HOLD_MIN": 45,
		"EARLY_EXIT_MAX_PER_DAY":  5,
		"EARLY_EXIT_COOLDOWN_MIN": 90,
		"EARLY_EXIT_CONF_FLOOR":   0.80,
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(validEarlyExitPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid EARLY_EXIT update, got %d, body: %s", rec.Code, rec.Body.String())
	}

	if cfg.EarlyExit.Enabled != false {
		t.Errorf("expected cfg.EarlyExit.Enabled=false, got %v", cfg.EarlyExit.Enabled)
	}
	if cfg.EarlyExit.MinHoldMin != 45 {
		t.Errorf("expected cfg.EarlyExit.MinHoldMin=45, got %d", cfg.EarlyExit.MinHoldMin)
	}
	if cfg.EarlyExit.MaxPerDay != 5 {
		t.Errorf("expected cfg.EarlyExit.MaxPerDay=5, got %d", cfg.EarlyExit.MaxPerDay)
	}
	if cfg.EarlyExit.CooldownMin != 90 {
		t.Errorf("expected cfg.EarlyExit.CooldownMin=90, got %d", cfg.EarlyExit.CooldownMin)
	}
	if cfg.EarlyExit.ConfFloor != 0.80 {
		t.Errorf("expected cfg.EarlyExit.ConfFloor=0.80, got %f", cfg.EarlyExit.ConfFloor)
	}

	// 8. Test Managed Parameter Overrides (spec-015 contracts §4 and §5)
	// (a) Verify GET returns parameter_modes map
	req = httptest.NewRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET config, got %d", rec.Code)
	}
	var cfgResp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&cfgResp)
	paramModes, ok := cfgResp["parameter_modes"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameter_modes map in GET config response, got %v", cfgResp["parameter_modes"])
	}
	for _, k := range []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"} {
		if _, exists := paramModes[k]; !exists {
			t.Errorf("expected key %q in parameter_modes map", k)
		}
	}

	// (b) PUT valid overrides for all 7 keys
	overridePayload, _ := json.Marshal(map[string]interface{}{
		"MIN_RISK_TO_REWARD_RATIO": 3.0,
		"DEFAULT_LEVERAGE":         10,
		"MAX_RISK_PER_TRADE_PCT":   0.015,
		"SL_ATR_MULT":              1.8,
		"TP_ATR_MULT":              3.6,
		"CLUSTER_DECAY_MODE":       "MACRO_THEMATIC",
		"CONFLUENCE_MIN":           0.70,
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(overridePayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for override PUT, got %d, body: %s", rec.Code, rec.Body.String())
	}

	// (c) PUT empty string to clear overrides back to core_managed
	clearPayload, _ := json.Marshal(map[string]interface{}{
		"MIN_RISK_TO_REWARD_RATIO": "",
		"DEFAULT_LEVERAGE":         "",
		"MAX_RISK_PER_TRADE_PCT":   "",
		"SL_ATR_MULT":              "",
		"TP_ATR_MULT":              "",
		"CLUSTER_DECAY_MODE":       "",
		"CONFLUENCE_MIN":           "",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/config", bytes.NewReader(clearPayload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for clear PUT, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var clearedResp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&clearedResp)
	clearedModes, ok := clearedResp["parameter_modes"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameter_modes in response after clear, got %v", clearedResp["parameter_modes"])
	}
	for _, k := range []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"} {
		item, ok := clearedModes[k].(map[string]interface{})
		if !ok || item["mode"] != "core_managed" {
			t.Errorf("expected param %q mode to be core_managed after clear, got %v", k, item)
		}
	}
}
