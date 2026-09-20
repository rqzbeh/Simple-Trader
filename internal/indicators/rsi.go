package indicators

// CalculateRSI computes the Relative Strength Index with standard Wilder's smoothing.
func CalculateRSI(closes []float64, period int) []float64 {
	n := len(closes)
	rsi := make([]float64, n)
	if n <= period || period <= 0 {
		return rsi
	}

	var gains, losses float64
	for i := 1; i <= period; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		rsi[period] = 100
	} else {
		rs := avgGain / avgLoss
		rsi[period] = 100 - (100 / (1 + rs))
	}

	for i := period + 1; i < n; i++ {
		change := closes[i] - closes[i-1]
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}

		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			rsi[i] = 100
		} else {
			rs := avgGain / avgLoss
			rsi[i] = 100 - (100 / (1 + rs))
		}
	}

	return rsi
}

// CalculateEMA computes the Exponential Moving Average series for a given period.
func CalculateEMA(data []float64, period int) []float64 {
	n := len(data)
	ema := make([]float64, n)
	if n == 0 || period <= 0 {
		return ema
	}

	multiplier := 2.0 / float64(period+1)

	// First valid EMA is the SMA of the initial period
	if n < period {
		var sum float64
		for _, v := range data {
			sum += v
		}
		avg := sum / float64(n)
		for i := range ema {
			ema[i] = avg
		}
		return ema
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	ema[period-1] = sum / float64(period)

	for i := 0; i < period-1; i++ {
		ema[i] = data[i]
	}

	for i := period; i < n; i++ {
		ema[i] = (data[i]-ema[i-1])*multiplier + ema[i-1]
	}

	return ema
}
