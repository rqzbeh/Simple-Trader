package indicators

// CalculateMACD calculates the Moving Average Convergence Divergence, signal line, and histogram.
func CalculateMACD(closes []float64, fastPeriod, slowPeriod, signalPeriod int) MACDResult {
	n := len(closes)
	res := MACDResult{
		MACD:      make([]float64, n),
		Signal:    make([]float64, n),
		Histogram: make([]float64, n),
	}
	if n == 0 {
		return res
	}

	fastEMA := CalculateEMA(closes, fastPeriod)
	slowEMA := CalculateEMA(closes, slowPeriod)

	for i := 0; i < n; i++ {
		res.MACD[i] = fastEMA[i] - slowEMA[i]
	}

	res.Signal = CalculateEMA(res.MACD, signalPeriod)

	for i := 0; i < n; i++ {
		res.Histogram[i] = res.MACD[i] - res.Signal[i]
	}

	return res
}
