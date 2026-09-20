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

func TestAuthEndpoints(t *testing.T) {
	cfg := &config.Config{
		AdminPassword: "SuperSecretAdminKey!2026",
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	router := srv.Router()

	// 1. Initial session check (unauthenticated)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var sessionResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&sessionResp); err != nil {
		t.Fatalf("failed to decode session response: %v", err)
	}
	if sessionResp["authenticated"] != false {
		t.Errorf("expected authenticated to be false")
	}

	// 2. Failed login attempt
	badPayload, _ := json.Marshal(map[string]string{"password": "WrongPassword"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(badPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for wrong password, got %d", rec.Code)
	}

	// 3. Successful login
	goodPayload, _ := json.Marshal(map[string]string{"password": cfg.AdminPassword})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(goodPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for correct login, got %d", rec.Code)
	}

	var loginResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	token, ok := loginResp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected non-empty token in login response")
	}

	// Check cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "simple_trader_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value != token {
		t.Errorf("expected session cookie with token value")
	}

	// 4. Session check with token in Authorization header
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	sessionResp = make(map[string]interface{})
	_ = json.NewDecoder(rec.Body).Decode(&sessionResp)
	if sessionResp["authenticated"] != true {
		t.Errorf("expected authenticated to be true with valid token")
	}

	// 5. Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 on logout, got %d", rec.Code)
	}

	// 6. Session check after logout
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	sessionResp = make(map[string]interface{})
	_ = json.NewDecoder(rec.Body).Decode(&sessionResp)
	if sessionResp["authenticated"] != false {
		t.Errorf("expected authenticated to be false after logout")
	}
}
