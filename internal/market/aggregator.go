package market

import (
	"sync"
	"time"
)

// CandleBar represents an OHLCV candlestick bar.
type CandleBar struct {
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"` // e.g. "3h"
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	Ticks     int       `json:"ticks"`
	Complete  bool      `json:"complete"`
}

// AggregatorConfig sets parameters for multi-horizon candlestick aggregation.
type AggregatorConfig struct {
	TimeframeSeconds int64 // Default 10800 for 3-Hour Bars
	MaxHistoryBars   int   // Ring buffer size per symbol (e.g. 100 bars)
}

// Default3HourAggregatorConfig provides standard 3-hour swing settings.
func Default3HourAggregatorConfig() AggregatorConfig {
	return AggregatorConfig{
		TimeframeSeconds: 10800, // 3 hours = 3 * 3600 = 10,800 seconds
		MaxHistoryBars:   100,
	}
}

// CandleAggregator aggregates real-time price ticks into completed 3-hour OHLCV bars.
type CandleAggregator struct {
	mu           sync.RWMutex
	cfg          AggregatorConfig
	currentBars  map[string]*CandleBar   // Active partial bar per symbol
	historyBars  map[string][]CandleBar  // Completed historical bars per symbol
}

// NewCandleAggregator initializes the multi-horizon aggregator.
func NewCandleAggregator(cfg AggregatorConfig) *CandleAggregator {
	if cfg.TimeframeSeconds <= 0 {
		cfg.TimeframeSeconds = 10800
	}
	if cfg.MaxHistoryBars <= 0 {
		cfg.MaxHistoryBars = 100
	}
	return &CandleAggregator{
		cfg:         cfg,
		currentBars: make(map[string]*CandleBar),
		historyBars: make(map[string][]CandleBar),
	}
}

// IngestTick processes an incoming price quote and updates or completes the 3-hour bar.
// Returns (completedBar, true) if a 3-hour bar was just closed and completed.
func (a *CandleAggregator) IngestTick(symbol string, price float64, volume float64, tickTime time.Time) (*CandleBar, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if price <= 0 {
		return nil, false
	}

	tf := a.cfg.TimeframeSeconds
	unixSec := tickTime.Unix()
	slotStartSec := (unixSec / tf) * tf
	slotStartTime := time.Unix(slotStartSec, 0).UTC()
	slotEndTime := slotStartTime.Add(time.Duration(tf) * time.Second)

	cur, exists := a.currentBars[symbol]
	var completedBar *CandleBar

	if !exists {
		// First tick for symbol
		a.currentBars[symbol] = &CandleBar{
			Symbol:    symbol,
			Timeframe: "3h",
			StartTime: slotStartTime,
			EndTime:   slotEndTime,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    volume,
			Ticks:     1,
			Complete:  false,
		}
		return nil, false
	}

	// If tick belongs to a new time window, finalize the prior bar
	if slotStartTime.After(cur.StartTime) {
		finished := *cur
		finished.Complete = true
		completedBar = &finished

		// Append to history
		hist := a.historyBars[symbol]
		hist = append(hist, finished)
		if len(hist) > a.cfg.MaxHistoryBars {
			hist = hist[len(hist)-a.cfg.MaxHistoryBars:]
		}
		a.historyBars[symbol] = hist

		// Initialize new current bar
		a.currentBars[symbol] = &CandleBar{
			Symbol:    symbol,
			Timeframe: "3h",
			StartTime: slotStartTime,
			EndTime:   slotEndTime,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    volume,
			Ticks:     1,
			Complete:  false,
		}
		return completedBar, true
	}

	// Update existing bar in same time window
	if price > cur.High {
		cur.High = price
	}
	if price < cur.Low {
		cur.Low = price
	}
	cur.Close = price
	cur.Volume += volume
	cur.Ticks++

	return nil, false
}

// GetHistory returns completed bars for a given symbol.
func (a *CandleAggregator) GetHistory(symbol string) []CandleBar {
	a.mu.RLock()
	defer a.mu.RUnlock()

	hist, exists := a.historyBars[symbol]
	if !exists {
		return []CandleBar{}
	}
	res := make([]CandleBar, len(hist))
	copy(res, hist)
	return res
}

// GetCurrentBar returns the active, in-progress 3-hour bar.
func (a *CandleAggregator) GetCurrentBar(symbol string) (*CandleBar, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cur, exists := a.currentBars[symbol]
	if !exists {
		return nil, false
	}
	cpy := *cur
	return &cpy, true
}
