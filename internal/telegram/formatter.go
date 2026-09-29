package telegram

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// FormatPrice renders a token price with precision safe for micro-cap assets.
// Fixed 2-decimal formatting collapses prices like 0.000002334544 to "0.00",
// making entry, stop loss and take profit indistinguishable in the alert.
//
// Rules:
//   - price >= 100: 2 decimals (BTC-scale)
//   - 1 <= price < 100: 4 decimals
//   - price < 1: enough decimals for ~7 significant digits (capped at 18)
func FormatPrice(v float64) string {
	switch {
	case v == 0:
		return "0.00"
	case v >= 100:
		return fmt.Sprintf("%.2f", v)
	case v >= 1:
		return fmt.Sprintf("%.4f", v)
	default:
		// Digits after the decimal point so ~7 significant digits survive.
		prec := int(math.Ceil(-math.Log10(v))) + 6
		if prec > 18 {
			prec = 18
		}
		s := fmt.Sprintf("%.*f", prec, v)
		// Drop padding zeros the precision estimate may add; keep >=2 decimals.
		if i := strings.IndexByte(s, '.'); i >= 0 && len(s) > i+3 {
			if t := strings.TrimRight(s, "0"); len(t) > i+3 {
				s = t
			}
		}
		return s
	}
}

// EscapeMarkdownV2 escapes characters reserved in Telegram MarkdownV2 format:
// '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!'
func EscapeMarkdownV2(text string) string {
	reserved := []string{"\\", "_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, r := range reserved {
		result = strings.ReplaceAll(result, r, "\\"+r)
	}
	return result
}

// FormatSignalEntry produces an institutional-grade Telegram MarkdownV2 alert card for new trade signals.
func FormatSignalEntry(sig *db.FuturesTradeSignal) string {
	if sig == nil {
		return ""
	}

	dirEmoji := "🟢"
	if sig.Direction == "SHORT" {
		dirEmoji = "🔴"
	}

	// spec-021: no fabricated catalyst text — when the signal carries no real
	// headline, the catalyst block is omitted entirely.
	catalystBlock := ""
	if sig.CatalystHeadline != "" {
		srcLine := ""
		if sig.CatalystSource != "" {
			srcLine = fmt.Sprintf("🗞️ *Source:* %s  •  ", EscapeMarkdownV2(sig.CatalystSource))
		}
		catalystBlock = fmt.Sprintf(
			"📰 *Primary News Catalyst:*\n_%s_\n%s*Sentiment:* %s\n\n",
			EscapeMarkdownV2(sig.CatalystHeadline),
			srcLine,
			EscapeMarkdownV2(fmt.Sprintf("%+.2f", sig.CatalystSentiment)),
		)
	}

	tp2Line := ""
	if sig.TakeProfit2 != nil && *sig.TakeProfit2 > 0 {
		tp2Line = fmt.Sprintf("\n🎯 *Take Profit 2:* $%s", EscapeMarkdownV2(FormatPrice(*sig.TakeProfit2)))
	}

	// spec-020 FR-705: show the REAL stored timeframe — the legacy hardcoded
	// horizon label claimed two hours on 15m signals. Legacy rows without a
	// timeframe omit the claim instead of guessing.
	tfLine := ""
	if sig.Timeframe != nil && *sig.Timeframe != "" {
		tfLine = fmt.Sprintf("*Timeframe:* *%s*\n", EscapeMarkdownV2(*sig.Timeframe))
	}

	rrFormatted := EscapeMarkdownV2(fmt.Sprintf("1:%.2f", sig.RiskRewardRatio))
	equityPctFormatted := EscapeMarkdownV2(fmt.Sprintf("%.2f%%", sig.AllocatedCapitalPct))

	return fmt.Sprintf(
		"🚨 *NEW TWO\\-SIDED FUTURES SIGNAL* 🚨\n\n"+
			"*Asset:* `%s`\n"+
			"*Direction:* %s *%s*\n"+
			"%s"+
			"*Isolated Leverage:* *%dx*\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📍 *Entry Price:* $%s\n"+
			"🛡️ *Stop Loss:* $%s\n"+
			"🎯 *Take Profit 1:* $%s%s\n"+
			"⚖️ *Risk / Reward:* *%s*\n"+
			"💵 *Capital Allocation:* $%s \\(%s Equity\\)\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"%s"+
			"⚠️ *Risk Guard:* min R:R floor enforced with stop-loss protection\\.",
		EscapeMarkdownV2(sig.Symbol),
		dirEmoji,
		EscapeMarkdownV2(sig.Direction),
		tfLine,
		sig.Leverage,
		EscapeMarkdownV2(FormatPrice(sig.EntryPrice)),
		EscapeMarkdownV2(FormatPrice(sig.StopLoss)),
		EscapeMarkdownV2(FormatPrice(sig.TakeProfit1)),
		tp2Line,
		rrFormatted,
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.AllocatedCapitalUSD)),
		equityPctFormatted,
		catalystBlock,
	)
}

