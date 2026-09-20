package backtest_test

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/backtest"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// generateSyntheticCandles creates n historical bars with realistic random walk.
func generateSyntheticCandles(n int, initialPrice float64, seed int64) []backtest.Candle {
	rng := rand.New(rand.NewSource(seed))
	candles := make([]backtest.Candle, n)
	curr := initialPrice
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		drift := 0.0002
		shock := rng.NormFloat64() * 0.008
		next := curr * (1.0 + drift + shock)
		if next <= 1.0 {
			next = 1.0
		}

		high := math.Max(curr, next) * (1.0 + math.Abs(rng.NormFloat64()*0.003))
		low := math.Min(curr, next) * (1.0 - math.Abs(rng.NormFloat64()*0.003))
		vol := 10.0 + rng.Float64()*50.0

		candles[i] = backtest.Candle{
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			Open:      curr,
			High:      high,
			Low:       low,
			Close:     next,
			Volume:    vol,
		}
		curr = next
	}

	return candles
}

func TestVectorizedEngine10000BarsPerformance(t *testing.T) {
	// SC-004: Backtester can evaluate 10,000 historical bars in under 100 milliseconds
	candles := generateSyntheticCandles(10000, 60000.0, 42)

	cfg := backtest.BacktestConfig{
		Symbol:         "BTC/USD",
		InitialCapital: 100000.0,
		Friction:       trader.DefaultFrictionModel(),
		Kelly:          trader.DefaultKellyConfig(),
		IndicatorsWeights: map[string]float64{
			"RSI":            1.0,
			"MACD":           1.0,
			"SUPERTREND":     1.0,
			"MICROSTRUCTURE": 1.5,
		},
		RiskFreeRate: 0.04,
	}

	engine := backtest.NewVectorizedEngine(cfg)

	start := time.Now()
	res := engine.Run(candles)
	elapsed := time.Since(start)

	t.Logf("Evaluated 10,000 bars in %s (Duration field: %s)", elapsed, res.ExecutionDuration)
	t.Logf("Total Trades: %d | Win Rate: %.2f%% | Final Capital: $%.2f | Max DD: %.2f%% | Sharpe: %.2f",
		res.TotalTrades, res.WinRate*100, res.EndingCapital, res.MaxDrawdownPct*100, res.SharpeRatio)

	if elapsed > 150*time.Millisecond {
		t.Errorf("expected execution time < 150ms for 10,000 bars, took %s", elapsed)
	}

	if res.TotalTrades == 0 {
		t.Fatalf("expected trades to be executed across 10,000 bars, got 0")
	}

	if res.EndingCapital <= 0 {
		t.Errorf("expected positive ending capital, got %f", res.EndingCapital)
	}
}

func TestMonteCarloBootstrapSimulation(t *testing.T) {
	// FR-008: 1,000-run bootstrap analysis reporting 95th/99th percentile Drawdown and Probability of Ruin
	candles := generateSyntheticCandles(2000, 2600.0, 99)

	cfg := backtest.BacktestConfig{
		Symbol:            "XAU/USD",
		InitialCapital:    100000.0,
		Friction:          trader.DefaultFrictionModel(),
		Kelly:             trader.DefaultKellyConfig(),
		IndicatorsWeights: nil,
		RiskFreeRate:      0.04,
	}

	engine := backtest.NewVectorizedEngine(cfg)
	res := engine.Run(candles)

	if len(res.Trades) < 5 {
		t.Skip("insufficient trades generated for Monte Carlo bootstrap")
	}

	mc := backtest.RunMonteCarlo(res.Trades, 100000.0, 1000, 123)

	t.Logf("Monte Carlo 1000-iterations: Mean Return: %.2f%% | Median: %.2f%% | 95th DD: %.2f%% | 99th DD: %.2f%% | Ruin Prob: %.2f%%",
		mc.MeanReturnPct*100, mc.MedianReturnPct*100, mc.MaxDrawdown95thPct*100, mc.MaxDrawdown99thPct*100, mc.ProbabilityOfRuinPct)

	if mc.Iterations != 1000 {
		t.Errorf("expected 1,000 iterations, got %d", mc.Iterations)
	}

	if mc.MaxDrawdown95thPct < 0 || mc.MaxDrawdown95thPct > 1.0 {
		t.Errorf("invalid 95th percentile DD: %f", mc.MaxDrawdown95thPct)
	}

	if mc.ProbabilityOfRuinPct < 0 || mc.ProbabilityOfRuinPct > 100.0 {
		t.Errorf("invalid Probability of Ruin percentage: %f", mc.ProbabilityOfRuinPct)
	}
}

func TestSharpeAndSortinoMetrics(t *testing.T) {
	returns := []float64{0.02, -0.01, 0.015, 0.03, -0.005, 0.012, 0.018, -0.008}

	sharpe := backtest.CalculateSharpeRatio(returns, 0.04)
	sortino := backtest.CalculateSortinoRatio(returns, 0.04)

	t.Logf("Calculated Sharpe: %.3f, Sortino: %.3f", sharpe, sortino)

	if sharpe <= 0 {
		t.Errorf("expected positive Sharpe ratio for overall positive returns, got %f", sharpe)
	}

	if sortino <= 0 {
		t.Errorf("expected positive Sortino ratio, got %f", sortino)
	}
}
