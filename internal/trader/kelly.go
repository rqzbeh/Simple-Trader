package trader

import (
	"math"
)

// KellyConfig holds parameters for fractional Kelly criterion calculation.
type KellyConfig struct {
	Fraction       float64 // Fractional Kelly multiplier (e.g. 0.5 for Half-Kelly)
	MinRiskPct     float64 // Minimum risk floor (FR-007: 0.005 / 0.5%)
	MaxRiskPct     float64 // Maximum risk ceiling (FR-007: 0.02 / 2.0%)
	DefaultWinRate float64 // Prior baseline win rate (e.g. 0.50)
	DefaultWinLoss float64 // Prior baseline win/loss payoff ratio (e.g. 1.50)
}

// NewKellyConfig creates a custom or dynamically configured KellyConfig.
func NewKellyConfig(fraction, minRiskPct, maxRiskPct, defaultWinRate, defaultWinLoss float64) KellyConfig {
	if fraction <= 0 {
		fraction = 0.50
	}
	if minRiskPct <= 0 {
		minRiskPct = 0.005
	}
	if maxRiskPct <= 0 {
		maxRiskPct = 0.020
	}
	if defaultWinRate <= 0 {
		defaultWinRate = 0.52
	}
	if defaultWinLoss <= 0 {
		defaultWinLoss = 1.60
	}
	return KellyConfig{
		Fraction:       fraction,
		MinRiskPct:     minRiskPct,
		MaxRiskPct:     maxRiskPct,
		DefaultWinRate: defaultWinRate,
		DefaultWinLoss: defaultWinLoss,
	}
}

// DefaultKellyConfig returns production Half-Kelly parameters conforming to FR-007.
func DefaultKellyConfig() KellyConfig {
	return NewKellyConfig(0.50, 0.005, 0.020, 0.52, 1.60)
}

// CalculateHalfKelly computes dynamic risk fraction using the Half-Kelly criterion:
// f* = (p * b - (1 - p)) / b
// Risk = clamp(fraction * f*, minRisk, maxRisk)
//
// Parameters:
// p: Win probability (0.0 to 1.0)
// b: Payoff ratio (Avg Win $ / Avg Loss $ or TP_dist / SL_dist)
func CalculateHalfKelly(cfg KellyConfig, winProb, payoffRatio float64) float64 {
	p := winProb
	b := payoffRatio

	if p <= 0 || p >= 1.0 {
		p = cfg.DefaultWinRate
	}
	if b <= 0 {
		b = cfg.DefaultWinLoss
	}

	// Kelly formula: f* = (p * b - (1 - p)) / b = p - (1 - p) / b
	fStar := (p*b - (1.0 - p)) / b

	// If edge is negative, return minimum floor risk or 0
	if fStar <= 0 {
		return cfg.MinRiskPct
	}

	// Scale by fractional Kelly (Half-Kelly: 0.5 * f*)
	fractionalRisk := cfg.Fraction * fStar

	// Clamp to [MinRiskPct, MaxRiskPct] (FR-007: 0.5% to 2.0%)
	if fractionalRisk < cfg.MinRiskPct {
		fractionalRisk = cfg.MinRiskPct
	}
	if fractionalRisk > cfg.MaxRiskPct {
		fractionalRisk = cfg.MaxRiskPct
	}

	return math.Round(fractionalRisk*10000) / 10000
}
