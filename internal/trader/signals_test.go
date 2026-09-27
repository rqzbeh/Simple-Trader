package trader_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
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

	sig, _, err := service.EvaluateMarketSignal(
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

	sigBear, _, err := serviceBear.EvaluateMarketSignal(
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
	sigHold, decisionHold, err := serviceHold.EvaluateMarketSignal(
		ctx, "BTC/USD", "ALPHA", quote, snap, nil, nil, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error on hold: %v", err)
	}
	if sigHold != nil {
		t.Errorf("expected nil signal on HOLD, got %+v", sigHold)
	}
	// The AI decision must come back on HOLD: an operator cannot act on a
	// silent nil, so the reasoning has to reach logs and the API.
	if decisionHold == nil {
		t.Fatal("expected decision response on HOLD, got nil")
	}
	if decisionHold.Reasoning != "No catalyst" {
		t.Errorf("expected hold reasoning to be preserved, got %q", decisionHold.Reasoning)
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
			Leverage:               15,  // AI requests 15, should be bounded to DefaultLeverage (10)
			SuggestedStopLossPct:   0.2, // Below MinStopLossPct (1.0), should be adjusted
			SuggestedTakeProfitPct: 2.0, // Low TP, will be forced by MinRiskRewardRatio (3.0)
		},
	}

	service := trader.NewSignalService(nil, mockAI, customCfg)

	quote := cache.TickerQuote{Symbol: "ETH/USDT", Price: 3000.0}
	snap := cache.IndicatorSnapshot{Symbol: "ETH/USDT", RSI: 62.0}

	sig, _, err := service.EvaluateMarketSignal(
		ctx, "ETH/USDT", "ALPHA", quote, snap, nil, []string{"Federal Reserve liquidity announcement"}, 100000.0, 40000.0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatalf("expected non-nil signal")
	}

	if sig.Leverage > 10 {
		t.Errorf("expected leverage bounded to DefaultLeverage 10 (liq-buffer walk-down allowed), got %d", sig.Leverage)
	}
	if sig.Leverage < 1 {
		t.Errorf("leverage must stay >= 1, got %d", sig.Leverage)
	}
	// Staged exits: TP1 from the ATR model stretches to the configured R:R
	// floor against the effective stop (FR-003 contract).
	if sig.RiskRewardRatio < 3.0 {
		t.Errorf("expected dynamic R:R >= 3.0, got %f", sig.RiskRewardRatio)
	}
}

// --- Spec 012 US2: staged exits + decay (FR-004/005) ---

// fakeDecayStore records stop moves and decay-state transitions.
type fakeDecayStore struct {
	closedIDs []int64
	stops     map[int64]float64
	states    map[int64]string
}

func (f *fakeDecayStore) InsertFuturesSignal(context.Context, *db.FuturesTradeSignal) (*db.FuturesTradeSignal, error) {
	return nil, nil
}
func (f *fakeDecayStore) ListFuturesSignals(context.Context, string, int) ([]db.FuturesTradeSignal, error) {
	return nil, nil
}
func (f *fakeDecayStore) GetActiveFuturesSignalBySymbol(context.Context, string) (*db.FuturesTradeSignal, error) {
	return nil, nil
}
func (f *fakeDecayStore) CloseFuturesSignal(_ context.Context, id int64, _ float64, _ string, _, _ float64) error {
	f.closedIDs = append(f.closedIDs, id)
	return nil
}
func (f *fakeDecayStore) MarkSignalDispatched(context.Context, int64) error { return nil }
func (f *fakeDecayStore) MarkSignalResolved(context.Context, int64) error   { return nil }
func (f *fakeDecayStore) InsertEntryFilterLog(context.Context, string, string, *int64, string, json.RawMessage) error {
	return nil
}
func (f *fakeDecayStore) UpdateSignalStop(_ context.Context, id int64, newStop float64) error {
	f.stops[id] = newStop
	return nil
}
func (f *fakeDecayStore) UpdateSignalDecay(_ context.Context, id int64, state string) error {
	f.states[id] = state
	return nil
}

