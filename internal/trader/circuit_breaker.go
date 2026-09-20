package trader

import (
	"sync"
)

// CircuitBreaker monitors portfolio equity and automatically halts trading if max drawdown is breached.
type CircuitBreaker struct {
	mu           sync.RWMutex
	peakEquity   float64
	currEquity   float64
	maxDDPct     float64 // Maximum allowable drawdown as fraction (e.g. 0.10 for 10%)
	isHalted     bool
	haltReason   string
}

// NewCircuitBreaker creates a circuit breaker instance.
func NewCircuitBreaker(initialCapital float64, maxDrawdownPct float64) *CircuitBreaker {
	return &CircuitBreaker{
		peakEquity: initialCapital,
		currEquity: initialCapital,
		maxDDPct:   maxDrawdownPct,
		isHalted:   false,
	}
}

// UpdateEquity updates the current portfolio equity and trips the circuit breaker if drawdown threshold is breached.
func (cb *CircuitBreaker) UpdateEquity(equity float64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.currEquity = equity
	if equity > cb.peakEquity {
		cb.peakEquity = equity
	}

	if cb.peakEquity > 0 {
		dd := (cb.peakEquity - equity) / cb.peakEquity
		if dd >= cb.maxDDPct {
			cb.isHalted = true
			cb.haltReason = "Maximum drawdown limit breached"
		}
	}
}

// IsHalted returns true if trading is halted by the circuit breaker.
func (cb *CircuitBreaker) IsHalted() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.isHalted
}

// CurrentDrawdownPct returns the current drawdown from peak in percent (0 - 100).
func (cb *CircuitBreaker) CurrentDrawdownPct() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.peakEquity <= 0 {
		return 0
	}
	dd := (cb.peakEquity - cb.currEquity) / cb.peakEquity * 100.0
	if dd < 0 {
		return 0
	}
	return dd
}

// Reset clears the circuit breaker state (manual admin override).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.isHalted = false
	cb.haltReason = ""
	cb.peakEquity = cb.currEquity
}
