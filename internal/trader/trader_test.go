package trader_test

import (
	"context"
	"testing"

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

func TestPaperExecutionEngine(t *testing.T) {
	engine := trader.NewExecutionEngine(100000.0)

	// Execute paper buy order
	order, err := engine.ExecuteOrder(context.Background(), trader.OrderRequest{
		Symbol:       "BTC/USD",
		Bucket:       "ALPHA",
		Side:         "BUY",
		Price:        68000.0,
		PositionSize: 0.5, // 0.5 BTC = $34,000
		StopLoss:     66980.0,
		TakeProfit:   70040.0,
		AIReasoning:  "Strong bullish momentum on SuperTrend breakout",
	})

	if err != nil {
		t.Fatalf("failed executing order: %v", err)
	}

	if order.Status != "OPEN" {
		t.Errorf("expected OPEN status, got %s", order.Status)
	}

	// Simulate price reaching Take Profit
	closedTrade, closed := engine.CheckExit("BTC/USD", 70100.0)
	if !closed {
		t.Fatalf("expected trade to be closed at Take Profit")
	}

	if closedTrade.Status != "CLOSED" {
		t.Errorf("expected CLOSED status, got %s", closedTrade.Status)
	}
	if closedTrade.RealizedPnL <= 0 {
		t.Errorf("expected positive PnL on TP hit, got %f", closedTrade.RealizedPnL)
	}
}
