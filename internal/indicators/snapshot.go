package indicators

import (
	"math"
	"sync"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// BuildSnapshot processes authentic historical candles and computes a unified Snapshot
// containing all institutional indicators concurrently across pipeline stages.
func BuildSnapshot(symbol string, candles []db.Candle, weights map[string]float64) Snapshot {
	n := len(candles)
	if n == 0 {
		return Snapshot{
			Symbol:          symbol,
			SuperTrendTrend: "NEUTRAL",
			Regime:          RegimeNormalTrending,
		}
	}

	latest := candles[n-1]
	closes := make([]float64, n)
	for i, c := range candles {
		closes[i] = c.Close
	}

	snap := Snapshot{
		Symbol: symbol,
		Price:  latest.Close,
	}

	var atrs []float64
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		computeTrendOscillators(&snap, closes, candles)
	}()

	go func() {
		defer wg.Done()
		atrs = computeVolatilityEstimators(&snap, closes, candles)
	}()

	go func() {
		defer wg.Done()
		computeVolumeFlowAndMicrostructure(&snap, candles)
	}()

	wg.Wait()

	// Volatility Regime Classification
	rc := NewRegimeClassifier(14, 50)
	snap.Regime, snap.VolRatio = rc.ClassifyRegime(snap.ATR, atrs)

	// Volume expansion ratio
	snap.VolumeRatio = computeVolumeRatio(candles)

	// Dynamic Confluence Score & Suggested Direction
	confScore, suggestedDir := CalculateConfluence(snap, weights)
	snap.Price = math.Round(snap.Price*10000) / 10000
	snap.RSI = math.Round(snap.RSI*100) / 100
	snap.MACD = math.Round(snap.MACD*10000) / 10000
	snap.MACDSignal = math.Round(snap.MACDSignal*10000) / 10000
	snap.MACDHistogram = math.Round(snap.MACDHistogram*10000) / 10000
	snap.ConfluenceScore = math.Round(confScore*10000) / 10000
	snap.SuggestedDirection = suggestedDir

	return snap
}

func computeTrendOscillators(snap *Snapshot, closes []float64, candles []db.Candle) {
	// 1. RSI (14)
	if rsis := CalculateRSI(closes, 14); len(rsis) > 0 {
		snap.RSI = rsis[len(rsis)-1]
	}

	// 2. MACD (12, 26, 9)
	macdRes := CalculateMACD(closes, 12, 26, 9)
	if len(macdRes.MACD) > 0 {
		lastIdx := len(macdRes.MACD) - 1
		snap.MACD = macdRes.MACD[lastIdx]
		snap.MACDSignal = macdRes.Signal[lastIdx]
		snap.MACDHistogram = macdRes.Histogram[lastIdx]
	}

	// 3. SuperTrend (10, 3.0)
	stRes := CalculateSuperTrend(candles, 10, 3.0)
	if len(stRes.Value) > 0 {
		lastIdx := len(stRes.Value) - 1
		snap.SuperTrendVal = stRes.Value[lastIdx]
		snap.SuperTrendTrend = stRes.Trend[lastIdx]
	}

	// 4. Bollinger Bands (20, 2.0)
	bbRes := CalculateBollinger(closes, 20, 2.0)
	if len(bbRes.Upper) > 0 {
		lastIdx := len(bbRes.Upper) - 1
		snap.UpperBand = bbRes.Upper[lastIdx]
		snap.MiddleBand = bbRes.Middle[lastIdx]
		snap.LowerBand = bbRes.Lower[lastIdx]
	}
}

func computeVolatilityEstimators(snap *Snapshot, closes []float64, candles []db.Candle) []float64 {
	// ATR (14)
	atrs := CalculateATR(candles, 14)
	if len(atrs) > 0 {
		snap.ATR = atrs[len(atrs)-1]
		if snap.Price > 0 {
			snap.NATR = snap.ATR / snap.Price
		}
	}

	// Garman-Klass & Parkinson
	if gks := CalculateGarmanKlass(candles, 14); len(gks) > 0 {
		snap.GarmanKlass = gks[len(gks)-1]
	}
	if parks := CalculateParkinson(candles, 14); len(parks) > 0 {
		snap.Parkinson = parks[len(parks)-1]
	}

	// Kaufman Efficiency Ratio (10)
	if kers := CalculateKaufmanER(closes, 10); len(kers) > 0 {
		snap.KaufmanER = kers[len(kers)-1]
	}

	return atrs
}

func computeVolumeFlowAndMicrostructure(snap *Snapshot, candles []db.Candle) {
	n := len(candles)
	// Chaikin Money Flow (20)
	if cmfs := CalculateCMF(candles, 20); len(cmfs) > 0 {
		snap.CMF = cmfs[len(cmfs)-1]
	}

	// VWAP
	if vwaps := CalculateVWAP(candles); len(vwaps) > 0 {
		snap.VWAP = vwaps[len(vwaps)-1]
	}

	// Microstructure & CVD Divergence from candle flow
	var buyerVol, sellerVol float64
	prices := getFloatSlice(n)
	cvdSeries := getFloatSlice(n)
	defer func() {
		putFloatSlice(prices)
		putFloatSlice(cvdSeries)
	}()

	var runningCVD float64
	for i, c := range candles {
		prices[i] = c.Close
		hl := c.High - c.Low
		if hl > 0 {
			buyRatio := (c.Close - c.Low) / hl
			sellRatio := (c.High - c.Close) / hl
			runningCVD += (c.Volume * buyRatio) - (c.Volume * sellRatio)
		}
		cvdSeries[i] = runningCVD
	}

	if n > 0 {
		lastCandle := candles[n-1]
		hl := lastCandle.High - lastCandle.Low
		if hl > 0 {
			buyerVol = lastCandle.Volume * ((lastCandle.Close - lastCandle.Low) / hl)
			sellerVol = lastCandle.Volume * ((lastCandle.High - lastCandle.Close) / hl)
		}
		totalVol := buyerVol + sellerVol
		if totalVol > 0 {
			snap.OBI = (buyerVol - sellerVol) / totalVol
		}
		snap.CVD = runningCVD
		snap.Divergence = DetectDivergence(prices, cvdSeries)
	}
}

func computeVolumeRatio(candles []db.Candle) float64 {
	n := len(candles)
	if n < 21 {
		return 0
	}
	var sum float64
	for _, c := range candles[n-21 : n-1] {
		sum += c.Volume
	}
	if avg := sum / 20.0; avg > 0 {
		return candles[n-1].Volume / avg
	}
	return 0
}
