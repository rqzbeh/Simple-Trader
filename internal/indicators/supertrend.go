package indicators

import (
	"github.com/rqzbeh/simple-trader/internal/db"
)

// CalculateSuperTrend computes the SuperTrend indicator values and dynamic trend states ("BULL" / "BEAR").
func CalculateSuperTrend(candles []db.Candle, period int, multiplier float64) SuperTrendResult {
	n := len(candles)
	res := SuperTrendResult{
		Value: make([]float64, n),
		Trend: make([]string, n),
	}
	if n == 0 || period <= 0 {
		return res
	}

	atr := CalculateATR(candles, period)

	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	for i := 0; i < n; i++ {
		hl2 := (candles[i].High + candles[i].Low) / 2.0
		basicUpper := hl2 + (multiplier * atr[i])
		basicLower := hl2 - (multiplier * atr[i])

		if i == 0 {
			upperBand[i] = basicUpper
			lowerBand[i] = basicLower
			res.Trend[i] = "BULL"
			res.Value[i] = lowerBand[i]
			continue
		}

		// Calculate trailing Upper Band
		if basicUpper < upperBand[i-1] || candles[i-1].Close > upperBand[i-1] {
			upperBand[i] = basicUpper
		} else {
			upperBand[i] = upperBand[i-1]
		}

		// Calculate trailing Lower Band
		if basicLower > lowerBand[i-1] || candles[i-1].Close < lowerBand[i-1] {
			lowerBand[i] = basicLower
		} else {
			lowerBand[i] = lowerBand[i-1]
		}

		// Determine Trend Direction
		prevTrend := res.Trend[i-1]
		if prevTrend == "BULL" {
			if candles[i].Close < lowerBand[i] {
				res.Trend[i] = "BEAR"
				res.Value[i] = upperBand[i]
			} else {
				res.Trend[i] = "BULL"
				res.Value[i] = lowerBand[i]
			}
		} else { // prevTrend == "BEAR"
			if candles[i].Close > upperBand[i] {
				res.Trend[i] = "BULL"
				res.Value[i] = lowerBand[i]
			} else {
				res.Trend[i] = "BEAR"
				res.Value[i] = upperBand[i]
			}
		}
	}

	return res
}
