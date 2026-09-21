package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

const (
	defaultTelegramAPIBase = "https://api.telegram.org"
	maxRetries             = 3
	initialBackoff         = 500 * time.Millisecond
)

// BotConfig encapsulates Telegram connection credentials and operational preferences.
type BotConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	Enabled  bool   `json:"enabled"`
}

// BotClient coordinates communication with Telegram Bot API with backoff and channel queuing.
type BotClient struct {
	mu         sync.RWMutex
	cfg        BotConfig
	httpClient *http.Client
	apiBase    string
	sendQueue  chan string
	stopChan   chan struct{}
	running    bool
}

// NewBotClient initializes a Telegram bot client instance.
func NewBotClient(cfg BotConfig) *BotClient {
	client := &BotClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiBase:   defaultTelegramAPIBase,
		sendQueue: make(chan string, 100),
		stopChan:  make(chan struct{}),
	}

	if cfg.Enabled && cfg.BotToken != "" && cfg.ChatID != "" {
		client.Start()
	}

	return client
}

// UpdateConfig dynamically updates bot token, chat ID, and enabled flag.
func (c *BotClient) UpdateConfig(cfg BotConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg = cfg
	if cfg.Enabled && cfg.BotToken != "" && cfg.ChatID != "" {
		if !c.running {
			c.running = true
			go c.processQueue()
		}
	} else if c.running {
		c.running = false
	}
}

// GetConfig returns the current configuration safe for read.
func (c *BotClient) GetConfig() BotConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// Start spawns background dispatch worker.
func (c *BotClient) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return
	}
	c.running = true
	go c.processQueue()
}

// Stop stops the background dispatch worker.
func (c *BotClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.running = false
	close(c.stopChan)
}

// processQueue drains messages from the queue with rate pacing.
func (c *BotClient) processQueue() {
	for {
		select {
		case <-c.stopChan:
			return
		case msg, ok := <-c.sendQueue:
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := c.SendMessageWithRetry(ctx, msg)
			if err != nil {
				log.Printf("[telegram] ERROR delivering message: %v", err)
			} else {
				log.Printf("[telegram] successfully delivered message to chat")
			}
			cancel()
			// Telegram rate limit guideline: ~1 message/sec per group
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// SendMessage enqueues a MarkdownV2 message for asynchronous dispatch.
func (c *BotClient) SendMessage(message string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.cfg.Enabled || c.cfg.BotToken == "" || c.cfg.ChatID == "" {
		return
	}

	select {
	case c.sendQueue <- message:
	default:
		log.Printf("[telegram] warning: sendQueue full, dropping message")
	}
}

// SendMessageWithRetry attempts immediate message transmission with exponential backoff.
func (c *BotClient) SendMessageWithRetry(ctx context.Context, text string) error {
	c.mu.RLock()
	token := c.cfg.BotToken
	chatID := c.cfg.ChatID
	c.mu.RUnlock()

	if token == "" || chatID == "" {
		return errors.New("telegram bot token or chat ID is empty")
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", c.apiBase, token)
	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "MarkdownV2",
		"disable_web_page_preview": true,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	var lastErr error
	backoff := initialBackoff

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		lastErr = fmt.Errorf("telegram api error (status %d): %s", resp.StatusCode, string(respBytes))
		if resp.StatusCode == http.StatusBadRequest {
			log.Printf("[telegram] entity parse error (status 400: %s), retrying as plain text fallback...", string(respBytes))
			fallbackPayload := map[string]interface{}{
				"chat_id":                  chatID,
				"text":                     StripMarkdownV2(text),
				"disable_web_page_preview": true,
			}
			fbBytes, err := json.Marshal(fallbackPayload)
			if err == nil {
				fbReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(fbBytes))
				if err == nil {
					fbReq.Header.Set("Content-Type", "application/json")
					fbResp, err := c.httpClient.Do(fbReq)
					if err == nil {
						defer fbResp.Body.Close()
						if fbResp.StatusCode == http.StatusOK {
							log.Printf("[telegram] plain text fallback delivery succeeded!")
							return nil
						}
						fbRespBytes, _ := io.ReadAll(fbResp.Body)
						log.Printf("[telegram] plain text fallback failed (status %d): %s", fbResp.StatusCode, string(fbRespBytes))
					}
				}
			}
			return lastErr
		}

		time.Sleep(backoff)
		backoff *= 2
	}

	return fmt.Errorf("telegram send failed after %d attempts: %w", maxRetries, lastErr)
}

// BroadcastSignalEntry queues a new trade signal notification.
func (c *BotClient) BroadcastSignalEntry(sig *db.FuturesTradeSignal) {
	msg := FormatSignalEntry(sig)
	if msg != "" {
		c.SendMessage(msg)
	}
}

// BroadcastSignalResolution queues a completed trade closure report with realized ROI.
func (c *BotClient) BroadcastSignalResolution(sig *db.FuturesTradeSignal, exitPrice float64, exitReason string, pnl, roi float64) {
	msg := FormatSignalResolution(sig, exitPrice, exitReason, pnl, roi)
	if msg != "" {
		c.SendMessage(msg)
	}
}
