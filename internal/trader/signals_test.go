package trader_test

import (
	"context"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// MockAIClient implements trader.AIAnalyzer
type MockAIClient struct {
	Response *ai.DecisionResponse
	Err      error
}

func (m *MockAIClient) Analyze(ctx context.Context, req ai.DecisionRequest) (*ai.DecisionResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Response, nil
}

func TestSignalService_EvaluateMarketSignal(t *testing.T) {
	ctx := context.Background()

	// Test 1: High conviction Bullish news catalyst -> LONG signal with R:R >= 1.5
	mockAI := &MockAIClient{
		Response: &ai.DecisionResponse{
			Decision:               "BUY",
			Confidence:             0.85,
			Reasoning:              "Institutional capital inflow catalyst confirmed by momentum.",
			Catalyst:               "BlackRock spot ETF reports record $1.1B single-day inflow.",
			Leverage:               5,
			AllocationPct:          1.5,
			SuggestedStopLossPct:   1.5,
			SuggestedTakeProfitPct: 3.5,
		},
	}

	service := trader.NewSignalService(nil, mockAI)

	quote := cache.TickerQuote{
		Symbol: "BTC/USD",
		Price:  60000.0,
	}
	snap := cache.IndicatorSnapshot{
		Symbol:     "BTC/USD",
		RSI:        58.0,
		SuperTrend: "BULL",
	}
	headlines := []string{"BlackRock spot ETF reports record $1.1B single-day inflow."}

	sig, err := service.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, headlines, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatalf("expected active signal, got nil")
	}

	if sig.Direction != "LONG" {
		t.Errorf("expected direction LONG, got %s", sig.Direction)
	}
	if sig.Leverage != 5 {
		t.Errorf("expected leverage 5, got %d", sig.Leverage)
	}
	if sig.RiskRewardRatio < 1.5 {
		t.Errorf("expected R:R >= 1.5, got %f", sig.RiskRewardRatio)
	}
	if sig.StopLoss >= sig.EntryPrice {
		t.Errorf("long stop loss must be below entry price: SL=%.2f, Entry=%.2f", sig.StopLoss, sig.EntryPrice)
	}
	if sig.TakeProfit1 <= sig.EntryPrice {
		t.Errorf("long take profit must be above entry price: TP=%.2f, Entry=%.2f", sig.TakeProfit1, sig.EntryPrice)
	}

	// Test 2: Bearish news catalyst -> SHORT signal
	mockAIBear := &MockAIClient{
		Response: &ai.DecisionResponse{
			Decision:               "SELL",
			Confidence:             0.90,
			Reasoning:              "Whale exchange dump of 45,000 BTC triggers liquidation cascade.",
			Catalyst:               "Whale deposit of 45k BTC to Binance detected.",
			Leverage:               4,
			AllocationPct:          2.0,
			SuggestedStopLossPct:   2.0,
			SuggestedTakeProfitPct: 4.5,
		},
	}
	serviceBear := trader.NewSignalService(nil, mockAIBear)

	sigBear, err := serviceBear.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, []string{"Whale deposit of 45k BTC to Binance detected."}, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sigBear == nil {
		t.Fatalf("expected active signal, got nil")
	}
	if sigBear.Direction != "SHORT" {
		t.Errorf("expected direction SHORT, got %s", sigBear.Direction)
	}
	if sigBear.StopLoss <= sigBear.EntryPrice {
		t.Errorf("short stop loss must be above entry: SL=%.2f, Entry=%.2f", sigBear.StopLoss, sigBear.EntryPrice)
	}
	if sigBear.TakeProfit1 >= sigBear.EntryPrice {
		t.Errorf("short take profit must be below entry: TP=%.2f, Entry=%.2f", sigBear.TakeProfit1, sigBear.EntryPrice)
	}

	// Test 3: HOLD output when no catalyst
	mockAIHold := &MockAIClient{
		Response: &ai.DecisionResponse{
			Decision:  "HOLD",
			Reasoning: "No catalyst",
		},
	}
	serviceHold := trader.NewSignalService(nil, mockAIHold)
	sigHold, err := serviceHold.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, nil, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error on hold: %v", err)
	}
	if sigHold != nil {
		t.Errorf("expected nil signal on HOLD, got %+v", sigHold)
	}
}

func TestSignalService_CheckSignalResolution(t *testing.T) {
	ctx := context.Background()
	service := trader.NewSignalService(nil, nil)

	// Long signal resolution on Take Profit
	longSig := &db.FuturesTradeSignal{
		ID:                  101,
		Symbol:              "BTC/USD",
		Direction:           "LONG",
		Status:              "ACTIVE",
		EntryPrice:          60000.0,
		StopLoss:            58000.0,
		TakeProfit1:         64000.0,
		Leverage:            5,
		AllocatedCapitalUSD: 2000.0, // Position value = $10,000 (0.1667 BTC)
	}

	// Price below TP -> should not resolve
	resolved, _, _, _, err := service.CheckSignalResolution(ctx, longSig, 62000.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved {
		t.Errorf("signal should not be resolved at 62000")
	}

	// Price hits Take Profit
	resolved, reason, pnl, roi, err := service.CheckSignalResolution(ctx, longSig, 64500.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved {
		t.Fatalf("expected signal to resolve at 64500")
	}
	if reason != "TAKE_PROFIT" {
		t.Errorf("expected exit reason TAKE_PROFIT, got %s", reason)
	}
	if pnl <= 0 {
		t.Errorf("expected positive PnL on TP hit, got %.2f", pnl)
	}
	if roi <= 0 {
		t.Errorf("expected positive ROI on TP hit, got %.2f%%", roi)
	}
}

func TestSignalService_DynamicConfigEnforcement(t *testing.T) {
	ctx := context.Background()

	customCfg := trader.SignalConfig{
		MinRiskRewardRatio: 3.0,
		DefaultLeverage:    10,
		MinStopLossPct:     1.0,
		MaxStopLossPct:     3.0,
		MinTakeProfitPct:   3.0,
		MaxTakeProfitPct:   12.0,
		MaxRiskPerTradePct: 0.02,
	}

	mockAI := &MockAIClient{
		Response: &ai.DecisionResponse{
			Decision:               "BUY",
			Confidence:             0.95,
			Reasoning:              "High momentum surge breaking historical resistance.",
			Catalyst:               "Federal Reserve announces liquidity easing window.",
			Leverage:               15, // AI requests 15, should be bounded to DefaultLeverage (10)
			SuggestedStopLossPct:   0.2, // Below MinStopLossPct (1.0), should be adjusted
			SuggestedTakeProfitPct: 2.0, // Low TP, will be forced by MinRiskRewardRatio (3.0)
		},
	}

	service := trader.NewSignalService(nil, mockAI, customCfg)

	quote := cache.TickerQuote{Symbol: "ETH/USDT", Price: 3000.0}
	snap := cache.IndicatorSnapshot{Symbol: "ETH/USDT", RSI: 62.0}

	sig, err := service.EvaluateMarketSignal(
		ctx, "ETH/USDT", "ALPHA", quote, snap, nil, []string{"Federal Reserve liquidity announcement"}, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatalf("expected non-nil signal")
	}

	if sig.Leverage != 10 {
		t.Errorf("expected leverage bounded to 10, got %d", sig.Leverage)
	}
	if sig.RiskRewardRatio < 3.0 {
		t.Errorf("expected dynamic R:R >= 3.0, got %f", sig.RiskRewardRatio)
	}
}
