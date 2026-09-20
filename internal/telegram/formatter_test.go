package telegram_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/telegram"
)

func TestEscapeMarkdownV2(t *testing.T) {
	raw := "Warning: BTC/USD! [ALERT] 100% (gain) +2.5% -1.2% {risk=high} #crypto_signals > 0.5 & `test`"
	escaped := telegram.EscapeMarkdownV2(raw)

	// In Telegram MarkdownV2, reserved chars like !, [, ], (, ), +, -, =, {, }, #, _, >, ., `, ~ must be escaped
	mustBeEscaped := []string{`\!`, `\[`, `\]`, `\(`, `\)`, `\+`, `\-`, `\=`, `\{`, `\}`, `\#`, `\_`, `\>`, `\.`, "\\`"}
	for _, target := range mustBeEscaped {
		if !strings.Contains(escaped, target) {
			t.Errorf("expected escaped string to contain '%s', got: %s", target, escaped)
		}
	}
}

func TestFormatSignalEntry(t *testing.T) {
	tp2 := 68500.00
	sig := &db.FuturesTradeSignal{
		ID:                  101,
		Symbol:              "BTC/USDT",
		Direction:           "LONG",
		Status:              "ACTIVE",
		CatalystHeadline:    "SEC approves institutional custody expansion; $450M volume surge.",
		CatalystSource:      "Reuters Financial",
		CatalystSentiment:   0.82,
		EntryPrice:          64200.50,
		StopLoss:            63100.00,
		TakeProfit1:         66400.00,
		TakeProfit2:         &tp2,
		Leverage:            5,
		RiskRewardRatio:     2.00,
		AllocatedCapitalUSD: 1500.00,
		AllocatedCapitalPct: 1.5,
		CreatedAt:           time.Now(),
	}

	msg := telegram.FormatSignalEntry(sig)

	if !strings.Contains(msg, "NEW TWO\\-SIDED FUTURES SIGNAL") {
		t.Errorf("expected header in message, got: %s", msg)
	}
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected symbol in message, got: %s", msg)
	}
	if !strings.Contains(msg, "LONG") {
		t.Errorf("expected direction in message, got: %s", msg)
	}
	if !strings.Contains(msg, "5x") {
		t.Errorf("expected leverage in message, got: %s", msg)
	}
	if !strings.Contains(msg, "Take Profit 2") {
		t.Errorf("expected Take Profit 2 line in message, got: %s", msg)
	}
	if !strings.Contains(msg, "Risk Guard") {
		t.Errorf("expected Risk Guard line in message, got: %s", msg)
	}
}

func TestFormatSignalResolution(t *testing.T) {
	sig := &db.FuturesTradeSignal{
		ID:                  101,
		Symbol:              "ETH/USDT",
		Direction:           "SHORT",
		Status:              "CLOSED",
		EntryPrice:          3450.00,
		StopLoss:            3550.00,
		TakeProfit1:         3250.00,
		Leverage:            4,
		AllocatedCapitalUSD: 2000.00,
		AllocatedCapitalPct: 2.0,
		CreatedAt:           time.Now().Add(-45 * time.Minute),
	}

	msg := telegram.FormatSignalResolution(sig, 3250.00, "TP1", 800.00, 40.00)

	if !strings.Contains(msg, "FUTURES TRADE COMPLETED & RESOLVED") {
		t.Errorf("expected resolution header, got: %s", msg)
	}
	if !strings.Contains(msg, "Target Take Profit 1 Hit") {
		t.Errorf("expected TP1 reason in message, got: %s", msg)
	}
	if !strings.Contains(msg, "+$800\\.00") && !strings.Contains(msg, "$800\\.00") {
		t.Errorf("expected realized PnL in message, got: %s", msg)
	}
	if !strings.Contains(msg, "40.00%") {
		t.Errorf("expected realized ROI in message, got: %s", msg)
	}
}

func TestFormatTestMessage(t *testing.T) {
	msg := telegram.FormatTestMessage("SimpleTraderSignalsBot")
	if !strings.Contains(msg, "TELEGRAM SIGNALS BOT CONNECTED") {
		t.Errorf("expected test header, got: %s", msg)
	}
	if !strings.Contains(msg, "SimpleTraderSignalsBot") {
		t.Errorf("expected bot name in test message, got: %s", msg)
	}
}
