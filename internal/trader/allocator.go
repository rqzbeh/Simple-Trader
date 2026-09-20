package trader

import (
	"math"
	"sync"
)

// AllocatorConfig sets the parameters for Core vs Alpha asset allocation and risk rules.
type AllocatorConfig struct {
	TotalCapital       float64
	CoreTargetPct      float64 // 0.0 - 1.0 (e.g. 0.60)
	AlphaTargetPct     float64 // 0.0 - 1.0 (e.g. 0.40)
	MaxRiskPerTradePct float64 // 0.0 - 1.0 (e.g. 0.02)
}

// Allocator dynamically balances capital between Core (Gold, Silver) and Alpha (Crypto, Forex, Oil) buckets.
type Allocator struct {
	mu     sync.RWMutex
	config AllocatorConfig
}

// NewAllocator creates a capital allocator.
func NewAllocator(cfg AllocatorConfig) *Allocator {
	return &Allocator{
		config: cfg,
	}
}

// GetAvailableBuckets returns (coreCapital, alphaCapital).
func (a *Allocator) GetAvailableBuckets() (float64, float64) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	core := a.config.TotalCapital * a.config.CoreTargetPct
	alpha := a.config.TotalCapital * a.config.AlphaTargetPct
	return core, alpha
}

// CalculatePositionSize computes the maximum position size based on fixed fractional risk.
// riskPct is stop-loss distance percentage (e.g. 0.015 for 1.5%).
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

	// Maximum allowable dollar risk for this single trade
	maxRiskDollars := a.config.TotalCapital * a.config.MaxRiskPerTradePct

	// Dollar risk per single contract/unit
	riskPerUnit := entryPrice * riskPct
	if riskPerUnit <= 0 {
		return 0
	}

	sizeByRisk := maxRiskDollars / riskPerUnit

	// Cap by bucket capital allocation (cannot exceed 50% of bucket for a single trade)
	maxDollarSize := bucketCapital * 0.50
	sizeByCap := maxDollarSize / entryPrice

	units := math.Min(sizeByRisk, sizeByCap)
	return math.Round(units*10000) / 10000
}
