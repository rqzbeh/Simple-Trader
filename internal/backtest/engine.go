package backtest

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/indicators"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// Candle represents an OHLCV historical market bar.
type Candle struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// BacktestConfig configures the vectorized historical backtest run.
type BacktestConfig struct {
	Symbol            string                `json:"symbol"`
	InitialCapital    float64               `json:"initial_capital"`
	Friction          trader.FrictionModel  `json:"friction"`
	Kelly             trader.KellyConfig    `json:"kelly"`
	IndicatorsWeights map[string]float64    `json:"indicator_weights"`
	RiskFreeRate      float64               `json:"risk_free_rate"` // Annualized, e.g. 0.04 (4%)
}

// TradeRecord stores individual trade performance from the backtest simulation.
type TradeRecord struct {
	EntryTime    time.Time `json:"entry_time"`
	ExitTime     time.Time `json:"exit_time"`
	Side         string    `json:"side"`
	EntryPrice   float64   `json:"entry_price"`
	ExitPrice    float64   `json:"exit_price"`
	Size         float64   `json:"size"`
	NetPnL       float64   `json:"net_pnl"`
	ReturnPct    float64   `json:"return_pct"`
	ExitReason   string    `json:"exit_reason"`
	FeePaid      float64   `json:"fee_paid"`
	SlippagePaid float64   `json:"slippage_paid"`
}

// BacktestResult summarizes strategy performance across historical candles.
type BacktestResult struct {
	TotalTrades       int           `json:"total_trades"`
	WinningTrades     int           `json:"winning_trades"`
	LosingTrades      int           `json:"losing_trades"`
	WinRate           float64       `json:"win_rate"`
	TotalReturnPct    float64       `json:"total_return_pct"`
	EndingCapital     float64       `json:"ending_capital"`
	MaxDrawdownPct    float64       `json:"max_drawdown_pct"`
	SharpeRatio       float64       `json:"sharpe_ratio"`
	SortinoRatio      float64       `json:"sortino_ratio"`
	ProfitFactor      float64       `json:"profit_factor"`
	AvgTradeReturnPct float64       `json:"avg_trade_return_pct"`
	Trades            []TradeRecord `json:"trades"`
	EquityCurve       []float64     `json:"equity_curve"`
	ExecutionDuration time.Duration `json:"execution_duration"`
}

// MonteCarloResult holds results of 1,000 bootstrap simulations.
type MonteCarloResult struct {
	Iterations           int     `json:"iterations"`
	MeanReturnPct        float64 `json:"mean_return_pct"`
	MedianReturnPct      float64 `json:"median_return_pct"`
	Percentile5thReturn  float64 `json:"percentile_5th_return"`
	Percentile95thReturn float64 `json:"percentile_95th_return"`
	MaxDrawdown95thPct   float64 `json:"max_drawdown_95th_pct"`
	MaxDrawdown99thPct   float64 `json:"max_drawdown_99th_pct"`
	ProbabilityOfRuinPct float64 `json:"probability_of_ruin_pct"` // Equity falling below 50% of initial
}

// VectorizedEngine executes high-speed vectorized backtests and Monte Carlo simulations.
type VectorizedEngine struct {
	config BacktestConfig
}

// NewVectorizedEngine creates a backtest engine instance.
func NewVectorizedEngine(cfg BacktestConfig) *VectorizedEngine {
	if cfg.InitialCapital <= 0 {
		cfg.InitialCapital = 100000.0
	}
	if cfg.RiskFreeRate == 0 {
		cfg.RiskFreeRate = 0.04
	}
	return &VectorizedEngine{
		config: cfg,
	}
}

