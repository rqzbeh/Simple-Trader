package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

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

	headline := sig.CatalystHeadline
	if headline == "" {
		headline = "Breaking Macro Catalyst Disclosed"
	}
	source := sig.CatalystSource
	if source == "" {
		source = "News Aggregator"
	}

	tp2Line := ""
	if sig.TakeProfit2 != nil && *sig.TakeProfit2 > 0 {
		tp2Line = fmt.Sprintf("\n🎯 *Take Profit 2:* $%s", EscapeMarkdownV2(fmt.Sprintf("%.2f", *sig.TakeProfit2)))
	}

	rrFormatted := EscapeMarkdownV2(fmt.Sprintf("1:%.2f", sig.RiskRewardRatio))
	equityPctFormatted := EscapeMarkdownV2(fmt.Sprintf("%.2f%%", sig.AllocatedCapitalPct))

	return fmt.Sprintf(
		"🚨 *NEW TWO\\-SIDED FUTURES SIGNAL* 🚨\n\n"+
			"*Asset:* `%s`\n"+
			"*Direction:* %s *%s*\n"+
			"*Isolated Leverage:* *%dx*\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📍 *Entry Price:* $%s\n"+
			"🛡️ *Stop Loss:* $%s\n"+
			"🎯 *Take Profit 1:* $%s%s\n"+
			"⚖️ *Risk / Reward:* *%s*\n"+
			"💵 *Capital Allocation:* $%s \\(%s Equity\\)\n"+
			"━━━━━━━━━━━━━━━━━━━━\n"+
			"📰 *Primary News Catalyst:*\n"+
			"_%s_\n"+
			"🗞️ *Source:* %s  •  *Sentiment:* %s\n\n"+
			"⚠️ *Risk Guard:* Max 2\\.0%% capital loss risk strictly enforced\\.",
		EscapeMarkdownV2(sig.Symbol),
		dirEmoji,
		EscapeMarkdownV2(sig.Direction),
		sig.Leverage,
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.EntryPrice)),
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.StopLoss)),
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.TakeProfit1)),
		tp2Line,
		rrFormatted,
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.AllocatedCapitalUSD)),
		equityPctFormatted,
		EscapeMarkdownV2(headline),
		EscapeMarkdownV2(source),
		EscapeMarkdownV2(fmt.Sprintf("%+.2f", sig.CatalystSentiment)),
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
	case "TP2":
		reasonLabel = "🎯 Target Take Profit 2 Hit"
	case "SL":
		reasonLabel = "🛡️ Protective Stop Loss Triggered"
	case "MANUAL":
		reasonLabel = "⚙️ Manual Position Resolution"
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
		EscapeMarkdownV2(fmt.Sprintf("%.2f", sig.EntryPrice)),
		EscapeMarkdownV2(fmt.Sprintf("%.2f", exitPrice)),
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
