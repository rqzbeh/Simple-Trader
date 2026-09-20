package indicators

import (
	"math"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// CalculateATR computes the Average True Range for a series of historical candles.
func CalculateATR(candles []db.Candle, period int) []float64 {
	n := len(candles)
	atr := make([]float64, n)
	if n == 0 || period <= 0 {
		return atr
	}

	tr := make([]float64, n)
	tr[0] = candles[0].High - candles[0].Low

	for i := 1; i < n; i++ {
		hl := candles[i].High - candles[i].Low
		hc := math.Abs(candles[i].High - candles[i-1].Close)
		lc := math.Abs(candles[i].Low - candles[i-1].Close)
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}

	if n <= period {
		var sum float64
		for _, v := range tr {
			sum += v
		}
		avg := sum / float64(n)
		for i := range atr {
			atr[i] = avg
		}
		return atr
	}

	var initialSum float64
	for i := 0; i < period; i++ {
		initialSum += tr[i]
	}
	atr[period-1] = initialSum / float64(period)

	for i := 0; i < period-1; i++ {
		atr[i] = tr[i]
	}

	for i := period; i < n; i++ {
		atr[i] = (atr[i-1]*float64(period-1) + tr[i]) / float64(period)
	}

	return atr
}
