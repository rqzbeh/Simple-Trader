package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/server"
)

func TestShadowReportEndpoint_NewsExitType(t *testing.T) {
	cfg := &config.Config{
		AdminPassword: "AdminSecret2026!",
	}
	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	router := srv.Router()

	// 1. Invalid type -> 400
	req := httptest.NewRequest(http.MethodGet, "/api/admin/shadow/report?type=invalid_type", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// Without auth, it checks auth first -> 401
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}

	// Login to get token
	goodPayload, _ := json.Marshal(map[string]string{"password": cfg.AdminPassword})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(goodPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginRec.Code)
	}
	var loginResp map[string]interface{}
	_ = json.NewDecoder(loginRec.Body).Decode(&loginResp)
	token, _ := loginResp["token"].(string)

	// Authenticated request with invalid type -> 400
	req = httptest.NewRequest(http.MethodGet, "/api/admin/shadow/report?type=invalid_type", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d", rec.Code)
	}

	// Authenticated request with type=news_exit -> should be accepted (not 400)
	// Because dbStore is nil, it will return 503 "database unavailable"
	req = httptest.NewRequest(http.MethodGet, "/api/admin/shadow/report?type=news_exit&days=14", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("expected type=news_exit to be valid, got 400: %s", rec.Body.String())
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when database is unavailable, got %d", rec.Code)
	}
}

func TestShadowReportEndpoint_ParamsType(t *testing.T) {
	cfg := &config.Config{
		AdminPassword: "AdminSecret2026!",
	}
	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	router := srv.Router()

	// Login to get token
	goodPayload, _ := json.Marshal(map[string]string{"password": cfg.AdminPassword})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(goodPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginRec.Code)
	}
	var loginResp map[string]interface{}
	_ = json.NewDecoder(loginRec.Body).Decode(&loginResp)
	token, _ := loginResp["token"].(string)

	// Authenticated request with type=params -> should be accepted (not 400)
	// Because dbStore is nil, it will return 503 "database unavailable"
	req := httptest.NewRequest(http.MethodGet, "/api/admin/shadow/report?type=params&days=14", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("expected type=params to be valid, got 400: %s", rec.Body.String())
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when database is unavailable, got %d", rec.Code)
	}
}
