package trader_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/trader"
)

func TestCapitalAllocator(t *testing.T) {
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02, // 2% max per trade
	})

	coreCap, alphaCap := allocator.GetAvailableBuckets()
	if coreCap != 60000.0 || alphaCap != 40000.0 {
		t.Errorf("expected 60k/40k split, got %f / %f", coreCap, alphaCap)
	}

	// Size calculation for Gold (CORE)
	goldSize := allocator.CalculatePositionSize("CORE", 2650.0, 0.015) // 1.5% SL
	if goldSize <= 0 {
		t.Errorf("expected positive position size, got %f", goldSize)
	}

	// Max risk for 100k total capital @ 2% is $2,000
	maxRisk := 100000.0 * 0.02
	actualRisk := goldSize * (2650.0 * 0.015)
	if actualRisk > maxRisk+1.0 {
		t.Errorf("risk %f exceeded maximum allowed %f", actualRisk, maxRisk)
	}
}

func TestDrawdownCircuitBreaker(t *testing.T) {
	cb := trader.NewCircuitBreaker(100000.0, 0.10) // 10% max drawdown threshold

	if cb.IsHalted() {
		t.Errorf("expected circuit breaker to be operational initially")
	}

	// Equity drops to 92,000 (8% drawdown) -> still active
	cb.UpdateEquity(92000.0)
	if cb.IsHalted() {
		t.Errorf("expected breaker to remain active at 8%% drawdown")
	}

	// Equity drops to 89,000 (11% drawdown) -> must trigger halt
	cb.UpdateEquity(89000.0)
	if !cb.IsHalted() {
		t.Errorf("expected circuit breaker to HALT trading at 11%% drawdown")
	}
	if cb.CurrentDrawdownPct() < 10.0 {
		t.Errorf("expected drawdown >= 10%%, got %f", cb.CurrentDrawdownPct())
	}
}

func TestFrictionModel(t *testing.T) {
	friction := trader.DefaultFrictionModel()

	// 10 BTC buy order at $60,000 with 100 BTC depth, 2 bps spread
	quote := friction.CalculateExecution("BUY", 10.0, 60000.0, 0.0002, 100.0, true)

	// Half spread: 60,000 * 0.0001 = 6.0
	if quote.HalfSpread != 6.0 {
		t.Errorf("expected half spread 6.0, got %f", quote.HalfSpread)
	}

	// Liquidity ratio = 10 / 100 = 0.1. Slippage = 60000 * 0.05 * 0.01 = 30.0
	if math.Abs(quote.Slippage-30.0) > 1e-6 {
		t.Errorf("expected slippage 30.0, got %f", quote.Slippage)
	}

	// Effective price BUY: 60000 + 6.0 + 30.0 = 60036.0
	expectedPrice := 60036.0
	if math.Abs(quote.EffectivePrice-expectedPrice) > 1e-6 {
		t.Errorf("expected effective price %f, got %f", expectedPrice, quote.EffectivePrice)
	}

	// Taker fee rate = 0.0005. Gross = 10 * 60036 = 600360. Fee = 600360 * 0.0005 = 300.18
	expectedFee := 600360.0 * 0.0005
	if quote.TotalFee != expectedFee {
		t.Errorf("expected fee %f, got %f", expectedFee, quote.TotalFee)
	}
	if quote.TotalNetCost != (600360.0 + expectedFee) {
		t.Errorf("expected net cost %f, got %f", 600360.0+expectedFee, quote.TotalNetCost)
	}

	// SELL quote test
	sellQuote := friction.CalculateExecution("SELL", 10.0, 60000.0, 0.0002, 100.0, true)
	expectedSellPrice := 60000.0 - 6.0 - 30.0
	if sellQuote.EffectivePrice != expectedSellPrice {
		t.Errorf("expected sell price %f, got %f", expectedSellPrice, sellQuote.EffectivePrice)
	}
}