// FormatSignalResolution generates a Telegram MarkdownV2 closure report detailing exit price, reason, and net ROI %.
func FormatSignalResolution(sig *db.FuturesTradeSignal, exitPrice float64, exitReason string, pnlUSD, roiPct float64) string {
	if sig == nil {
		return ""
	}

	dirEmoji := "🟢"
	if sig.Direction == "SHORT" {
		dirEmoji = "🔴"
	}

	outcomeEmoji := "🏆"
	if pnlUSD < 0 {
		outcomeEmoji = "🛑"
	}

	reasonLabel := exitReason
	switch exitReason {
	case "TP1":
		reasonLabel = "🎯 Target Take Profit 1 Hit"
	case "TAKE_PROFIT":
		reasonLabel = "🎯 Target Take Profit Hit"
	case "TP2":
		reasonLabel = "🎯 Target Take Profit 2 Hit"
	case "SL", "STOP_LOSS":
		reasonLabel = "🛡️ Protective Stop Loss Triggered"
	case "MANUAL", "MANUAL_EXIT":
		reasonLabel = "⚙️ Manual Position Resolution"
	default:
		reasonLabel = EscapeMarkdownV2(exitReason)
	}

	// Calculate hold duration if exit time and created_at are available
	durationStr := "N/A"
	if !sig.CreatedAt.IsZero() {
		dur := time.Since(sig.CreatedAt).Round(time.Minute)
		if dur < time.Hour {
			durationStr = fmt.Sprintf("%dm", int(dur.Minutes()))
		} else {
			durationStr = fmt.Sprintf("%dh %dm", int(dur.Hours()), int(dur.Minutes())%60)
		}
	}

	signPnL := "\\+"
	if pnlUSD < 0 {
		signPnL = "\\-"
	}
	absPnL := pnlUSD
	if absPnL < 0 {
		absPnL = -absPnL
	}

	signROI := "\\+"
	if roiPct < 0 {
		signROI = "\\-"
	}
	absROI := roiPct
	if absROI < 0 {
		absROI = -absROI
	}

	return fmt.Sprintf(
		"%s *FUTURES TRADE COMPLETED & RESOLVED* %s\n\n"+
			"*Asset:* `%s`\n"+
			"*Direction:* %s *%s* \\(%dx Isolated\\)\n"+
			"*Outcome:* *%s*\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📍 *Entry Price:* $%s\n"+
			"🏁 *Exit Price:* $%s\n"+
			"⏱️ *Hold Duration:* %s\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"💰 *Realized Net PnL:* *%s$%s*\n"+
			"📈 *Realized ROI:* *%s%s*\n\n"+
			"✅ *Settled directly to Portfolio Liquid Capital\\.*",
		outcomeEmoji,
		outcomeEmoji,
		EscapeMarkdownV2(sig.Symbol),
		dirEmoji,
		EscapeMarkdownV2(sig.Direction),
		sig.Leverage,
		EscapeMarkdownV2(reasonLabel),
		EscapeMarkdownV2(FormatPrice(sig.EntryPrice)),
		EscapeMarkdownV2(FormatPrice(exitPrice)),
		EscapeMarkdownV2(durationStr),
		signPnL,
		EscapeMarkdownV2(fmt.Sprintf("%.2f", absPnL)),
		signROI,
		EscapeMarkdownV2(fmt.Sprintf("%.2f%%", absROI)),
	)
}

