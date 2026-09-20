package indicators

import (
	"math"
)

// CalculateConfluence computes a weighted agreement score and suggested direction ("BUY", "SELL", or "NEUTRAL").
func CalculateConfluence(snap Snapshot, weights map[string]float64) (float64, string) {
	rsiWeight := 1.0
	macdWeight := 1.0
	stWeight := 1.0
	microWeight := 1.5 // Microstructure (OBI + CVD) carries strong institutional weight

	if w, ok := weights["RSI"]; ok && w > 0 {
		rsiWeight = w
	}
	if w, ok := weights["MACD"]; ok && w > 0 {
		macdWeight = w
	}
	if w, ok := weights["SUPERTREND"]; ok && w > 0 {
		stWeight = w
	}
	if w, ok := weights["MICROSTRUCTURE"]; ok && w > 0 {
		microWeight = w
	}

	totalWeight := rsiWeight + macdWeight + stWeight + microWeight
	if totalWeight == 0 {
		return 0.0, "NEUTRAL"
	}

	var directionalScore float64

	// RSI signal contribution (-1.0 to +1.0)
	// RSI > 50 leans bullish, RSI < 50 leans bearish; extremes provide momentum
	if snap.RSI >= 50.0 {
		rsiNorm := math.Min(1.0, (snap.RSI-50.0)/20.0)
		directionalScore += rsiNorm * rsiWeight
	} else if snap.RSI > 0 {
		rsiNorm := math.Max(-1.0, (snap.RSI-50.0)/20.0)
		directionalScore += rsiNorm * rsiWeight
	}

	// MACD Histogram contribution (-1.0 to +1.0)
	if snap.MACDHistogram > 0 {
		directionalScore += 1.0 * macdWeight
	} else if snap.MACDHistogram < 0 {
		directionalScore += -1.0 * macdWeight
	}

	// SuperTrend contribution (-1.0 to +1.0)
	if snap.SuperTrendTrend == "BULL" {
		directionalScore += 1.0 * stWeight
	} else if snap.SuperTrendTrend == "BEAR" {
		directionalScore += -1.0 * stWeight
	}

	// Microstructure contribution (-1.0 to +1.0) via L2 OBI and CVD divergence
	microScore := EvaluateMicrostructure(snap.OBI, snap.Divergence)
	directionalScore += microScore * microWeight

	// Apply Volatility Regime dampening/adaptation
	// Under High Volatility Chop, damp directional conviction to filter out false breakouts
	if snap.Regime == RegimeHighVolChop {
		directionalScore *= 0.60
	}

	normalizedScore := directionalScore / totalWeight
	confidence := math.Abs(normalizedScore)

	var direction string
	if normalizedScore >= 0.25 {
		direction = "BUY"
	} else if normalizedScore <= -0.25 {
		direction = "SELL"
	} else {
		direction = "NEUTRAL"
	}

	return confidence, direction
}
