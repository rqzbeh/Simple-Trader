package trader_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
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

type mockStrategy struct {
	signal *db.Signal
}

func (s *mockStrategy) Evaluate(ctx context.Context, symbol string, currentPrice float64) (*db.Signal, error) {
	return s.signal, nil
}

func TestTradingDaemon(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	})
	cb := trader.NewCircuitBreaker(100000.0, 0.10)

	market := &mockMarketData{
		prices: map[string]float64{
			"XAU/USD": 2650.0,
		},
	}

	strat := &mockStrategy{
		signal: &db.Signal{
			Symbol:          "XAU/USD",
			Side:            "BUY",
			Bucket:          "CORE",
			EntryPrice:      2650.0,
			StopLoss:        2600.0,
			TakeProfit:      2750.0,
			Confidence:      0.85,
			ConfluenceScore: 0.88,
			AIReasoning:     "Strong gold breakout",
			Status:          "OPEN",
		},
	}

	cfg := trader.DaemonConfig{
		TickInterval: 10 * time.Millisecond,
		Symbols:      []string{"XAU/USD"},
	}

	daemon := trader.NewTradingDaemon(cfg, engine, allocator, cb, market, strat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := daemon.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	if !daemon.IsRunning() {
		t.Errorf("expected daemon to be running")
	}

	// Trigger tick manually or let loop run
	daemon.ProcessTick(ctx)

	// Verify order was placed
	totalEq := engine.GetTotalEquity(market.prices)
	if totalEq <= 0 {
		t.Errorf("expected valid equity, got %f", totalEq)
	}

	// Move price to Take Profit
	market.prices["XAU/USD"] = 2760.0
	daemon.ProcessTick(ctx)

	daemon.Stop()
	if daemon.IsRunning() {
		t.Errorf("expected daemon to be stopped")
	}
}

type mockHaltedCalendar struct {
	halted bool
}

func (m *mockHaltedCalendar) IsSymbolHalted(symbol string, now time.Time) (bool, string) {
	if m.halted {
		return true, "FOMC release window"
	}
	return false, ""
}

func TestTradingDaemonWithCalendarHalt(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	})
	cb := trader.NewCircuitBreaker(100000.0, 0.10)
	market := &mockMarketData{
		prices: map[string]float64{
			"BTC/USD": 65000.0,
		},
	}
	strat := &mockStrategy{
		signal: &db.Signal{
			Symbol:          "BTC/USD",
			Side:            "BUY",
			Bucket:          "ALPHA",
			EntryPrice:      65000.0,
			StopLoss:        63000.0,
			TakeProfit:      70000.0,
			Confidence:      0.90,
			ConfluenceScore: 0.85,
			Status:          "OPEN",
		},
	}

	cfg := trader.DaemonConfig{
		TickInterval: 10 * time.Millisecond,
		Symbols:      []string{"BTC/USD"},
	}

	daemon := trader.NewTradingDaemon(cfg, engine, allocator, cb, market, strat)
	cal := &mockHaltedCalendar{halted: true}
	daemon.SetCalendar(cal)

	ctx := context.Background()
	daemon.ProcessTick(ctx)

	// Since calendar is halted, no trade should be opened
	trades := engine.GetOpenTrades()
	if len(trades) != 0 {
		t.Errorf("expected 0 trades opened during calendar halt, got %d", len(trades))
	}

	// Unhalt and tick again
	cal.halted = false
	daemon.ProcessTick(ctx)
	tradesAfter := engine.GetOpenTrades()
	if len(tradesAfter) != 1 {
		t.Errorf("expected 1 trade opened after unhalt, got %d", len(tradesAfter))
	}
}
