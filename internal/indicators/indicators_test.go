package indicators_test

import (
	"math"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/indicators"
)

func TestRSI(t *testing.T) {
	// Monotonically increasing closes should produce RSI > 80
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = float64(100 + i*5)
	}

	rsi := indicators.CalculateRSI(closes, 14)
	lastRSI := rsi[len(rsi)-1]
	if lastRSI < 80.0 {
		t.Errorf("expected high RSI on steep uptrend, got %f", lastRSI)
	}

	// Monotonically decreasing closes should produce RSI < 20
	decreasing := make([]float64, 30)
	for i := range decreasing {
		decreasing[i] = float64(300 - i*5)
	}
	rsiDown := indicators.CalculateRSI(decreasing, 14)
	lastRSIDown := rsiDown[len(rsiDown)-1]
	if lastRSIDown > 20.0 {
		t.Errorf("expected low RSI on steep downtrend, got %f", lastRSIDown)
	}
}

func TestMACD(t *testing.T) {
	closes := make([]float64, 60)
	for i := range closes {
		closes[i] = 100.0 + math.Sin(float64(i))*10.0
	}

	res := indicators.CalculateMACD(closes, 12, 26, 9)
	if len(res.MACD) != len(closes) || len(res.Signal) != len(closes) || len(res.Histogram) != len(closes) {
		t.Errorf("expected MACD outputs to match input length")
	}
}

func TestBollingerBands(t *testing.T) {
	closes := []float64{
		10, 11, 10, 12, 10, 11, 10, 12, 10, 11,
		10, 12, 10, 11, 10, 12, 10, 11, 10, 12,
	}
	bb := indicators.CalculateBollinger(closes, 10, 2.0)
	lastIdx := len(closes) - 1

	if bb.Upper[lastIdx] <= bb.Middle[lastIdx] {
		t.Errorf("upper band must be strictly greater than middle band")
	}
	if bb.Lower[lastIdx] >= bb.Middle[lastIdx] {
		t.Errorf("lower band must be strictly less than middle band")
	}
}

func TestSuperTrend(t *testing.T) {
	candles := make([]db.Candle, 40)
	for i := range candles {
		p := float64(100 + i*2)
		candles[i] = db.Candle{
			High:  p + 1.0,
			Low:   p - 1.0,
			Close: p,
			Open:  p - 0.5,
		}
	}

	res := indicators.CalculateSuperTrend(candles, 10, 3.0)
	lastTrend := res.Trend[len(res.Trend)-1]
	if lastTrend != "BULL" {
		t.Errorf("expected BULL trend on steady uptrend, got %s", lastTrend)
	}
}

func TestConfluenceScore(t *testing.T) {
	weights := map[string]float64{
		"RSI":        1.2,
		"MACD":       1.0,
		"SUPERTREND": 1.5,
		"CMF":        1.2,
		"KER":        1.0,
	}

	snap := indicators.Snapshot{
		RSI:             65.0,
		MACDHistogram:   0.8,
		SuperTrendTrend: "BULL",
		CMF:             0.12,
		KaufmanER:       0.65,
	}

	score, direction := indicators.CalculateConfluence(snap, weights)
	if direction != "BUY" {
		t.Errorf("expected BUY direction, got %s", direction)
	}
	if score <= 0.3 {
		t.Errorf("expected strong score > 0.3, got %f", score)
	}
}

func TestOrderBookImbalance(t *testing.T) {
	bids := []indicators.OrderBookLevel{
		{Price: 60000, Quantity: 70.0},
		{Price: 59990, Quantity: 30.0},
	}
	asks := []indicators.OrderBookLevel{
		{Price: 60010, Quantity: 20.0},
		{Price: 60020, Quantity: 10.0},
	}

	obi := indicators.CalculateOBI(bids, asks)
	// (100 - 30) / (100 + 30) = 70 / 130 = ~0.53846
	expected := 70.0 / 130.0
	if math.Abs(obi-expected) > 0.001 {
		t.Errorf("expected OBI %f, got %f", expected, obi)
	}

	// Empty book test
	emptyOBI := indicators.CalculateOBI(nil, nil)
	if emptyOBI != 0.0 {
		t.Errorf("expected 0.0 for empty book, got %f", emptyOBI)
	}
}

