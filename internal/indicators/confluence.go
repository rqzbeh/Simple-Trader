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
	cmfWeight := 1.2   // Chaikin Money Flow institutional accumulation/distribution
	kerWeight := 1.0   // Kaufman Efficiency Ratio (signal-to-noise)

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
	if w, ok := weights["CMF"]; ok && w > 0 {
		cmfWeight = w
	}
	if w, ok := weights["KER"]; ok && w > 0 {
		kerWeight = w
	}

	totalWeight := rsiWeight + macdWeight + stWeight + microWeight + cmfWeight + kerWeight
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

	// Chaikin Money Flow contribution (-1.0 to +1.0)
	// CMF > +0.05 indicates institutional accumulation (bullish)
	// CMF < -0.05 indicates institutional distribution (bearish)
	if snap.CMF > 0.02 {
		cmfNorm := math.Min(1.0, snap.CMF/0.15)
		directionalScore += cmfNorm * cmfWeight
	} else if snap.CMF < -0.02 {
		cmfNorm := math.Max(-1.0, snap.CMF/0.15)
		directionalScore += cmfNorm * cmfWeight
	}

	// Kaufman Efficiency Ratio (KER) acts as a trend conviction multiplier or filter.
	// When KER is high (> 0.50), trend signals are clean and amplified.
	// When KER is low (< 0.25), market is noise-dominated.
	if snap.KaufmanER > 0 {
		if snap.KaufmanER >= 0.50 {
			bonus := 0.35
			if directionalScore < 0 {
				bonus = -0.35
			}
			directionalScore += bonus * kerWeight
		} else if snap.KaufmanER < 0.25 {
			directionalScore *= 0.75
		}
	}

	// Apply Volatility Regime dampening/adaptation
	// Under High Volatility Chop, damp directional conviction to filter out false breakouts
	if snap.Regime == RegimeHighVolChop {
		directionalScore *= 0.60
	} else if snap.Regime == RegimeVolatileBreakout {
		directionalScore *= 1.15
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