func TestPaperExecutionEngineWithFriction(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)

	// Execute paper buy order with spread & slippage
	order, err := engine.ExecuteOrder(context.Background(), trader.OrderRequest{
		Symbol:            "BTC/USD",
		Bucket:            "ALPHA",
		Side:              "BUY",
		Price:             60000.0,
		PositionSize:      0.5,
		StopLoss:          58000.0,
		TakeProfit:        65000.0,
		AIReasoning:       "Bullish confluence",
		SymbolSpreadPct:   0.0002,
		AvailableDepthQty: 100.0,
		IsTaker:           true,
	})

	if err != nil {
		t.Fatalf("failed executing order: %v", err)
	}

	if order.Status != "OPEN" {
		t.Errorf("expected OPEN status, got %s", order.Status)
	}
	if order.EntryPrice <= 60000.0 {
		t.Errorf("expected entry price > 60000 due to friction, got %f", order.EntryPrice)
	}
	if order.ExecutionFee <= 0 {
		t.Errorf("expected non-zero execution fee, got %f", order.ExecutionFee)
	}

	// Simulate price reaching Take Profit
	closedTrade, closed := engine.CheckExit("BTC/USD", 65500.0)
	if !closed {
		t.Fatalf("expected trade to be closed at Take Profit")
	}

	if closedTrade.Status != "CLOSED" {
		t.Errorf("expected CLOSED status, got %s", closedTrade.Status)
	}
	if closedTrade.RealizedPnL <= 0 {
		t.Errorf("expected positive PnL on TP hit, got %f", closedTrade.RealizedPnL)
	}
	if closedTrade.ExitPrice >= 65500.0 {
		t.Errorf("expected exit price < 65500 due to sell friction, got %f", closedTrade.ExitPrice)
	}
}

// Mock providers for daemon testing
type mockMarketData struct {
	prices map[string]float64
}

func (m *mockMarketData) GetLatestPrice(symbol string) (float64, error) {
	p, ok := m.prices[symbol]
	if !ok {
		return 0, fmt.Errorf("symbol not found")
	}
	return p, nil
}

func (m *mockMarketData) GetMarketDepth(symbol string) (float64, float64, error) {
	return 0.0002, 100.0, nil
}

// The daemon is a position guardian, not an entry path. Entries come only from
// the news-gated signal pipeline, so an open position can never exist without a
// matching catalyst record in futures_trade_signals. This guards against
// reintroducing the silent order path that opened positions with no signal.
func TestTradingDaemonNeverOpensPositions(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	})
	cb := trader.NewCircuitBreaker(100000.0, 0.10)

	prices := map[string]float64{
		"BTC/USD": 65000.0,
		"XAU/USD": 2650.0,
	}
	market := &mockMarketData{prices: prices}

	cfg := trader.DaemonConfig{
		TickInterval: 10 * time.Millisecond,
		Symbols:      []string{"BTC/USD", "XAU/USD"},
	}
	daemon := trader.NewTradingDaemon(cfg, engine, allocator, cb, market)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := daemon.Start(ctx); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	if !daemon.IsRunning() {
		t.Error("expected daemon to be running")
	}

	// Sweep prices hard in both directions to tempt any technical entry path.
	for i := 0; i < 20; i++ {
		prices["BTC/USD"] = 65000.0 * (1.0 + float64(i%7)/10.0)
		prices["XAU/USD"] = 2650.0 * (1.0 - float64(i%5)/10.0)
		daemon.ProcessTick(ctx)
	}

	daemon.Stop()
	if daemon.IsRunning() {
		t.Error("expected daemon to be stopped")
	}

	if equity := engine.GetTotalEquity(prices); equity <= 0 {
		t.Errorf("expected valid equity, got %f", equity)
	}
	if trades := engine.GetOpenTrades(); len(trades) != 0 {
		t.Errorf("daemon must never open positions (entries belong to the signal pipeline), got %d open", len(trades))
	}
	if closed := engine.GetClosedTrades(); len(closed) != 0 {
		t.Errorf("daemon must never execute orders, got %d closed trades", len(closed))
	}
}