func TestCVDAndDivergence(t *testing.T) {
	cvd := indicators.NewCVDTracker(10)

	cvd.Update(10.0, 5.0)  // +5
	cvd.Update(20.0, 10.0) // +15
	cvd.Update(15.0, 5.0)  // +25
	val := cvd.Update(30.0, 10.0) // +45

	if val != 45.0 {
		t.Errorf("expected cumulative delta 45.0, got %f", val)
	}

	// Test Bullish Absorption: Price makes lower low, CVD makes higher low
	prices := []float64{100.0, 99.0, 98.0, 97.0}
	cvdVals := []float64{10.0, 15.0, 20.0, 25.0}

	div := indicators.DetectDivergence(prices, cvdVals)
	if div != indicators.DivergenceBullishAbsorption {
		t.Errorf("expected Bullish Absorption, got %s", div)
	}

	// Test Bearish Exhaustion: Price makes higher high, CVD makes lower high
	pricesBear := []float64{100.0, 101.0, 102.0, 103.0}
	cvdBear := []float64{50.0, 40.0, 30.0, 20.0}

	divBear := indicators.DetectDivergence(pricesBear, cvdBear)
	if divBear != indicators.DivergenceBearishExhaustion {
		t.Errorf("expected Bearish Exhaustion, got %s", divBear)
	}
}

func TestRegimeClassifier(t *testing.T) {
	rc := indicators.NewRegimeClassifier(14, 5)

	// Historical ATRs around 10.0
	hist := []float64{10.0, 10.0, 10.0, 10.0, 10.0}

	// Case 1: Low Vol Consolidation (ratio < 0.70)
	regime, ratio := rc.ClassifyRegime(6.0, hist)
	if regime != indicators.RegimeLowVolMeanReversion {
		t.Errorf("expected LOW_VOL_CONSOLIDATION, got %s (ratio=%f)", regime, ratio)
	}

	// Case 2: High Vol Chop (ratio > 1.30)
	regimeHV, ratioHV := rc.ClassifyRegime(15.0, hist)
	if regimeHV != indicators.RegimeHighVolChop {
		t.Errorf("expected HIGH_VOL_CHOP, got %s (ratio=%f)", regimeHV, ratioHV)
	}

	// Case 3: Normal Trending
	regimeNorm, ratioNorm := rc.ClassifyRegime(10.5, hist)
	if regimeNorm != indicators.RegimeNormalTrending {
		t.Errorf("expected NORMAL_TRENDING, got %s (ratio=%f)", regimeNorm, ratioNorm)
	}

	// Multiplier tests
	mHigh := indicators.CalculateSizingMultiplier(indicators.RegimeHighVolChop, 2.0)
	if mHigh != 0.5 {
		t.Errorf("expected 0.5 multiplier for ratio 2.0 in chop, got %f", mHigh)
	}
}

func TestATR(t *testing.T) {
	candles := make([]db.Candle, 20)
	for i := range candles {
		candles[i] = db.Candle{
			High:  105,
			Low:   95,
			Close: 100,
			Open:  100,
		}
	}
	atr := indicators.CalculateATR(candles, 14)
	if len(atr) != len(candles) {
		t.Fatalf("expected ATR length %d, got %d", len(candles), len(atr))
	}
	if atr[len(atr)-1] <= 0 {
		t.Errorf("expected positive ATR, got %f", atr[len(atr)-1])
	}
}

func TestVWAP(t *testing.T) {
	candles := []db.Candle{
		{High: 10, Low: 10, Close: 10, Volume: 100},
		{High: 20, Low: 20, Close: 20, Volume: 100},
	}
	vwap := indicators.CalculateVWAP(candles)
	if len(vwap) != 2 {
		t.Fatalf("expected 2 VWAP points, got %d", len(vwap))
	}
	if math.Abs(vwap[1]-15.0) > 0.001 {
		t.Errorf("expected VWAP 15.0, got %f", vwap[1])
	}
}

