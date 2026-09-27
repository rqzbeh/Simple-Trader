package market_test

import (
	"context"
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

func TestNewsClassifierContract(t *testing.T) {
	// Spec-013 v3.0: no lexicon — default classifier must fail explicitly
	// and never fabricate a polarity.
	_, err := market.DefaultClassifier([]string{"Bitcoin ETF inflows surge to new record high"})
	if err == nil {
		t.Fatal("expected explicit error from unconfigured classifier, got nil")
	}
	if _, ok := err.(market.ClassifierNotConfiguredError); !ok {
		t.Fatalf("expected ClassifierNotConfiguredError, got %T: %v", err, err)
	}
}

func TestLiveMacroEventsFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := market.FetchLiveMacroEvents(ctx, "")
	if err != nil {
		t.Skipf("Live calendar fetch skipped (network restricted): %v", err)
		return
	}

	if len(events) == 0 {
		t.Fatalf("expected at least 1 macro event from live institutional feed, got 0")
	}

	t.Logf("Successfully fetched %d authentic macro events from ForexFactory feed", len(events))
	first := events[0]
	t.Logf("Sample event: [%s] %s (%s) scheduled at %v", first.Impact, first.Title, first.Currency, first.ScheduledAt)
}
