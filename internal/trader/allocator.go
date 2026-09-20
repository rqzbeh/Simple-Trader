package trader

import (
	"errors"
	"math"
	"sync"
)

var (
	ErrExcessiveWithdrawal = errors.New("requested withdrawal exceeds Tier 1 unencumbered cash buffer")
	ErrNegativeCapital     = errors.New("capital amounts must be non-negative")
)

// AllocatorConfig sets the parameters for 3-Tier capital allocation and risk rules.
type AllocatorConfig struct {
	TotalCapital       float64
	Tier1TargetPct     float64 // Target 0.15 (15% Cash/Withdrawal Buffer)
	CoreTargetPct      float64 // Target 0.45 (45% Core Commodities: Gold/Silver)
	AlphaTargetPct     float64 // Target 0.40 (40% Tactical Alpha Multi-Horizon)
	MaxRiskPerTradePct float64 // e.g. 0.02 (2% max equity risk per trade)
}

// Default3TierConfig returns the institutional 3-tier liquidity configuration.
func Default3TierConfig(totalCapital float64) AllocatorConfig {
	return AllocatorConfig{
		TotalCapital:       totalCapital,
		Tier1TargetPct:     0.15, // 15% Unencumbered liquid cash
		CoreTargetPct:      0.45, // 45% Core wealth preservation (Gold, Silver)
		AlphaTargetPct:     0.40, // 40% Tactical alpha trading
		MaxRiskPerTradePct: 0.02, // 2% risk limit
	}
}

// CapitalAllocator is an alias for Allocator.
type CapitalAllocator = Allocator

// Allocator dynamically balances capital across the 3 Tiers:
// Tier 1 (Liquid Cash Buffer), Tier 2 (Core Commodities: Gold/Silver), Tier 3 (Tactical Alpha)
type Allocator struct {
	mu          sync.RWMutex
	config      AllocatorConfig
	tier1Cash   float64 // Actual held unencumbered cash reserve
	macroEngine *MacroRegimeEngine
}

// NewAllocator creates a 3-tier capital allocator.
func NewAllocator(cfg AllocatorConfig) *Allocator {
	macroEngine := NewMacroRegimeEngine()
	regimeState := macroEngine.GetCurrentState()

	if cfg.Tier1TargetPct <= 0 && cfg.CoreTargetPct <= 0 && cfg.AlphaTargetPct <= 0 {
		// Use dynamic macro regime targets as default
		cfg.Tier1TargetPct = regimeState.TargetTier1Pct
		cfg.CoreTargetPct = regimeState.TargetCorePct
		cfg.AlphaTargetPct = regimeState.TargetAlphaPct
	} else {
		if cfg.Tier1TargetPct <= 0 {
			cfg.Tier1TargetPct = 0.15
		}
		if cfg.CoreTargetPct <= 0 {
			cfg.CoreTargetPct = 0.45
		}
		if cfg.AlphaTargetPct <= 0 {
			cfg.AlphaTargetPct = 0.40
		}
	}

	initialTier1 := cfg.TotalCapital * cfg.Tier1TargetPct

	return &Allocator{
		config:      cfg,
		tier1Cash:   initialTier1,
		macroEngine: macroEngine,
	}
}

// GetMacroEngine returns the underlying macroeconomic stress scoring engine.
func (a *Allocator) GetMacroEngine() *MacroRegimeEngine {
	return a.macroEngine
}

// ApplyMacroRegime updates the allocator targets dynamically based on latest macroeconomic indicators.
func (a *Allocator) ApplyMacroRegime(ind MacroIndicators) MacroRegimeState {
	a.mu.Lock()
	defer a.mu.Unlock()

	state := a.macroEngine.UpdateIndicators(ind)
	a.config.Tier1TargetPct = state.TargetTier1Pct
	a.config.CoreTargetPct = state.TargetCorePct
	a.config.AlphaTargetPct = state.TargetAlphaPct

	return state
}

// GetAvailableBuckets returns (coreCapital, alphaCapital).
func (a *Allocator) GetAvailableBuckets() (float64, float64) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	core := a.config.TotalCapital * a.config.CoreTargetPct
	alpha := a.config.TotalCapital * a.config.AlphaTargetPct
	return core, alpha
}

// GetTier1CashReserve returns the current unencumbered liquid cash buffer.
func (a *Allocator) GetTier1CashReserve() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tier1Cash
}

