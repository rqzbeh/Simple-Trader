package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// rawCalendarItem models the JSON representation from the public ForexFactory calendar feed.
type rawCalendarItem struct {
	Title    string `json:"title"`
	Country  string `json:"country"`
	Date     string `json:"date"`
	Impact   string `json:"impact"`
	Forecast string `json:"forecast"`
	Previous string `json:"previous"`
}

// FetchLiveMacroEvents retrieves authentic live macroeconomic events from the institutional calendar feed.
func FetchLiveMacroEvents(ctx context.Context, calendarURL string) ([]MacroEvent, error) {
	if calendarURL == "" {
		calendarURL = "https://nfs.faireconomy.media/ff_calendar_thisweek.json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, calendarURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar request: %w", err)
	}
	req.Header.Set("User-Agent", "SimpleTrader-Quant/1.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch live economic calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar feed returned HTTP %d", resp.StatusCode)
	}

	var rawItems []rawCalendarItem
	if err := json.NewDecoder(resp.Body).Decode(&rawItems); err != nil {
		return nil, fmt.Errorf("failed to decode calendar items: %w", err)
	}

	var events []MacroEvent
	for i, item := range rawItems {
		parsedTime, err := time.Parse(time.RFC3339, item.Date)
		if err != nil {
			continue
		}

		var impact ImpactLevel
		switch strings.ToUpper(strings.TrimSpace(item.Impact)) {
		case "HIGH":
			impact = ImpactHigh
		case "MEDIUM":
			impact = ImpactMedium
		default:
			impact = ImpactLow
		}

		id := fmt.Sprintf("%s-%s-%d", item.Country, strings.ReplaceAll(item.Title, " ", "_"), i)

		events = append(events, MacroEvent{
			ID:          id,
			Title:       item.Title,
			Currency:    strings.ToUpper(item.Country),
			Impact:      impact,
			ScheduledAt: parsedTime,
			Forecast:    item.Forecast,
			Previous:    item.Previous,
		})
	}

	return events, nil
}

// RefreshFromLiveFeed fetches and updates authentic calendar events from the institutional feed.
func (ec *EconomicCalendar) RefreshFromLiveFeed(ctx context.Context, calendarURL string) error {
	events, err := FetchLiveMacroEvents(ctx, calendarURL)
	if err != nil {
		return err
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = events
	return nil
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