// Run executes the vectorized historical backtest against a slice of candles.
// Conforms to SC-004: evaluates 10,000 historical bars in < 100 milliseconds.
func (e *VectorizedEngine) Run(candles []Candle) *BacktestResult {
	startTime := time.Now()

	n := len(candles)
	if n < 50 {
		return &BacktestResult{
			EndingCapital:     e.config.InitialCapital,
			ExecutionDuration: time.Since(startTime),
		}
	}

	// Pre-extract close and high/low slices for indicator vectorization
	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	dbCandles := make([]db.Candle, n)
	for i := 0; i < n; i++ {
		closes[i] = candles[i].Close
		highs[i] = candles[i].High
		lows[i] = candles[i].Low
		dbCandles[i] = db.Candle{
			OpenTime: candles[i].Timestamp,
			Open:     candles[i].Open,
			High:     candles[i].High,
			Low:      candles[i].Low,
			Close:    candles[i].Close,
			Volume:   candles[i].Volume,
		}
	}

	// Vectorized technical indicators calculation
	rsis := indicators.CalculateRSI(closes, 14)
	macdRes := indicators.CalculateMACD(closes, 12, 26, 9)
	stRes := indicators.CalculateSuperTrend(dbCandles, 10, 3.0)
	atrs := indicators.CalculateATR(dbCandles, 14)
	gkVol := indicators.CalculateGarmanKlass(dbCandles, 14)
	parkVol := indicators.CalculateParkinson(dbCandles, 14)
	ker := indicators.CalculateKaufmanER(closes, 10)
	cmf := indicators.CalculateCMF(dbCandles, 20)
	natr := indicators.CalculateNATR(dbCandles, 14)
	regimeClassifier := indicators.NewRegimeClassifier(14, 50)

	capital := e.config.InitialCapital
	peakCapital := capital
	maxDrawdown := 0.0

	var trades []TradeRecord
	equityCurve := make([]float64, 0, n)
	equityCurve = append(equityCurve, capital)

	var activePosition *TradeRecord

	// Evaluate bars sequentially
	for i := 30; i < n; i++ {
		currentPrice := closes[i]
		currentTime := candles[i].Timestamp

		// 1. Check open position exit rules (Take Profit or Stop Loss)
		if activePosition != nil {
			exited := false
			var exitPrice float64
			var reason string

			// Dynamic ATR-based SL/TP using live indicator data
			atrSL := atrs[i] * 1.5 // 1.5x ATR for stop loss
			atrTP := atrs[i] * 3.0 // 3.0x ATR for take profit (2:1 R:R)
			if atrSL <= 0 {
				atrSL = activePosition.EntryPrice * 0.015
			}
			if atrTP <= 0 {
				atrTP = activePosition.EntryPrice * 0.030
			}

			if activePosition.Side == "BUY" {
				sl := activePosition.EntryPrice - atrSL
				tp := activePosition.EntryPrice + atrTP
				if lows[i] <= sl {
					exitPrice = sl
					reason = "STOP_LOSS"
					exited = true
				} else if highs[i] >= tp {
					exitPrice = tp
					reason = "TAKE_PROFIT"
					exited = true
				}
			} else if activePosition.Side == "SELL" {
				sl := activePosition.EntryPrice + atrSL
				tp := activePosition.EntryPrice - atrTP
				if highs[i] >= sl {
					exitPrice = sl
					reason = "STOP_LOSS"
					exited = true
				} else if lows[i] <= tp {
					exitPrice = tp
					reason = "TAKE_PROFIT"
					exited = true
				}
			}

			if exited {
				exitSide := "SELL"
				if activePosition.Side == "SELL" {
					exitSide = "BUY"
				}

				exitQuote := e.config.Friction.CalculateExecution(
					exitSide,
					activePosition.Size,
					exitPrice,
					0.0002, // 2 bps spread
					100.0,
					true,
				)

				var grossPnL float64
				if activePosition.Side == "BUY" {
					grossPnL = (exitQuote.EffectivePrice - activePosition.EntryPrice) * activePosition.Size
				} else {
					grossPnL = (activePosition.EntryPrice - exitQuote.EffectivePrice) * activePosition.Size
				}

				netPnL := grossPnL - (exitQuote.TotalFee + exitQuote.Slippage)
				returnPct := netPnL / (activePosition.EntryPrice * activePosition.Size)

				activePosition.ExitTime = currentTime
				activePosition.ExitPrice = exitQuote.EffectivePrice
				activePosition.NetPnL = netPnL
				activePosition.ReturnPct = returnPct
				activePosition.ExitReason = reason
				activePosition.FeePaid += exitQuote.TotalFee
				activePosition.SlippagePaid += exitQuote.Slippage

				capital += netPnL
				trades = append(trades, *activePosition)
				activePosition = nil
			}
		}

		// Update Drawdown tracking
		if capital > peakCapital {
			peakCapital = capital
		}
		dd := (peakCapital - capital) / peakCapital
		if dd > maxDrawdown {
			maxDrawdown = dd
		}
		equityCurve = append(equityCurve, capital)

		// 2. Evaluate confluence for trade entry if flat
		if activePosition == nil && i < n-1 {
			// Multi-factor regime classification incorporating volatility and Kaufman ER
			regime, volRatio := regimeClassifier.ClassifyMultiFactorRegime(atrs[i], atrs[:i+1], ker[i], cmf[i])

			snapshot := indicators.Snapshot{
				RSI:             rsis[i],
				MACDHistogram:   macdRes.Histogram[i],
				SuperTrendTrend: stRes.Trend[i],
				OBI:             0.0, // No order book data in historical backtest
				Regime:          regime,
				VolRatio:        volRatio,
				GarmanKlass:     gkVol[i],
				Parkinson:       parkVol[i],
				KaufmanER:       ker[i],
				CMF:             cmf[i],
				NATR:            natr[i],
			}
			score, side := indicators.CalculateConfluence(snapshot, e.config.IndicatorsWeights)

			if score >= 0.25 && side != "NEUTRAL" {
				// Dynamic win probability derived from confluence confidence
				winProb := 0.50 + (score * 0.15) // Higher confluence → higher estimated win prob
				if winProb > 0.70 {
					winProb = 0.70
				}

				// ATR-based dynamic SL/TP for position sizing
				slDist := atrs[i] * 1.5
				tpDist := atrs[i] * 3.0
				if slDist <= 0 {
					slDist = currentPrice * 0.015
				}
				if tpDist <= 0 {
					tpDist = currentPrice * 0.030
				}
				payoffRatio := tpDist / slDist

				riskFraction := trader.CalculateHalfKelly(e.config.Kelly, winProb, payoffRatio)

				dollarRisk := capital * riskFraction
				positionSize := dollarRisk / slDist

				// Cap position size to maximum 20% of capital
				maxUnits := (capital * 0.20) / currentPrice
				if positionSize > maxUnits {
					positionSize = maxUnits
				}

				entryQuote := e.config.Friction.CalculateExecution(
					side,
					positionSize,
					currentPrice,
					0.0002,
					100.0,
					true,
				)

				activePosition = &TradeRecord{
					EntryTime:    currentTime,
					Side:         side,
					EntryPrice:   entryQuote.EffectivePrice,
					Size:         positionSize,
					FeePaid:      entryQuote.TotalFee,
					SlippagePaid: entryQuote.Slippage,
				}
			}
		}
	}

	duration := time.Since(startTime)

	// Compute statistics
	totalTrades := len(trades)
	winning := 0
	losing := 0
	grossProfit := 0.0
	grossLoss := 0.0
	returns := make([]float64, 0, totalTrades)

	for _, t := range trades {
		returns = append(returns, t.ReturnPct)
		if t.NetPnL > 0 {
			winning++
			grossProfit += t.NetPnL
		} else {
			losing++
			grossLoss += math.Abs(t.NetPnL)
		}
	}

	winRate := 0.0
	if totalTrades > 0 {
		winRate = float64(winning) / float64(totalTrades)
	}

	profitFactor := 0.0
	if grossLoss > 0 {
		profitFactor = grossProfit / grossLoss
	} else if grossProfit > 0 {
		profitFactor = 99.9
	}

	totalReturnPct := (capital - e.config.InitialCapital) / e.config.InitialCapital
	sharpe := CalculateSharpeRatio(returns, e.config.RiskFreeRate)
	sortino := CalculateSortinoRatio(returns, e.config.RiskFreeRate)

	avgReturn := 0.0
	if len(returns) > 0 {
		var sum float64
		for _, r := range returns {
			sum += r
		}
		avgReturn = sum / float64(len(returns))
	}

	return &BacktestResult{
		TotalTrades:       totalTrades,
		WinningTrades:     winning,
		LosingTrades:      losing,
		WinRate:           winRate,
		TotalReturnPct:    totalReturnPct,
		EndingCapital:     capital,
		MaxDrawdownPct:    maxDrawdown,
		SharpeRatio:       sharpe,
		SortinoRatio:      sortino,
		ProfitFactor:      profitFactor,
		AvgTradeReturnPct: avgReturn,
		Trades:            trades,
		EquityCurve:       equityCurve,
		ExecutionDuration: duration,
	}
}

