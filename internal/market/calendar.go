package market

import (
	"strings"
	"sync"
	"time"
)

// ImpactLevel represents economic event volatility severity.
type ImpactLevel string

const (
	ImpactLow    ImpactLevel = "LOW"
	ImpactMedium ImpactLevel = "MEDIUM"
	ImpactHigh   ImpactLevel = "HIGH"
)

// MacroEvent represents a high-impact macroeconomic calendar release.
type MacroEvent struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Currency    string      `json:"currency"` // USD, EUR, etc.
	Impact      ImpactLevel `json:"impact"`
	ScheduledAt time.Time   `json:"scheduled_at"`
	Actual      string      `json:"actual,omitempty"`
	Forecast    string      `json:"forecast,omitempty"`
	Previous    string      `json:"previous,omitempty"`
}

// EconomicCalendar manages upcoming high-impact economic releases and enforces trade halts.
type EconomicCalendar struct {
	mu           sync.RWMutex
	events       []MacroEvent
	haltWindow   time.Duration // typically 15 minutes
}

// NewEconomicCalendar initializes the calendar tracker.
func NewEconomicCalendar(haltWindow time.Duration) *EconomicCalendar {
	if haltWindow <= 0 {
		haltWindow = 15 * time.Minute
	}
	return &EconomicCalendar{
		events:     make([]MacroEvent, 0),
		haltWindow: haltWindow,
	}
}

// AddEvents appends or updates scheduled macroeconomic releases.
func (ec *EconomicCalendar) AddEvents(events ...MacroEvent) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = append(ec.events, events...)
}

// ClearEvents resets the calendar events list.
func (ec *EconomicCalendar) ClearEvents() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = make([]MacroEvent, 0)
}

// GetEvents returns a copy of all registered events.
func (ec *EconomicCalendar) GetEvents() []MacroEvent {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	res := make([]MacroEvent, len(ec.events))
	copy(res, ec.events)
	return res
}

// PopulateDefaultEvents registers mock upcoming macro releases for runtime simulation.
func (ec *EconomicCalendar) PopulateDefaultEvents() {
	now := time.Now()
	ec.AddEvents(
		MacroEvent{
			ID:          "FOMC-RATE-DECISION",
			Title:       "FOMC Interest Rate Decision",
			Currency:    "USD",
			Impact:      ImpactHigh,
			ScheduledAt: now.Add(2 * time.Hour),
			Forecast:    "5.25%",
			Previous:    "5.50%",
		},
		MacroEvent{
			ID:          "US-CPI-YOY",
			Title:       "US Consumer Price Index (YoY)",
			Currency:    "USD",
			Impact:      ImpactHigh,
			ScheduledAt: now.Add(24 * time.Hour),
			Forecast:    "2.9%",
			Previous:    "3.1%",
		},
		MacroEvent{
			ID:          "ECB-PRESS-CONFERENCE",
			Title:       "ECB Monetary Policy Statement",
			Currency:    "EUR",
			Impact:      ImpactHigh,
			ScheduledAt: now.Add(48 * time.Hour),
			Forecast:    "3.75%",
			Previous:    "3.75%",
		},
	)
}

// IsSymbolHalted evaluates if trading for a specific asset is halted due to a high-impact macro event.
// Halt rule (FR-005): Engine MUST halt opening new trades inside [T_event - 15min, T_event + 15min]
// for high-impact macro releases affecting the asset's quote or base currency.
func (ec *EconomicCalendar) IsSymbolHalted(symbol string, now time.Time) (bool, string) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	relevantCurrencies := getCurrenciesForSymbol(symbol)

	for _, ev := range ec.events {
		if ev.Impact != ImpactHigh {
			continue
		}

		// Check if currency matches
		isRelevant := false
		for _, curr := range relevantCurrencies {
			if strings.EqualFold(ev.Currency, curr) {
				isRelevant = true
				break
			}
		}
		if !isRelevant {
			continue
		}

		// Check event time window: [ScheduledAt - haltWindow, ScheduledAt + haltWindow]
		windowStart := ev.ScheduledAt.Add(-ec.haltWindow)
		windowEnd := ev.ScheduledAt.Add(ec.haltWindow)

		if (now.Equal(windowStart) || now.After(windowStart)) && (now.Equal(windowEnd) || now.Before(windowEnd)) {
			reason := "HALT: Macro Event [" + ev.Title + " (" + ev.Currency + ")] at " + ev.ScheduledAt.UTC().Format(time.RFC3339)
			return true, reason
		}
	}

	return false, ""
}

// getCurrenciesForSymbol extracts base and quote currencies affecting the symbol.
func getCurrenciesForSymbol(symbol string) []string {
	clean := strings.ToUpper(strings.TrimSpace(symbol))
	switch {
	case strings.HasPrefix(clean, "EUR/"):
		return []string{"EUR", "USD"}
	case strings.Contains(clean, "USD"):
		return []string{"USD"}
	case strings.Contains(clean, "BTC") || strings.Contains(clean, "ETH") || strings.Contains(clean, "SOL"):
		return []string{"USD"}
	case strings.Contains(clean, "XAU") || strings.Contains(clean, "XAG") || strings.Contains(clean, "WTI"):
		return []string{"USD"}
	default:
		return []string{"USD"}
	}
}
