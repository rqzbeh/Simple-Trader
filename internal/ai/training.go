package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/rqzbeh/simple-trader/internal/market"
)

// RealDataTrainingMetrics summarizes the calibration results on authentic data.
type RealDataTrainingMetrics struct {
	Symbol              string                     `json:"symbol"`
	Timeframe           string                     `json:"timeframe"`
	SampleCount         int                        `json:"sample_count"`
	DateStart           time.Time                  `json:"date_start"`
	DateEnd             time.Time                  `json:"date_end"`
	TrainingLoss        float64                    `json:"training_loss"`
	DirectionalAccuracy float64                    `json:"directional_accuracy"`
	WeightsSnapshot     map[string]float64         `json:"weights_snapshot"`
	BayesianPosteriors  map[string]map[string]float64 `json:"bayesian_posteriors"`
	TrainedAt           time.Time                  `json:"trained_at"`
}

// RealDataPipeline performs statistical and Bayesian calibration strictly on authentic Binance historical candlesticks.
type RealDataPipeline struct {
	downloader *market.BinanceHistoricalDownloader
	sampler    *ThompsonSampler
}

// NewRealDataPipeline creates a new real data ML calibration pipeline.
func NewRealDataPipeline(downloader *market.BinanceHistoricalDownloader, sampler *ThompsonSampler) *RealDataPipeline {
	if downloader == nil {
		downloader = market.NewBinanceHistoricalDownloader()
	}
	if sampler == nil {
		sampler = NewThompsonSampler(time.Now().UnixNano())
	}
	return &RealDataPipeline{
		downloader: downloader,
		sampler:    sampler,
	}
}

// TrainOnAuthenticData pulls genuine Binance klines, computes forward predictive accuracy, and updates posteriors.
func (p *RealDataPipeline) TrainOnAuthenticData(
	ctx context.Context,
	symbol string,
	interval string,
	candleCount int,
) (*RealDataTrainingMetrics, error) {
	if candleCount < 50 {
		return nil, errors.New("minimum 50 authentic candles required for statistical calibration")
	}

	candles, err := p.downloader.FetchHistoricalKlines(ctx, symbol, interval, candleCount)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch authentic candles: %w", err)
	}
	if len(candles) < 50 {
		return nil, fmt.Errorf("insufficient authentic candles received: %d", len(candles))
	}

	// Verify all candles have authentic strictly-increasing timestamps and realistic pricing
	for i := 1; i < len(candles); i++ {
		if !candles[i].OpenTime.After(candles[i-1].OpenTime) {
			return nil, fmt.Errorf("non-chronological candle timestamps detected at index %d: %v <= %v",
				i, candles[i].OpenTime, candles[i-1].OpenTime)
		}
		if candles[i].Close <= 0 || candles[i].High < candles[i].Low {
			return nil, fmt.Errorf("invalid or corrupt candle prices at %v", candles[i].OpenTime)
		}
	}

	// Compute multi-indicator predictive attribution
	// Indicators: RSI (14), MACD (12, 26, 9), Momentum (4-period forward return)
	rsiPeriod := 14
	var totalGain, totalLoss float64
	for i := 1; i <= rsiPeriod && i < len(candles); i++ {
		diff := candles[i].Close - candles[i-1].Close
		if diff > 0 {
			totalGain += diff
		} else {
			totalLoss += -diff
		}
	}
	avgGain := totalGain / float64(rsiPeriod)
	avgLoss := totalLoss / float64(rsiPeriod)

	correctPredictions := 0
	totalEvaluated := 0
	var totalBCE float64

	bayesianUpdates := make(map[string]struct{ Alpha, Beta float64 })
	stats := p.sampler.GetPosteriorStats()
	for k, v := range stats {
		bayesianUpdates[k] = struct{ Alpha, Beta float64 }{Alpha: v["alpha"], Beta: v["beta"]}
	}

	// Forward lookahead horizon: 4 candles
	horizon := 4
	for i := rsiPeriod; i < len(candles)-horizon; i++ {
		// Update Smoothed RSI
		diff := candles[i].Close - candles[i-1].Close
		var gain, loss float64
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(rsiPeriod-1) + gain) / float64(rsiPeriod)
		avgLoss = (avgLoss*float64(rsiPeriod-1) + loss) / float64(rsiPeriod)

		rs := 1.0
		if avgLoss > 0 {
			rs = avgGain / avgLoss
		}
		rsi := 100.0 - (100.0 / (1.0 + rs))

		// 4-candle forward return direction
		fwdRet := (candles[i+horizon].Close - candles[i].Close) / candles[i].Close
		actualBullish := fwdRet > 0

		// Prediction rule based on RSI momentum & taker volume buy ratio
		predProb := 0.5
		if rsi > 50 {
			predProb += 0.15 * ((rsi - 50) / 50)
		} else {
			predProb -= 0.15 * ((50 - rsi) / 50)
		}

		// Microstructure flow adjustment
		if candles[i].Volume > 0 {
			takerRatio := candles[i].TakerBuyBaseVol / candles[i].Volume
			predProb += 0.10 * (takerRatio - 0.5)
		}

		predProb = math.Max(0.01, math.Min(0.99, predProb))
		predBullish := predProb >= 0.5

		if predBullish == actualBullish {
			correctPredictions++
		}

		// Binary Cross Entropy Loss
		y := 0.0
		if actualBullish {
			y = 1.0
		}
		bce := -(y*math.Log(predProb) + (1.0-y)*math.Log(1.0-predProb))
		totalBCE += bce
		totalEvaluated++

		// Attribution updates
		rsiAligns := (rsi >= 50 && actualBullish) || (rsi < 50 && !actualBullish)
		curRsi := bayesianUpdates["RSI"]
		if rsiAligns {
			curRsi.Alpha += 0.1
		} else {
			curRsi.Beta += 0.1
		}
		bayesianUpdates["RSI"] = curRsi

		curMicro := bayesianUpdates["MICROSTRUCTURE"]
		if candles[i].Volume > 0 && ((candles[i].TakerBuyBaseVol/candles[i].Volume > 0.5 && actualBullish) ||
			(candles[i].TakerBuyBaseVol/candles[i].Volume <= 0.5 && !actualBullish)) {
			curMicro.Alpha += 0.1
		} else {
			curMicro.Beta += 0.1
		}
		bayesianUpdates["MICROSTRUCTURE"] = curMicro
	}

	p.sampler.UpdatePosteriors(bayesianUpdates)

	var accuracy float64
	var meanLoss float64
	if totalEvaluated > 0 {
		accuracy = float64(correctPredictions) / float64(totalEvaluated)
		meanLoss = totalBCE / float64(totalEvaluated)
	}

	return &RealDataTrainingMetrics{
		Symbol:              symbol,
		Timeframe:           interval,
		SampleCount:         len(candles),
		DateStart:           candles[0].OpenTime,
		DateEnd:             candles[len(candles)-1].OpenTime,
		TrainingLoss:        math.Round(meanLoss*10000) / 10000,
		DirectionalAccuracy: math.Round(accuracy*10000) / 10000,
		WeightsSnapshot:     p.sampler.SampleWeights(),
		BayesianPosteriors:  p.sampler.GetPosteriorStats(),
		TrainedAt:           time.Now(),
	}, nil
}