func TestInstitutionalIndicators(t *testing.T) {
	candles := make([]db.Candle, 30)
	for i := range candles {
		candles[i] = db.Candle{
			High:   100.0 + float64(i)*0.5 + 2.0,
			Low:    100.0 + float64(i)*0.5 - 1.5,
			Open:   100.0 + float64(i)*0.5,
			Close:  100.0 + float64(i)*0.5 + 1.0,
			Volume: 1000 + float64(i)*10,
		}
	}

	// 1. Garman-Klass
	gk := indicators.CalculateGarmanKlass(candles, 14)
	if len(gk) != len(candles) {
		t.Fatalf("expected GK length %d, got %d", len(candles), len(gk))
	}
	if gk[len(gk)-1] <= 0 {
		t.Errorf("expected positive Garman-Klass volatility, got %f", gk[len(gk)-1])
	}

	// 2. Parkinson
	pk := indicators.CalculateParkinson(candles, 14)
	if len(pk) != len(candles) {
		t.Fatalf("expected Parkinson length %d, got %d", len(candles), len(pk))
	}
	if pk[len(pk)-1] <= 0 {
		t.Errorf("expected positive Parkinson volatility, got %f", pk[len(pk)-1])
	}

	// 3. Kaufman ER
	closes := make([]float64, len(candles))
	for i := range candles {
		closes[i] = candles[i].Close
	}
	ker := indicators.CalculateKaufmanER(closes, 10)
	if len(ker) != len(closes) {
		t.Fatalf("expected KER length %d, got %d", len(closes), len(ker))
	}
	// Clean monotonic trend should yield high efficiency
	if ker[len(ker)-1] < 0.80 {
		t.Errorf("expected high efficiency for monotonic trend, got %f", ker[len(ker)-1])
	}

	// 4. Chaikin Money Flow (CMF)
	cmf := indicators.CalculateCMF(candles, 20)
	if len(cmf) != len(candles) {
		t.Fatalf("expected CMF length %d, got %d", len(candles), len(cmf))
	}
	// Close is near high, so CMF should be positive accumulation
	if cmf[len(cmf)-1] <= 0 {
		t.Errorf("expected positive CMF accumulation, got %f", cmf[len(cmf)-1])
	}

	// 5. NATR
	natr := indicators.CalculateNATR(candles, 14)
	if len(natr) != len(candles) {
		t.Fatalf("expected NATR length %d, got %d", len(candles), len(natr))
	}
	if natr[len(natr)-1] <= 0 {
		t.Errorf("expected positive NATR, got %f", natr[len(natr)-1])
	}
}

func TestBuildSnapshot(t *testing.T) {
	now := time.Now()
	var candles []db.Candle
	for i := 0; i < 50; i++ {
		p := 100.0 + float64(i)*0.5
		candles = append(candles, db.Candle{
			Symbol:    "BTC/USDT",
			Open:      p - 0.2,
			High:      p + 1.0,
			Low:       p - 1.0,
			Close:     p,
			Volume:    1000.0,
			OpenTime:  now.Add(time.Duration(i) * time.Hour),
		})
	}

	snap := indicators.BuildSnapshot("BTC/USDT", candles, map[string]float64{"RSI": 1.0, "MACD": 1.0})
	if snap.Symbol != "BTC/USDT" {
		t.Errorf("expected symbol BTC/USDT, got %s", snap.Symbol)
	}
	if snap.Price <= 0 {
		t.Errorf("expected positive price, got %f", snap.Price)
	}
	if snap.RSI <= 0 || snap.RSI > 100 {
		t.Errorf("expected RSI in (0, 100], got %f", snap.RSI)
	}
	if snap.ATR <= 0 {
		t.Errorf("expected positive ATR, got %f", snap.ATR)
	}
	if snap.GarmanKlass <= 0 {
		t.Errorf("expected positive GarmanKlass, got %f", snap.GarmanKlass)
	}
	if snap.ConfluenceScore < 0 || snap.ConfluenceScore > 1.0 {
		t.Errorf("expected ConfluenceScore in [0, 1], got %f", snap.ConfluenceScore)
	}
}