// RunMonteCarlo executes 1,000 bootstrap simulations over trade returns.
// Fulfills FR-008: 95th/99th percentile Drawdown and Probability of Ruin.
func RunMonteCarlo(trades []TradeRecord, initialCapital float64, iterations int, seed int64) *MonteCarloResult {
	if iterations <= 0 {
		iterations = 1000
	}
	if initialCapital <= 0 {
		initialCapital = 100000.0
	}

	n := len(trades)
	if n < 5 {
		return &MonteCarloResult{
			Iterations: iterations,
		}
	}

	var rng *rand.Rand
	if seed != 0 {
		rng = rand.New(rand.NewSource(seed))
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	finalReturns := make([]float64, iterations)
	maxDrawdowns := make([]float64, iterations)
	ruinCount := 0
	ruinThreshold := initialCapital * 0.50 // Ruin defined as 50% portfolio loss

	for iter := 0; iter < iterations; iter++ {
		simCapital := initialCapital
		peak := simCapital
		simMaxDD := 0.0
		hitRuin := false

		// Resample trades with replacement for n periods
		for step := 0; step < n; step++ {
			randIdx := rng.Intn(n)
			t := trades[randIdx]

			// Re-apply trade net PnL scaled to current simulated equity
			simCapital += simCapital * t.ReturnPct

			if simCapital > peak {
				peak = simCapital
			}
			dd := (peak - simCapital) / peak
			if dd > simMaxDD {
				simMaxDD = dd
			}

			if simCapital <= ruinThreshold {
				hitRuin = true
			}
		}

		if hitRuin {
			ruinCount++
		}

		finalReturns[iter] = (simCapital - initialCapital) / initialCapital
		maxDrawdowns[iter] = simMaxDD
	}

	sort.Float64s(finalReturns)
	sort.Float64s(maxDrawdowns)

	var totalReturn float64
	for _, ret := range finalReturns {
		totalReturn += ret
	}
	meanReturn := totalReturn / float64(iterations)
	medianReturn := finalReturns[iterations/2]
	p5Return := finalReturns[int(float64(iterations)*0.05)]
	p95Return := finalReturns[int(float64(iterations)*0.95)]

	// 95th and 99th percentile worst drawdowns
	p95DD := maxDrawdowns[int(float64(iterations)*0.95)]
	p99DD := maxDrawdowns[int(float64(iterations)*0.99)]

	probRuin := (float64(ruinCount) / float64(iterations)) * 100.0

	return &MonteCarloResult{
		Iterations:           iterations,
		MeanReturnPct:        meanReturn,
		MedianReturnPct:      medianReturn,
		Percentile5thReturn:  p5Return,
		Percentile95thReturn: p95Return,
		MaxDrawdown95thPct:   p95DD,
		MaxDrawdown99thPct:   p99DD,
		ProbabilityOfRuinPct: probRuin,
	}
}

// CalculateSharpeRatio computes annualized Sharpe Ratio from per-trade return percentages.
func CalculateSharpeRatio(returns []float64, riskFreeAnnual float64) float64 {
	if len(returns) < 2 {
		return 0.0
	}

	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	var varianceSum float64
	for _, r := range returns {
		varianceSum += (r - mean) * (r - mean)
	}
	stdDev := math.Sqrt(varianceSum / float64(len(returns)-1))

	if stdDev <= 0 {
		return 0.0
	}

	// Assuming ~252 trading periods/trades annualized
	rfPerTrade := riskFreeAnnual / 252.0
	sharpe := (mean - rfPerTrade) / stdDev * math.Sqrt(252.0)
	return math.Round(sharpe*1000) / 1000
}

// CalculateSortinoRatio computes annualized Sortino Ratio focusing on downside volatility.
func CalculateSortinoRatio(returns []float64, riskFreeAnnual float64) float64 {
	if len(returns) < 2 {
		return 0.0
	}

	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	rfPerTrade := riskFreeAnnual / 252.0

	var downsideSum float64
	downsideCount := 0
	for _, r := range returns {
		diff := r - rfPerTrade
		if diff < 0 {
			downsideSum += diff * diff
			downsideCount++
		}
	}

	if downsideCount == 0 || downsideSum <= 0 {
		return 10.0 // Ceiling if no downside deviation
	}

	downsideDev := math.Sqrt(downsideSum / float64(len(returns)))
	if downsideDev <= 0 {
		return 0.0
	}

	sortino := (mean - rfPerTrade) / downsideDev * math.Sqrt(252.0)
	return math.Round(sortino*1000) / 1000
}