// The daemon still owns exits: a seeded position must close when price
// crosses its stop, freeing margin without waiting for a manual close.
func TestTradingDaemonClosesPositionOnStopLoss(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	})
	cb := trader.NewCircuitBreaker(100000.0, 0.10)

	prices := map[string]float64{"BTC/USD": 65000.0}
	market := &mockMarketData{prices: prices}

	ctx := context.Background()
	if _, err := engine.ExecuteOrder(ctx, trader.OrderRequest{
		Symbol:            "BTC/USD",
		Bucket:            "ALPHA",
		Side:              "BUY",
		Price:             65000.0,
		PositionSize:      0.1,
		StopLoss:          63000.0,
		TakeProfit:        70000.0,
		Leverage:          1,
		SymbolSpreadPct:   0.0002,
		AvailableDepthQty: 100.0,
		IsTaker:           true,
	}); err != nil {
		t.Fatalf("failed to seed position: %v", err)
	}

	cfg := trader.DaemonConfig{
		TickInterval: 10 * time.Millisecond,
		Symbols:      []string{"BTC/USD"},
	}
	daemon := trader.NewTradingDaemon(cfg, engine, allocator, cb, market)

	daemon.ProcessTick(ctx)
	if got := len(engine.GetOpenTrades()); got != 1 {
		t.Fatalf("expected position to stay open above stop, got %d", got)
	}

	// Through the stop: the daemon must close it on the next tick.
	prices["BTC/USD"] = 62000.0
	daemon.ProcessTick(ctx)

	if got := len(engine.GetOpenTrades()); got != 0 {
		t.Errorf("expected stop-loss exit, %d still open", got)
	}
	if got := len(engine.GetClosedTrades()); got != 1 {
		t.Errorf("expected 1 closed trade, got %d", got)
	}
	if equity := engine.GetTotalEquity(prices); equity <= 0 {
		t.Errorf("expected valid equity after exit, got %f", equity)
	}
}

// A manual close must release the position even when no SL/TP level is
// crossed. Closing a signal previously left its position open and its margin
// tied up, which then made every later signal report "capital fully reserved".
func TestForceClosePositionIgnoresLevels(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)
	ctx := context.Background()

	if _, err := engine.ExecuteOrder(ctx, trader.OrderRequest{
		Symbol:            "ETH/USDT",
		Bucket:            "ALPHA",
		Side:              "BUY",
		Price:             3000.0,
		PositionSize:      1.0,
		StopLoss:          2900.0,
		TakeProfit:        3300.0,
		Leverage:          5,
		SymbolSpreadPct:   0.0002,
		AvailableDepthQty: 100.0,
		IsTaker:           true,
	}); err != nil {
		t.Fatalf("failed to seed position: %v", err)
	}

	// Price sits between SL and TP: CheckExit must not close.
	if _, exited := engine.CheckExit("ETH/USDT", 3100.0); exited {
		t.Error("CheckExit should not close inside the SL/TP band")
	}
	if got := len(engine.GetOpenTrades()); got != 1 {
		t.Fatalf("expected position still open, got %d", got)
	}

	// Force close at a non-triggering price must still exit.
	closed, exited := engine.ForceClosePosition("ETH/USDT", 3100.0, "MANUAL_CLOSE")
	if !exited {
		t.Fatal("expected ForceClosePosition to close the position")
	}
	if closed.ExitReason != "MANUAL_CLOSE" {
		t.Errorf("expected MANUAL_CLOSE exit reason, got %q", closed.ExitReason)
	}
	if got := len(engine.GetOpenTrades()); got != 0 {
		t.Errorf("expected no open positions after force close, got %d", got)
	}
	if got := len(engine.GetClosedTrades()); got != 1 {
		t.Errorf("expected 1 closed trade, got %d", got)
	}
	if engine.GetCash() <= 0 {
		t.Errorf("expected margin released back to cash, got %f", engine.GetCash())
	}
}
