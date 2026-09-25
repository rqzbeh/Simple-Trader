package trader

import (
	"context"
	"log"
	"sync"
	"time"
)

// DaemonConfig holds parameters for the continuous autonomous trading daemon.
type DaemonConfig struct {
	TickInterval time.Duration
	Symbols      []string
}

// MarketDataProvider abstracts live market price and book queries.
type MarketDataProvider interface {
	GetLatestPrice(symbol string) (float64, error)
	GetMarketDepth(symbol string) (spreadPct float64, availableDepth float64, err error)
}

// MacroCalendar abstracts event halt queries for news/macro risk control.
type MacroCalendar interface {
	IsSymbolHalted(symbol string, now time.Time) (bool, string)
}

// TradingDaemon manages the 24/7 autonomous execution loop.
type TradingDaemon struct {
	mu         sync.RWMutex
	config     DaemonConfig
	engine     *ExecutionEngine
	allocator  *CapitalAllocator
	circuit    *CircuitBreaker
	market     MarketDataProvider
	calendar   MacroCalendar
	running    bool
	stopChan   chan struct{}
	lastErrors map[string]error
}

// NewTradingDaemon initializes a continuous trading daemon instance.
func NewTradingDaemon(
	cfg DaemonConfig,
	engine *ExecutionEngine,
	allocator *CapitalAllocator,
	circuit *CircuitBreaker,
	market MarketDataProvider,
) *TradingDaemon {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 1 * time.Second
	}
	return &TradingDaemon{
		config:     cfg,
		engine:     engine,
		allocator:  allocator,
		circuit:    circuit,
		market:     market,
		stopChan:   make(chan struct{}),
		lastErrors: make(map[string]error),
	}
}

// SetCalendar assigns a macro economic calendar for event-driven trade halts.
func (d *TradingDaemon) SetCalendar(cal MacroCalendar) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calendar = cal
}

// Start begins continuous background processing.
func (d *TradingDaemon) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = true
	d.stopChan = make(chan struct{})
	d.mu.Unlock()

	go d.loop(ctx)
	return nil
}

// Stop terminates the continuous background loop.
func (d *TradingDaemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running {
		return
	}
	d.running = false
	close(d.stopChan)
}

// IsRunning returns whether the daemon is currently active.
func (d *TradingDaemon) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// ProcessTick runs one complete tick iteration across all configured symbols.
func (d *TradingDaemon) ProcessTick(ctx context.Context) {
	if d.circuit != nil && d.circuit.IsHalted() {
		return
	}

	currentPrices := make(map[string]float64)

	// 1. Gather prices & check open position exits
	for _, symbol := range d.config.Symbols {
		if d.market == nil {
			continue
		}
		price, err := d.market.GetLatestPrice(symbol)
		if err != nil {
			d.mu.Lock()
			d.lastErrors[symbol] = err
			d.mu.Unlock()
			continue
		}
		currentPrices[symbol] = price

		// Check exits for open position
		if d.engine != nil {
			if closedTrade, exited := d.engine.CheckExit(symbol, price); exited {
				log.Printf("[Daemon] Position exited for %s: reason=%s pnl=%.2f ret=%.2f%% fee=%.4f slippage=%.4f",
					symbol, closedTrade.ExitReason, closedTrade.RealizedPnL, closedTrade.ReturnPct, closedTrade.ExecutionFee, closedTrade.SlippagePaid)

				// Sweep tactical profit to Tier 1 cash buffer
				if closedTrade.RealizedPnL > 0 && d.allocator != nil {
					d.allocator.SweepProfitToTier1(closedTrade.RealizedPnL)
					log.Printf("[Daemon] Swept profit of $%.2f from %s into Tier 1 cash reserve", closedTrade.RealizedPnL, symbol)
				}
			}
		}
	}

	// 2. Update equity & circuit breaker
	if d.engine != nil && d.circuit != nil && len(currentPrices) > 0 {
		totalEquity := d.engine.GetTotalEquity(currentPrices)
		d.circuit.UpdateEquity(totalEquity)
		if d.circuit.IsHalted() {
			log.Printf("[Daemon] Circuit breaker triggered halt at equity %.2f", totalEquity)
			return
		}
	}

}

func (d *TradingDaemon) loop(ctx context.Context) {
	ticker := time.NewTicker(d.config.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.Stop()
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			d.ProcessTick(ctx)
		}
	}
}