// GetTotalCapital returns the total capital managed by the allocator.
func (a *Allocator) GetTotalCapital() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config.TotalCapital
}

// Get3TierBreakdown returns current allocation amounts and targets.
func (a *Allocator) Get3TierBreakdown() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	total := a.config.TotalCapital
	core := total * a.config.CoreTargetPct
	tactical := total * a.config.AlphaTargetPct

	return map[string]interface{}{
		"total_equity":               total,
		"tier1_cash":                 math.Round(a.tier1Cash*100) / 100,
		"tier1_target_pct":           a.config.Tier1TargetPct,
		"tier2_core":                 math.Round(core*100) / 100,
		"tier2_target_pct":           a.config.CoreTargetPct,
		"tier3_tactical":             math.Round(tactical*100) / 100,
		"tier3_target_pct":           a.config.AlphaTargetPct,
		"available_for_withdrawal":   math.Round(a.tier1Cash*100) / 100,
	}
}

// DebitTier1Cash deducts capital from Tier 1 unencumbered cash during an investor withdrawal.
func (a *Allocator) DebitTier1Cash(amount float64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if amount <= 0 {
		return ErrNegativeCapital
	}
	if amount > a.tier1Cash {
		return ErrExcessiveWithdrawal
	}

	a.tier1Cash -= amount
	a.config.TotalCapital = math.Max(0, a.config.TotalCapital-amount)
	return nil
}

// SweepProfitToTier1 transfers realized trading profits from tactical trades into Tier 1 cash buffer.
func (a *Allocator) SweepProfitToTier1(profitAmount float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if profitAmount > 0 {
		a.tier1Cash += profitAmount
		a.config.TotalCapital += profitAmount
	}
}

// AddCapital deposits new capital into the fund and recalculates buffers.
func (a *Allocator) AddCapital(depositAmount float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if depositAmount > 0 {
		a.config.TotalCapital += depositAmount
		// Direct portion immediately to Tier 1 cash buffer
		a.tier1Cash += depositAmount * a.config.Tier1TargetPct
	}
}

// CalculatePositionSize computes maximum position size based on fixed fractional risk.
func (a *Allocator) CalculatePositionSize(bucket string, entryPrice float64, riskPct float64) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if entryPrice <= 0 || riskPct <= 0 {
		return 0
	}

	var bucketCapital float64
	if bucket == "CORE" {
		bucketCapital = a.config.TotalCapital * a.config.CoreTargetPct
	} else {
		bucketCapital = a.config.TotalCapital * a.config.AlphaTargetPct
	}

	maxRiskDollars := a.config.TotalCapital * a.config.MaxRiskPerTradePct
	riskPerUnit := entryPrice * riskPct
	if riskPerUnit <= 0 {
		return 0
	}

	sizeByRisk := maxRiskDollars / riskPerUnit
	maxDollarSize := bucketCapital * 0.50
	sizeByCap := maxDollarSize / entryPrice

	units := math.Min(sizeByRisk, sizeByCap)
	return math.Round(units*10000) / 10000
}

// CalculateKellyPositionSize computes position size using the dynamic Half-Kelly criterion.
func (a *Allocator) CalculateKellyPositionSize(bucket string, entryPrice, stopLoss, takeProfit, winProb float64) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if entryPrice <= 0 || stopLoss <= 0 || takeProfit <= 0 {
		return a.CalculatePositionSize(bucket, entryPrice, 0.015)
	}

	slDistance := math.Abs(entryPrice - stopLoss)
	tpDistance := math.Abs(takeProfit - entryPrice)
	if slDistance <= 0 {
		return 0
	}

	payoffRatio := tpDistance / slDistance
	riskFraction := CalculateHalfKelly(DefaultKellyConfig(), winProb, payoffRatio)

	var bucketCapital float64
	if bucket == "CORE" {
		bucketCapital = a.config.TotalCapital * a.config.CoreTargetPct
	} else {
		bucketCapital = a.config.TotalCapital * a.config.AlphaTargetPct
	}

	maxRiskDollars := a.config.TotalCapital * riskFraction
	sizeByRisk := maxRiskDollars / slDistance

	maxDollarSize := bucketCapital * 0.50
	sizeByCap := maxDollarSize / entryPrice

	units := math.Min(sizeByRisk, sizeByCap)
	return math.Round(units*10000) / 10000
}
