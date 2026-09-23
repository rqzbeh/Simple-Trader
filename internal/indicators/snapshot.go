package indicators

import (
	"math"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// BuildSnapshot processes authentic historical candles and computes a unified Snapshot
// containing all institutional indicators (RSI, MACD, SuperTrend, Bollinger Bands, ATR,
// Garman-Klass, Parkinson, Kaufman Efficiency Ratio, Chaikin Money Flow, Market Regime,
// and Confluence Score).
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

	// 5. ATR (14)
	atrs := CalculateATR(candles, 14)
	if len(atrs) > 0 {
		snap.ATR = atrs[len(atrs)-1]
		if snap.Price > 0 {
			snap.NATR = snap.ATR / snap.Price
		}
	}

	// 6. Institutional Extreme-Value Volatility (Garman-Klass & Parkinson)
	gks := CalculateGarmanKlass(candles, 14)
	if len(gks) > 0 {
		snap.GarmanKlass = gks[len(gks)-1]
	}

	parks := CalculateParkinson(candles, 14)
	if len(parks) > 0 {
		snap.Parkinson = parks[len(parks)-1]
	}

	// 7. Kaufman Efficiency Ratio (10)
	kers := CalculateKaufmanER(closes, 10)
	if len(kers) > 0 {
		snap.KaufmanER = kers[len(kers)-1]
	}

	// 8. Chaikin Money Flow (20)
	cmfs := CalculateCMF(candles, 20)
	if len(cmfs) > 0 {
		snap.CMF = cmfs[len(cmfs)-1]
	}

	// 9. VWAP
	vwaps := CalculateVWAP(candles)
	if len(vwaps) > 0 {
		snap.VWAP = vwaps[len(vwaps)-1]
	}

	// 10. Microstructure & CVD Divergence from candle flow
	var buyerVol, sellerVol float64
	prices := make([]float64, n)
	cvdSeries := make([]float64, n)
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

	// 11. Volatility Regime Classification
	rc := NewRegimeClassifier(14, 50)
	regime, volRatio := rc.ClassifyRegime(snap.ATR, atrs)
	snap.Regime = regime
	snap.VolRatio = volRatio

	// 12. Dynamic Confluence Score & Suggested Direction
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
