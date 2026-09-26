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
	if !strings.Contains(msg, "\\+$800\\.00") && !strings.Contains(msg, "$800\\.00") {
		t.Errorf("expected realized PnL in message, got: %s", msg)
	}
	if !strings.Contains(msg, "40\\.00%") {
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

func TestStripMarkdownV2(t *testing.T) {
	formatted := "*Asset:* `BTC/USDT`\\n\\*Take Profit 1:\\* $66400\\.00\\n"
	plain := telegram.StripMarkdownV2(formatted)
	if strings.Contains(plain, "*") || strings.Contains(plain, "`") || strings.Contains(plain, "\\.") {
		t.Errorf("expected stripped plain text without markdown markers, got: %s", plain)
	}
}

func TestFormatPrice(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"zero", 0, "0.00"},
		{"btc-scale keeps 2dp", 43250.67, "43250.67"},
		{"large caps 2dp", 1234.5, "1234.50"},
		{"mid price 4dp", 12.34567, "12.3457"},
		{"stablecoin 4dp", 1.0001, "1.0001"},
		{"sub-dollar keeps precision", 0.0512345, "0.0512345"},
		// Regression: 2dp collapsed this to "0.00" — entry, SL and TP
		// rendered identically in the Telegram alert.
		{"micro-cap not collapsed", 0.000002334544, "0.000002334544"},
		{"micro-cap derived stop", 0.000001987654, "0.000001987654"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := telegram.FormatPrice(tc.in)
			if got != tc.want {
				t.Errorf("FormatPrice(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Micro-cap entry alert must show three DISTINCT prices: a 2-decimal format
// rendered entry, stop loss and TP1 all as "0.00".
func TestFormatSignalEntryMicroCapDistinctPrices(t *testing.T) {
	sig := &db.FuturesTradeSignal{
		Symbol:              "PEPE/USDT",
		Direction:           "LONG",
		Leverage:            8,
		EntryPrice:          0.000002334544,
		StopLoss:            0.000002286200,
		TakeProfit1:         0.000002684726,
		RiskRewardRatio:     2.5,
		AllocatedCapitalUSD: 40,
		AllocatedCapitalPct: 40,
		CatalystHeadline:    "Whale accumulation spike",
		CatalystSource:      "Wire",
	}
	msg := telegram.FormatSignalEntry(sig)

	prices := []string{
		telegram.FormatPrice(sig.EntryPrice),
		telegram.FormatPrice(sig.StopLoss),
		telegram.FormatPrice(sig.TakeProfit1),
	}
	for i, p := range prices {
		if p == "0.00" || p == "" {
			t.Fatalf("price %d collapsed: %q", i, p)
		}
		if !strings.Contains(msg, telegram.EscapeMarkdownV2(p)) {
			t.Errorf("message missing price %q", p)
		}
	}
	if prices[0] == prices[1] || prices[0] == prices[2] || prices[1] == prices[2] {
		t.Errorf("entry/SL/TP1 not distinct: %v", prices)
	}
}
