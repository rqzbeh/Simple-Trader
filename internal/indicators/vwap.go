package indicators

import (
	"github.com/rqzbeh/simple-trader/internal/db"
)

// CalculateVWAP calculates the Volume-Weighted Average Price across candles.
func CalculateVWAP(candles []db.Candle) []float64 {
	n := len(candles)
	vwap := make([]float64, n)
	if n == 0 {
		return vwap
	}

	var cumulativeTPV float64
	var cumulativeVolume float64

	for i, c := range candles {
		typicalPrice := (c.High + c.Low + c.Close) / 3.0
		cumulativeTPV += typicalPrice * c.Volume
		cumulativeVolume += c.Volume

		if cumulativeVolume > 0 {
			vwap[i] = cumulativeTPV / cumulativeVolume
		} else {
			vwap[i] = typicalPrice
		}
	}

	return vwap
}
