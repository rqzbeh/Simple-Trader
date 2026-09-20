package indicators_test

import (
	"math"
	"testing"

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
	}

	snap := indicators.Snapshot{
		RSI:             65.0,
		MACDHistogram:   0.8,
		SuperTrendTrend: "BULL",
	}

	score, direction := indicators.CalculateConfluence(snap, weights)
	if direction != "BUY" {
		t.Errorf("expected BUY direction, got %s", direction)
	}
	if score <= 0.5 {
		t.Errorf("expected strong score > 0.5, got %f", score)
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

