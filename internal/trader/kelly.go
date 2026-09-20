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

// DefaultKellyConfig returns production Half-Kelly parameters conforming to FR-007.
func DefaultKellyConfig() KellyConfig {
	return KellyConfig{
		Fraction:       0.50,  // Half-Kelly
		MinRiskPct:     0.005, // 0.5%
		MaxRiskPct:     0.020, // 2.0%
		DefaultWinRate: 0.52,  // 52% baseline
		DefaultWinLoss: 1.60,  // 1.6 R:R baseline
	}
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
