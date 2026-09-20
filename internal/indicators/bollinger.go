package indicators

import "math"

// CalculateBollinger calculates Bollinger Bands (Upper, Middle, Lower).
func CalculateBollinger(closes []float64, period int, multiplier float64) BollingerResult {
	n := len(closes)
	res := BollingerResult{
		Upper:  make([]float64, n),
		Middle: make([]float64, n),
		Lower:  make([]float64, n),
	}
	if n == 0 || period <= 0 {
		return res
	}

	for i := 0; i < n; i++ {
		start := i - period + 1
		if start < 0 {
			start = 0
		}
		count := float64(i - start + 1)

		var sum float64
		for j := start; j <= i; j++ {
			sum += closes[j]
		}
		mean := sum / count
		res.Middle[i] = mean

		var sumSqDiff float64
		for j := start; j <= i; j++ {
			diff := closes[j] - mean
			sumSqDiff += diff * diff
		}
		variance := sumSqDiff / count
		stdDev := math.Sqrt(variance)

		res.Upper[i] = mean + (multiplier * stdDev)
		res.Lower[i] = mean - (multiplier * stdDev)
	}

	return res
}
