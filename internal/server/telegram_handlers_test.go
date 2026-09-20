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

func TestTelegramConfigHandlers(t *testing.T) {
	cfg := &config.Config{
		TelegramBotToken: "123456789:ABCDefghIJKLmnOPQRstuvwxyz",
		TelegramChatID:   "987654321",
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	r := srv.Router()

	// 1. GET /api/v1/telegram/config
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telegram/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp server.TelegramConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.BotTokenConfigured {
		t.Errorf("expected bot_token_configured to be true")
	}
	if resp.ChatID != "987654321" {
		t.Errorf("expected chat_id '987654321', got '%s'", resp.ChatID)
	}
	if resp.BotTokenMasked != "123...xyz" {
		t.Errorf("expected masked token '123...xyz', got '%s'", resp.BotTokenMasked)
	}

	// 2. POST /api/v1/telegram/config (Update configuration)
	updatePayload := map[string]interface{}{
		"bot_token": "999888777:NEWTokenForTesting123",
		"chat_id":   "555444333",
		"enabled":   true,
	}
	body, _ := json.Marshal(updatePayload)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/config", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200 on update, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 server.TelegramConfigResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("failed to decode updated response: %v", err)
	}

	if resp2.ChatID != "555444333" {
		t.Errorf("expected updated chat_id '555444333', got '%s'", resp2.ChatID)
	}
}
