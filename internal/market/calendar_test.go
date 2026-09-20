package market_test

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestEconomicCalendarHalt(t *testing.T) {
	cal := market.NewEconomicCalendar(15 * time.Minute)

	baseTime := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)

	// Add high-impact FOMC event at 14:00 UTC for USD
	cal.AddEvents(market.MacroEvent{
		ID:          "fomc-01",
		Title:       "FOMC Rate Decision",
		Currency:    "USD",
		Impact:      market.ImpactHigh,
		ScheduledAt: baseTime,
	})

	// Add low-impact event for EUR
	cal.AddEvents(market.MacroEvent{
		ID:          "eur-01",
		Title:       "Minor Eurozone Sentiment",
		Currency:    "EUR",
		Impact:      market.ImpactLow,
		ScheduledAt: baseTime,
	})

	// Test 1: Exactly at event time (14:00) -> USD symbol BTC/USD MUST be halted
	halted, reason := cal.IsSymbolHalted("BTC/USD", baseTime)
	if !halted {
		t.Errorf("expected BTC/USD to be halted at event time, reason: %s", reason)
	}

	// Test 2: 10 minutes before event (13:50) -> inside 15min window -> MUST be halted
	haltedPre, _ := cal.IsSymbolHalted("XAU/USD", baseTime.Add(-10*time.Minute))
	if !haltedPre {
		t.Errorf("expected XAU/USD to be halted 10m before FOMC")
	}

	// Test 3: 10 minutes after event (14:10) -> inside 15min window -> MUST be halted
	haltedPost, _ := cal.IsSymbolHalted("EUR/USD", baseTime.Add(10*time.Minute))
	if !haltedPost {
		t.Errorf("expected EUR/USD to be halted 10m after FOMC")
	}

	// Test 4: 20 minutes before event (13:40) -> outside 15min window -> MUST NOT be halted
	haltedOutside, _ := cal.IsSymbolHalted("BTC/USD", baseTime.Add(-20*time.Minute))
	if haltedOutside {
		t.Errorf("expected BTC/USD to NOT be halted 20m before FOMC")
	}

	// Test 5: Low-impact EUR event does NOT halt trading
	cal.ClearEvents()
	cal.AddEvents(market.MacroEvent{
		ID:          "eur-low",
		Title:       "Low Impact EUR Event",
		Currency:    "EUR",
		Impact:      market.ImpactLow,
		ScheduledAt: baseTime,
	})
	haltedLow, _ := cal.IsSymbolHalted("EUR/USD", baseTime)
	if haltedLow {
		t.Errorf("low-impact event should NOT trigger halt")
	}
}

func TestNewsSentimentAnalyzer(t *testing.T) {
	// Bullish news headlines
	bullishHeadlines := []string{
		"Fed signals unexpected rate cut as inflation drops to 2%",
		"Bitcoin ETF inflows surge to new record high",
		"Gold breaks out amidst global liquidity easing",
	}

	report := market.AnalyzeNewsSentiment(bullishHeadlines)
	if report.Polarity != market.PolarityBullish {
		t.Errorf("expected BULLISH polarity, got %s (score=%f)", report.Polarity, report.Score)
	}
	if report.Score <= 0.20 {
		t.Errorf("expected score > 0.20, got %f", report.Score)
	}

	// Bearish news headlines
	bearishHeadlines := []string{
		"SEC files emergency lawsuit, triggering major selloff",
		"Contagion risk rises as crypto lending firm halts withdrawals after major hack",
		"Recession fears escalate as inflation surges higher",
	}

	bearReport := market.AnalyzeNewsSentiment(bearishHeadlines)
	if bearReport.Polarity != market.PolarityBearish {
		t.Errorf("expected BEARISH polarity, got %s (score=%f)", bearReport.Polarity, bearReport.Score)
	}
	if bearReport.Score >= -0.20 {
		t.Errorf("expected score < -0.20, got %f", bearReport.Score)
	}

	// Empty headlines
	emptyReport := market.AnalyzeNewsSentiment(nil)
	if emptyReport.Polarity != market.PolarityNeutral || emptyReport.Score != 0.0 {
		t.Errorf("expected NEUTRAL for empty headlines, got %s (%f)", emptyReport.Polarity, emptyReport.Score)
	}
}