// StripMarkdownV2 removes Markdown formatting and backslashes for plain-text fallback.
func StripMarkdownV2(s string) string {
	reserved := []string{"\\", "_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := s
	for _, r := range reserved {
		result = strings.ReplaceAll(result, "\\"+r, r)
	}
	styling := []string{"*", "_", "`", "~"}
	for _, st := range styling {
		result = strings.ReplaceAll(result, st, "")
	}
	return result
}

// FormatTestMessage generates an initial verification handshake message for Telegram Bot API verification.
func FormatTestMessage(botName string) string {
	name := botName
	if name == "" {
		name = "Simple-Trader Signals"
	}
	return fmt.Sprintf(
		"🤖 *TELEGRAM SIGNALS BOT CONNECTED*\n\n"+
			"Bot Name: `%s`\n"+
			"Status: *Active & Operational*\n"+
			"Parsed Protocol: *Telegram MarkdownV2*\n\n"+
			"Institutional two\\-sided futures signals \\(LONG/SHORT\\), catalyst alerts, and ROI resolution reports will be autonomously broadcasted to this channel\\.",
		EscapeMarkdownV2(name),
	)
}

// Sender defines the contract for dispatching Telegram messages with retry (FR-103).
// *BotClient satisfies this interface.
type Sender interface {
	SendMessageWithRetry(ctx context.Context, text string) error
}

// EarlyExitMessage holds fields for early exit notification per spec-014 contracts §4.
type EarlyExitMessage struct {
	Symbol     string
	Direction  string
	Cluster    string
	Confidence float64
	Route      string
	PnLUSD     float64
	ReturnPct  float64
	ExitReason string
}

// FormatEarlyExit produces a Telegram MarkdownV2 alert for news-driven early trade exits.
// Contracts §4:
// 🔴 EARLY EXIT — NEWS
// Symbol: BTC/USDT (closed LONG)
// Reason: SEC sues exchange... [cluster]
// Confidence: 0.82 · route: jev_direct
// PnL: +$142.10 (+1.4%) · exit: NEWS_EARLY_EXIT
func FormatEarlyExit(msg EarlyExitMessage) string {
	if msg.ExitReason == "" {
		msg.ExitReason = "NEWS_EARLY_EXIT"
	}
	signPnL := "\\+"
	if msg.PnLUSD < 0 {
		signPnL = "\\-"
	}
	absPnL := math.Abs(msg.PnLUSD)

	signPct := "\\+"
	if msg.ReturnPct < 0 {
		signPct = "\\-"
	}
	absPct := math.Abs(msg.ReturnPct)

	return fmt.Sprintf(
		"🔴 *EARLY EXIT — NEWS*\n\n"+
			"*Symbol:* `%s` \\(closed %s\\)\n"+
			"*Reason:* %s\n"+
			"*Confidence:* %s · *route:* %s\n"+
			"*PnL:* %s$%s \\(%s%s%%\\) · *exit:* %s",
		EscapeMarkdownV2(msg.Symbol),
		EscapeMarkdownV2(msg.Direction),
		EscapeMarkdownV2(msg.Cluster),
		EscapeMarkdownV2(fmt.Sprintf("%.2f", msg.Confidence)),
		EscapeMarkdownV2(msg.Route),
		signPnL,
		EscapeMarkdownV2(fmt.Sprintf("%.2f", absPnL)),
		signPct,
		EscapeMarkdownV2(fmt.Sprintf("%.1f", absPct)),
		EscapeMarkdownV2(msg.ExitReason),
	)
}
