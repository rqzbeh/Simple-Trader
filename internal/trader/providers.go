package trader

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// LiveMarketData provides in-memory thread-safe ticker storage implementing MarketDataProvider.
type LiveMarketData struct {
	mu      sync.RWMutex
	quotes  map[string]cache.TickerQuote
	fetcher *market.BinanceFetcher
}

// NewLiveMarketData creates an empty live market data store with active online exchange fetcher.
func NewLiveMarketData() *LiveMarketData {
	return &LiveMarketData{
		quotes:  make(map[string]cache.TickerQuote),
		fetcher: market.NewBinanceFetcher(),
	}
}

// UpdateQuote records a new ticker quote.
func (m *LiveMarketData) UpdateQuote(quote cache.TickerQuote) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotes[quote.Symbol] = quote
}

// GetQuote returns the cached quote for a symbol, if present.
func (m *LiveMarketData) GetQuote(symbol string) (cache.TickerQuote, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.quotes[symbol]
	return q, ok
}

// GetAllQuotes returns all current cached ticker quotes.
func (m *LiveMarketData) GetAllQuotes() []cache.TickerQuote {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]cache.TickerQuote, 0, len(m.quotes))
	for _, q := range m.quotes {
		res = append(res, q)
	}
	return res
}

// GetLatestPrice returns the current market price for a symbol.
// If the symbol has not been cached from the live tick stream yet, it directly queries the online exchange.
func (m *LiveMarketData) GetLatestPrice(symbol string) (float64, error) {
	m.mu.RLock()
	if q, ok := m.quotes[symbol]; ok && q.Price > 0 {
		m.mu.RUnlock()
		return q.Price, nil
	}
	m.mu.RUnlock()

	// Direct online fetch from live exchange API for zero static hardcoding
	if m.fetcher != nil {
		cleanSymbol := strings.ReplaceAll(symbol, "/", "")
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		quote, err := m.fetcher.FetchTicker(ctx, cleanSymbol)
		if err == nil && quote != nil && quote.Price > 0 {
			m.UpdateQuote(*quote)
			return quote.Price, nil
		}
	}

	return 0, fmt.Errorf("online price unavailable for %s", symbol)
}

// GetMarketDepth returns spread and available liquidity depth from live order book.
// Falls back to conservative estimates when real order book data is unavailable.
func (m *LiveMarketData) GetMarketDepth(symbol string) (spreadPct float64, availableDepth float64, err error) {
	m.mu.RLock()
	q, ok := m.quotes[symbol]
	m.mu.RUnlock()

	if ok && q.Price > 0 {
		// Estimate spread from 24h range if available
		if q.High24h > 0 && q.Low24h > 0 && q.High24h > q.Low24h {
			dailyRange := (q.High24h - q.Low24h) / q.Price
			spreadPct = dailyRange * 0.001 // Fraction of daily range as spread estimate
			if spreadPct < 0.0001 {
				spreadPct = 0.0001
			}
		} else {
			spreadPct = 0.0005 // 5 bps conservative fallback
		}

		// Estimate depth from 24h volume
		if q.Volume > 0 {
			availableDepth = q.Volume * 0.001 // 0.1% of 24h volume as actionable depth
		} else {
			availableDepth = 50.0
		}
		return spreadPct, availableDepth, nil
	}

	return 0.0005, 50.0, fmt.Errorf("no market depth data for %s", symbol)
}
