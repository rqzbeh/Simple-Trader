package indicators

import (
	"math"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// CalculateGarmanKlass computes the rolling Garman-Klass volatility estimator.
// The Garman-Klass estimator incorporates High, Low, Open, and Close prices,
// providing an unbiased minimum-variance estimator approximately 8x more efficient
// than simple close-to-close volatility.
func CalculateGarmanKlass(candles []db.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 || period <= 0 {
		return result
	}

	const c = 0.3862943611198906 // 2*ln(2) - 1

	instVariance := make([]float64, n)
	for i, candle := range candles {
		if candle.Low <= 0 || candle.Open <= 0 {
			instVariance[i] = 0
			continue
		}
		hlRatio := math.Log(candle.High / candle.Low)
		coRatio := math.Log(candle.Close / candle.Open)

		varGK := 0.5*(hlRatio*hlRatio) - c*(coRatio*coRatio)
		if varGK < 0 {
			varGK = 0
		}
		instVariance[i] = varGK
	}

	// Rolling average of variance over period, then square root
	var rollingSum float64
	for i := 0; i < n; i++ {
		rollingSum += instVariance[i]
		if i >= period {
			rollingSum -= instVariance[i-period]
			avgVar := rollingSum / float64(period)
			result[i] = math.Sqrt(avgVar)
		} else {
			avgVar := rollingSum / float64(i+1)
			result[i] = math.Sqrt(avgVar)
		}
	}

	return result
}

// CalculateParkinson computes the rolling Parkinson volatility estimator.
// It uses the high-low price range to estimate diffusion constant with minimal bid-ask bounce noise.
func CalculateParkinson(candles []db.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 || period <= 0 {
		return result
	}

	const factor = 2.772588722239781 // 4 * ln(2)

	instVariance := make([]float64, n)
	for i, candle := range candles {
		if candle.Low <= 0 {
			instVariance[i] = 0
			continue
		}
		hlRatio := math.Log(candle.High / candle.Low)
		instVariance[i] = (hlRatio * hlRatio) / factor
	}

	var rollingSum float64
	for i := 0; i < n; i++ {
		rollingSum += instVariance[i]
		if i >= period {
			rollingSum -= instVariance[i-period]
			avgVar := rollingSum / float64(period)
			result[i] = math.Sqrt(avgVar)
		} else {
			avgVar := rollingSum / float64(i+1)
			result[i] = math.Sqrt(avgVar)
		}
	}

	return result
}

// CalculateKaufmanER computes the Kaufman Efficiency Ratio (KER).
// KER = |Price(t) - Price(t-n)| / sum(|Price(i) - Price(i-1)|) for i in [t-n+1..t].
// KER approaches 1.0 during clean directional trends, and approaches 0.0 in noisy, choppy sideways markets.
func CalculateKaufmanER(closes []float64, period int) []float64 {
	n := len(closes)
	result := make([]float64, n)
	if n == 0 || period <= 0 {
		return result
	}

	diffs := make([]float64, n)
	for i := 1; i < n; i++ {
		diffs[i] = math.Abs(closes[i] - closes[i-1])
	}

	for i := 0; i < n; i++ {
		if i < period {
			// Window not yet full: calculate with available history
			if i == 0 {
				result[i] = 0
				continue
			}
			direction := math.Abs(closes[i] - closes[0])
			var path float64
			for k := 1; k <= i; k++ {
				path += diffs[k]
			}
			if path > 1e-9 {
				result[i] = math.Min(1.0, direction/path)
			} else {
				result[i] = 0
			}
			continue
		}

		direction := math.Abs(closes[i] - closes[i-period])
		var path float64
		for k := i - period + 1; k <= i; k++ {
			path += diffs[k]
		}

		if path > 1e-9 {
			er := direction / path
			if er > 1.0 {
				er = 1.0
			}
			result[i] = er
		} else {
			result[i] = 0
		}
	}

	return result
}

// CalculateCMF computes the Chaikin Money Flow over a given period (typically 20).
// CMF = sum(CLV * Volume) / sum(Volume), where CLV = ((Close - Low) - (High - Close)) / (High - Low).
// Values > +0.05 indicate institutional accumulation; values < -0.05 indicate distribution.
func CalculateCMF(candles []db.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 || period <= 0 {
		return result
	}

	mfv := make([]float64, n)
	vols := make([]float64, n)

	for i, c := range candles {
		rangeHL := c.High - c.Low
		if rangeHL > 1e-9 {
			clv := ((c.Close - c.Low) - (c.High - c.Close)) / rangeHL
			mfv[i] = clv * c.Volume
		} else {
			mfv[i] = 0
		}
		vols[i] = c.Volume
	}

	var sumMFV, sumVol float64
	for i := 0; i < n; i++ {
		sumMFV += mfv[i]
		sumVol += vols[i]

		if i >= period {
			sumMFV -= mfv[i-period]
			sumVol -= vols[i-period]
		}

		if sumVol > 1e-9 {
			cmf := sumMFV / sumVol
			if cmf > 1.0 {
				cmf = 1.0
			} else if cmf < -1.0 {
				cmf = -1.0
			}
			result[i] = cmf
		} else {
			result[i] = 0
		}
	}

	return result
}

// CalculateNATR calculates the Normalized Average True Range as a percentage of close price.
// NATR = (ATR / Close) * 100.
// This enables uniform volatility comparison across assets of drastically different price scales.
func CalculateNATR(candles []db.Candle, period int) []float64 {
	n := len(candles)
	result := make([]float64, n)
	if n == 0 || period <= 0 {
		return result
	}

	atr := CalculateATR(candles, period)
	for i := 0; i < n; i++ {
		if candles[i].Close > 1e-9 {
			result[i] = (atr[i] / candles[i].Close) * 100.0
		} else {
			result[i] = 0
		}
	}
	return result
}