// TestCheckSignalResolutionStagedTP1Hit drives the TP1 partial-close staging
// (FR-004): a TP1 hit closes at TP1 with settled PnL/ROI; TP2 hit closes the
// runner. Signal struct carries TakeProfit2 from the ATR model.
func TestCheckSignalResolutionStagedTP1Hit(t *testing.T) {
	ctx := context.Background()
	service := trader.NewSignalService(nil, nil)

	tp2 := 102.7
	sig := &db.FuturesTradeSignal{
		ID:                  201,
		Symbol:              "BTC/USDT",
		Direction:           "LONG",
		Status:              "ACTIVE",
		EntryPrice:          100.0,
		StopLoss:            98.5, // 1.5R risk distance
		TakeProfit1:         101.5,
		TakeProfit2:         &tp2,
		Leverage:            5,
		AllocatedCapitalUSD: 4000.0,
		Profile:             "CRYPTO",
	}

	// Between TP1 and TP2: TP1 hit already consumed; behavior defined by the
	// level check on the runner target.
	resolved, reason, pnl, roi, err := service.CheckSignalResolution(ctx, sig, 101.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved {
		t.Errorf("price between entry and TP1 must not resolve")
	}

	// TP2 hit closes the runner.
	resolved, reason, pnl, roi, err = service.CheckSignalResolution(ctx, sig, 102.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved || reason != "TAKE_PROFIT" {
		t.Fatalf("expected TAKE_PROFIT at TP2, got resolved=%v reason=%q", resolved, reason)
	}
	if pnl <= 0 || roi <= 0 {
		t.Errorf("expected settled PnL/ROI at TP2, got pnl=%.2f roi=%.2f", pnl, roi)
	}
}

// TestEvaluateDecayCheckpointsInOrder locks the state-machine integration:
// a losing trade passes NONE -> BREAKEVEN (protect) -> CLOSED (flat) at the
// profile checkpoints; a winner rides to the hard horizon TIME_EXIT.
func TestEvaluateDecayCheckpointsInOrder(t *testing.T) {
	prof := config.DefaultCryptoProfile() // BE 30m, flat 40m, horizon 60m

	// Losing trade lifecycle
	store := &fakeDecayStore{stops: map[int64]float64{}, states: map[int64]string{}}
	sig := &db.FuturesTradeSignal{
		ID: 301, Symbol: "BTC/USDT", Direction: "LONG", Status: "ACTIVE",
		EntryPrice: 100, StopLoss: 99, TakeProfit1: 101.5,
		Leverage: 5, AllocatedCapitalUSD: 4000, Profile: "CRYPTO",
	}

	// age 10m, at entry: NONE, untouched
	state, action := trader.EvaluateDecay(10, 0.0, "NONE", prof)
	if state != "NONE" || action != "" {
		t.Fatalf("age 10: state=%q action=%q, want NONE/''", state, action)
	}

	// age 31m, +0.2R: protect
	state, action = trader.EvaluateDecay(31, 0.2, "NONE", prof)
	if state != "BREAKEVEN" || action != "BREAKEVEN" {
		t.Fatalf("age 31: state=%q action=%q, want BREAKEVEN/BREAKEVEN", state, action)
	}
	beStop := trader.BreakevenStopPrice(100, trader.DirectionLong, 0.001)
	if err := store.UpdateSignalStop(context.Background(), sig.ID, beStop); err != nil {
		t.Fatalf("stop move: %v", err)
	}
	if err := store.UpdateSignalDecay(context.Background(), sig.ID, state); err != nil {
		t.Fatalf("decay persist: %v", err)
	}

	// age 41m, still below entry: flat checkpoint closes
	state, action = trader.EvaluateDecay(41, -0.2, "BREAKEVEN", prof)
	if state != "CLOSED" || action != "CLOSE" {
		t.Fatalf("age 41: state=%q action=%q, want CLOSED/CLOSE", state, action)
	}
	if len(store.closedIDs) != 0 {
		t.Errorf("decide close is the reconciler's job; store untouched by math: %v", store.closedIDs)
	}

	// Winning trade: rides to the hard horizon.
	state, action = trader.EvaluateDecay(50, 1.5, "BREAKEVEN", prof)
	if state != "BREAKEVEN" {
		t.Errorf("winning trade must not decay-close early: state=%q", state)
	}
	state, action = trader.EvaluateDecay(60, 1.5, "BREAKEVEN", prof)
	if state != "CLOSED" || action != "TIME_EXIT" {
		t.Errorf("hard horizon: state=%q action=%q, want CLOSED/TIME_EXIT", state, action)
	}
}
