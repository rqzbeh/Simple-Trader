package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/telegram"
)

// TelegramConfigResponse represents safe configuration telemetry for the UI.
type TelegramConfigResponse struct {
	BotTokenConfigured bool   `json:"bot_token_configured"`
	BotTokenMasked     string `json:"bot_token_masked"`
	ChatID             string `json:"chat_id"`
	Enabled            bool   `json:"enabled"`
}

// TelegramConfigRequest represents update payload for Telegram parameters.
type TelegramConfigRequest struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	Enabled  bool   `json:"enabled"`
}

// GetTelegramConfigHandler handles GET /api/v1/telegram/config
func (s *Server) GetTelegramConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.telegramBot == nil {
		json.NewEncoder(w).Encode(TelegramConfigResponse{
			BotTokenConfigured: false,
			BotTokenMasked:     "",
			ChatID:             "",
			Enabled:            false,
		})
		return
	}

	cfg := s.telegramBot.GetConfig()
	tokenMasked := ""
	if len(cfg.BotToken) > 6 {
		tokenMasked = cfg.BotToken[:3] + "..." + cfg.BotToken[len(cfg.BotToken)-3:]
	} else if len(cfg.BotToken) > 0 {
		tokenMasked = "***"
	}

	json.NewEncoder(w).Encode(TelegramConfigResponse{
		BotTokenConfigured: cfg.BotToken != "",
		BotTokenMasked:     tokenMasked,
		ChatID:             cfg.ChatID,
		Enabled:            cfg.Enabled,
	})
}

// UpdateTelegramConfigHandler handles POST /api/v1/telegram/config
func (s *Server) UpdateTelegramConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req TelegramConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json format: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if s.telegramBot == nil {
		s.telegramBot = telegram.NewBotClient(telegram.BotConfig{
			BotToken: req.BotToken,
			ChatID:   req.ChatID,
			Enabled:  req.Enabled,
		})
	} else {
		currentCfg := s.telegramBot.GetConfig()
		newToken := req.BotToken
		if newToken == "" {
			newToken = currentCfg.BotToken
		}
		newChatID := req.ChatID
		if newChatID == "" {
			newChatID = currentCfg.ChatID
		}

		s.telegramBot.UpdateConfig(telegram.BotConfig{
			BotToken: newToken,
			ChatID:   newChatID,
			Enabled:  req.Enabled,
		})
	}

	s.GetTelegramConfigHandler(w, r)
}

// TestTelegramHandler handles POST /api/v1/telegram/test
func (s *Server) TestTelegramHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		BotToken string `json:"bot_token,omitempty"`
		ChatID   string `json:"chat_id,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	token := req.BotToken
	chatID := req.ChatID

	if s.telegramBot != nil {
		cfg := s.telegramBot.GetConfig()
		if token == "" {
			token = cfg.BotToken
		}
		if chatID == "" {
			chatID = cfg.ChatID
		}
	}

	if token == "" || chatID == "" {
		http.Error(w, `{"error":"both bot_token and chat_id are required to execute test"}`, http.StatusBadRequest)
		return
	}

	testClient := telegram.NewBotClient(telegram.BotConfig{
		BotToken: token,
		ChatID:   chatID,
		Enabled:  true,
	})

	testMessage := telegram.FormatTestMessage("Simple-Trader Signal Bot")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err := testClient.SendMessageWithRetry(ctx, testMessage)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "chat not found") {
			errMsg = fmt.Sprintf("Chat not found for chat ID %s. Telegram requires that you initiate a conversation with the bot first: open Telegram, search for @IUST_Trader_Bot, and tap 'Start' or send /start.", chatID)
		}
		http.Error(w, fmt.Sprintf(`{"error":"failed to transmit message via Telegram API: %s"}`, errMsg), http.StatusBadGateway)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Telegram test notification delivered successfully.",
	})
}
