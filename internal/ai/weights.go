package ai

import "math"

// WeightEngine manages adaptive weight adjustments based on post-trade outcomes.
type WeightEngine struct {
	learningRate float64
	minWeight    float64
	maxWeight    float64
}

// NewWeightEngine initializes an adaptive weight manipulation engine.
func NewWeightEngine() *WeightEngine {
	return &WeightEngine{
		learningRate: 0.15,
		minWeight:    0.20,
		maxWeight:    3.00,
	}
}

// clamp bounds weight to [minWeight, maxWeight].
func (e *WeightEngine) clamp(w float64) float64 {
	if w < e.minWeight {
		return e.minWeight
	}
	if w > e.maxWeight {
		return e.maxWeight
	}
	return w
}

// AdjustWeights updates indicator weights based on trade success or failure attribution.
func (e *WeightEngine) AdjustWeights(currentWeights map[string]float64, outcome TradeOutcome) map[string]float64 {
	updated := make(map[string]float64)
	for k, v := range currentWeights {
		updated[k] = v
	}

	isProfitable := outcome.Pnl > 0

	// 1. Evaluate SuperTrend attribution
	if current, exists := updated["SUPERTREND"]; exists {
		stAligned := (outcome.Side == "BUY" && outcome.SuperTrendTrend == "BULL") ||
			(outcome.Side == "SELL" && outcome.SuperTrendTrend == "BEAR")

		if isProfitable && stAligned {
			// Reward indicator for successful alignment
			updated["SUPERTREND"] = e.clamp(current * (1.0 + e.learningRate))
		} else if !isProfitable && stAligned {
			// Regret penalty: indicator signaled direction but trade lost
			updated["SUPERTREND"] = e.clamp(current * (1.0 - e.learningRate))
		}
	}

	// 2. Evaluate MACD attribution
	if current, exists := updated["MACD"]; exists {
		macdAligned := (outcome.Side == "BUY" && outcome.MACDHistogram > 0) ||
			(outcome.Side == "SELL" && outcome.MACDHistogram < 0)

		if isProfitable && macdAligned {
			updated["MACD"] = e.clamp(current * (1.0 + e.learningRate))
		} else if !isProfitable && macdAligned {
			updated["MACD"] = e.clamp(current * (1.0 - e.learningRate))
		}
	}

	// 3. Evaluate RSI attribution
	if current, exists := updated["RSI"]; exists {
		// If buying when overbought (>70) and trade lost, penalize RSI
		if !isProfitable && outcome.Side == "BUY" && outcome.RSI > 70 {
			updated["RSI"] = e.clamp(current * (1.0 - e.learningRate*1.5))
		} else if isProfitable && outcome.Side == "BUY" && outcome.RSI > 50 && outcome.RSI <= 70 {
			updated["RSI"] = e.clamp(current * (1.0 + e.learningRate))
		} else if !isProfitable && outcome.Side == "SELL" && outcome.RSI < 30 {
			updated["RSI"] = e.clamp(current * (1.0 - e.learningRate*1.5))
		} else if isProfitable && outcome.Side == "SELL" && outcome.RSI < 50 && outcome.RSI >= 30 {
			updated["RSI"] = e.clamp(current * (1.0 + e.learningRate))
		}
	}

	// 4. Exponential decay back towards baseline (1.0)
	for k, v := range updated {
		// Subtle pull towards 1.0 to prevent runaway drift
		decayFactor := 0.02
		updated[k] = e.clamp(v + (1.0-v)*decayFactor)
		// Round to 4 decimals
		updated[k] = math.Round(updated[k]*10000) / 10000
	}

	return updated
}
