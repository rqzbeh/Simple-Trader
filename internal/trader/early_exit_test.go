package trader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// mockTelegramSender tracks calls for exactly-once verification.
type mockTelegramSender struct {
	calls []string
	err   error
}

func (m *mockTelegramSender) SendMessageWithRetry(ctx context.Context, text string) error {
	m.calls = append(m.calls, text)
	return m.err
}

// mockEarlyExitStore records judgments for verification.
type mockEarlyExitStore struct {
	judgments      []*db.EarlyExitJudgment
	terminalMap    map[string]bool
	todayCounts    map[string]int
	lastExits      map[string]time.Time
	recordCloseErr error
	closedSignalID *int64
}

func newMockEarlyExitStore() *mockEarlyExitStore {
	return &mockEarlyExitStore{
		terminalMap: make(map[string]bool),
		todayCounts: make(map[string]int),
		lastExits:   make(map[string]time.Time),
	}
}

func (m *mockEarlyExitStore) InsertEarlyExitJudgment(ctx context.Context, j *db.EarlyExitJudgment) error {
	m.judgments = append(m.judgments, j)
	if j.ClusterID != nil && (j.Action == "closed" || j.Action == "guarded_skip") {
		key := fmt.Sprintf("%d:%d", j.PositionID, *j.ClusterID)
		m.terminalMap[key] = true
	}
	return nil
}

func (m *mockEarlyExitStore) RecordEarlyExitClose(ctx context.Context, j *db.EarlyExitJudgment, signalID *int64, exitPrice float64, exitReason string, pnl, roi float64) error {
	m.closedSignalID = signalID
	if m.recordCloseErr != nil {
		return m.recordCloseErr
	}
	return m.InsertEarlyExitJudgment(ctx, j)
}

func (m *mockEarlyExitStore) HasTerminalEarlyExitJudgment(ctx context.Context, positionID int64, clusterID int64) (bool, error) {
	key := fmt.Sprintf("%d:%d", positionID, clusterID)
	return m.terminalMap[key], nil
}

func (m *mockEarlyExitStore) CountEarlyExitsToday(ctx context.Context, symbol string, now time.Time) (int, error) {
	return m.todayCounts[symbol], nil
}

func (m *mockEarlyExitStore) GetLastEarlyExitTime(ctx context.Context, symbol string) (time.Time, bool, error) {
	t, ok := m.lastExits[symbol]
	return t, ok, nil
}

// T006: Test question builder close_now Noul shape per contracts §1.
func TestBuildEarlyExitQuestions(t *testing.T) {
	now := time.Now()
	p1 := &db.Trade{
		ID:         101,
		Symbol:     "BTC/USDT",
		Side:       "BUY",
		EntryPrice: 65000.0,
		StopLoss:   63000.0,
		TakeProfit: 69000.0,
		EntryTime:  now.Add(-40 * time.Minute),
		Status:     "OPEN",
	}
	p2 := &db.Trade{
		ID:         102,
		Symbol:     "ETH/USDT",
		Side:       "SELL",
		EntryPrice: 3500.0,
		StopLoss:   3600.0,
		TakeProfit: 3300.0,
		EntryTime:  now.Add(-50 * time.Minute),
		Status:     "OPEN",
	}
	positions := []*db.Trade{p1, p2}

	cluster := &market.NewsCluster{
		ID:          "cluster-abc-1",
		Headline:    "SEC files lawsuit against exchange",
		StoryCount:  5,
		FirstSeen:   now.Add(-10 * time.Minute),
		LastSeen:    now.Add(-5 * time.Minute),
		Headlines:   []string{"SEC files lawsuit against exchange", "Exchange sued by SEC"},
	}

	questions := BuildEarlyExitQuestions(positions, cluster, now)

	if len(questions) != 2 {
		t.Fatalf("expected 2 batched questions, got %d", len(questions))
	}

	q1, ok1 := questions["close_now:101"]
	if !ok1 {
		t.Fatalf("missing question for position 101 with key close_now:101")
	}
	if q1.Type != "noul" {
		t.Errorf("expected question type 'noul', got %q", q1.Type)
	}

	criteriaMap, ok := q1.Criteria.(map[string]string)
	if !ok {
		t.Fatalf("expected criteria map[string]string, got %T", q1.Criteria)
	}
	if criteriaMap["true"] != "News invalidates the thesis via high-severity opposing catalyst — close now" {
		t.Errorf("criteria true must mean close now, got %q", criteriaMap["true"])
	}
	if criteriaMap["false"] != "Thesis intact — hold" {
		t.Errorf("criteria false must mean hold, got %q", criteriaMap["false"])
	}

	instr, ok := q1.Instructions.(EarlyExitQuestionInstructions)
	if !ok {
		t.Fatalf("expected EarlyExitQuestionInstructions, got %T", q1.Instructions)
	}
	if instr.Position.Direction != "LONG" {
		t.Errorf("expected position direction LONG for BUY side, got %q", instr.Position.Direction)
	}
	if instr.Position.Entry != 65000.0 {
		t.Errorf("expected entry 65000, got %f", instr.Position.Entry)
	}
	if instr.Cluster.Headline != cluster.Headline {
		t.Errorf("expected cluster headline %q, got %q", cluster.Headline, instr.Cluster.Headline)
	}

	// Verify position 2 has question key close_now:102
	q2, ok2 := questions["close_now:102"]
	if !ok2 {
		t.Fatalf("missing question for position 102 with key close_now:102")
	}
	instr2 := q2.Instructions.(EarlyExitQuestionInstructions)
	if instr2.Position.Direction != "SHORT" {
		t.Errorf("expected position direction SHORT for SELL side, got %q", instr2.Position.Direction)
	}
}

// T007: Verdict mapping — noul >= floor -> close, telegram called once; noul < floor -> HOLD.
func TestEarlyExitVerdictMapping(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	cfg := config.EarlyExitConfig{
		Enabled:     true,
		MinHoldMin:  30,
		MaxPerDay:   3,
		CooldownMin: 60,
		ConfFloor:   0.75,
	}

	// Case 1: noul >= floor -> closes position, calls telegram once
	t.Run("noul >= floor closes position and notifies", func(t *testing.T) {
		engine := NewExecutionEngine(10000.0)
		pos := &db.Trade{
			ID:           201,
			Symbol:       "BTC/USDT",
			Side:         "BUY",
			EntryPrice:   60000.0,
			EntryTime:    now.Add(-45 * time.Minute),
			PositionSize: 0.1,
			Status:       "OPEN",
		}
		engine.positions["BTC/USDT"] = pos

		store := newMockEarlyExitStore()
		tg := &mockTelegramSender{}

		noulVal := 0.85
		answers := map[string]ai.JevAnswer{
			"close_now:201": {
				Type:       "noul",
				Noul:       &noulVal,
				Confidence: 0.85,
			},
		}

		cluster := &market.NewsCluster{
			ID:        "c1",
			Headline:  "SEC cracks down on crypto liquidity",
			FirstSeen: now.Add(-5 * time.Minute),
		}

		err := ProcessEarlyExitVerdict(ctx, "cycle-test-1", cfg, pos, cluster, answers["close_now:201"], "jev_direct", engine, store, tg, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Engine position must be CLOSED
		if len(engine.GetOpenTrades()) != 0 {
			t.Errorf("expected position to be closed in engine, but open positions remain")
		}
		closedTrades := engine.GetClosedTrades()
		if len(closedTrades) != 1 || closedTrades[0].ExitReason != "NEWS_EARLY_EXIT" {
			t.Errorf("expected 1 closed trade with NEWS_EARLY_EXIT, got %+v", closedTrades)
		}

		// Telegram must be called EXACTLY ONCE
		if len(tg.calls) != 1 {
			t.Errorf("expected telegram called exactly once, called %d times", len(tg.calls))
		}

		// Judgment stored with action='closed'
		if len(store.judgments) != 1 {
			t.Fatalf("expected 1 judgment stored, got %d", len(store.judgments))
		}
		j := store.judgments[0]
		if j.Action != "closed" || j.Verdict != "DO_NOT_HOLD" || j.Status != "ok" {
			t.Errorf("expected closed DO_NOT_HOLD ok, got action=%q verdict=%q status=%q", j.Action, j.Verdict, j.Status)
		}
		if j.TelegramSent == nil || !*j.TelegramSent {
			t.Errorf("expected TelegramSent=true")
		}
	})

	// Case 2: noul < floor -> HOLD record, position untouched
	t.Run("noul < floor records HOLD and keeps position open", func(t *testing.T) {
		engine := NewExecutionEngine(10000.0)
		pos := &db.Trade{
			ID:           202,
			Symbol:       "ETH/USDT",
			Side:         "BUY",
			EntryPrice:   3000.0,
			EntryTime:    now.Add(-45 * time.Minute),
			PositionSize: 1.0,
			Status:       "OPEN",
		}
		engine.positions["ETH/USDT"] = pos

		store := newMockEarlyExitStore()
		tg := &mockTelegramSender{}

		noulVal := 0.40 // below floor 0.75
		ans := ai.JevAnswer{
			Type:       "noul",
			Noul:       &noulVal,
			Confidence: 0.60,
		}

		cluster := &market.NewsCluster{
			ID:        "c2",
			Headline:  "Minor token listing announced",
			FirstSeen: now.Add(-5 * time.Minute),
		}

		err := ProcessEarlyExitVerdict(ctx, "cycle-test-2", cfg, pos, cluster, ans, "jev_direct", engine, store, tg, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Position stays OPEN
		if len(engine.GetOpenTrades()) != 1 {
			t.Errorf("expected position to stay OPEN")
		}
		if len(tg.calls) != 0 {
			t.Errorf("expected no telegram message on HOLD")
		}

		// Judgment stored with action='guarded_skip', verdict='HOLD'
		if len(store.judgments) != 1 {
			t.Fatalf("expected 1 judgment, got %d", len(store.judgments))
		}
		j := store.judgments[0]
		if j.Verdict != "HOLD" {
			t.Errorf("expected verdict HOLD, got %q", j.Verdict)
		}
		if j.Action != "guarded_skip" {
			t.Errorf("expected action guarded_skip, got %q", j.Action)
		}
	})

	// Case 3: Core error -> error row, position untouched (FR-105)
	t.Run("core error records error row and keeps position open", func(t *testing.T) {
		engine := NewExecutionEngine(10000.0)
		pos := &db.Trade{
			ID:           203,
			Symbol:       "SOL/USDT",
			Side:         "BUY",
			EntryPrice:   150.0,
			EntryTime:    now.Add(-45 * time.Minute),
			PositionSize: 5.0,
			Status:       "OPEN",
		}
		engine.positions["SOL/USDT"] = pos

		store := newMockEarlyExitStore()
		cluster := &market.NewsCluster{
			ID:        "c3",
			Headline:  "Network upgrade completed",
			FirstSeen: now.Add(-5 * time.Minute),
		}

		coreErr := errors.New("jev deadline exceeded")
		RecordEarlyExitCoreError(ctx, "cycle-test-3", pos, cluster, coreErr, store)

		if len(engine.GetOpenTrades()) != 1 {
			t.Errorf("expected position to stay open on error")
		}
		if len(store.judgments) != 1 {
			t.Fatalf("expected 1 judgment, got %d", len(store.judgments))
		}
		j := store.judgments[0]
		if j.Status != "error" || j.Action != "error" || j.Verdict != "" {
			t.Errorf("expected status=error action=error verdict='', got status=%q action=%q verdict=%q", j.Status, j.Action, j.Verdict)
		}
		if j.Error == "" {
			t.Errorf("expected non-empty error on error row")
		}
	})

	// Case 4: Store transaction failure -> records action=error with close_error and returns error
	t.Run("store tx failure records close_error row and returns explicit error", func(t *testing.T) {
		engine := NewExecutionEngine(10000.0)
		sigID := int64(999)
		pos := &db.Trade{
			ID:           204,
			SignalID:     &sigID,
			Symbol:       "AVAX/USDT",
			Side:         "BUY",
			EntryPrice:   25.0,
			EntryTime:    now.Add(-45 * time.Minute),
			PositionSize: 10.0,
			Status:       "OPEN",
		}
		engine.positions["AVAX/USDT"] = pos

		store := newMockEarlyExitStore()
		store.recordCloseErr = fmt.Errorf("simulated db connection drop")
		tg := &mockTelegramSender{}

		noulVal := 0.90
		ans := ai.JevAnswer{
			Type:       "noul",
			Noul:       &noulVal,
			Confidence: 0.90,
		}
		cluster := &market.NewsCluster{
			ID:        "c4",
			Headline:  "Major network outage confirmed",
			FirstSeen: now.Add(-5 * time.Minute),
		}

		err := ProcessEarlyExitVerdict(ctx, "cycle-test-4", cfg, pos, cluster, ans, "jev_direct", engine, store, tg, now)
		if err == nil {
			t.Fatal("expected explicit error when store tx fails, got nil")
		}

		// In-memory position was closed, but divergence was recorded
		if len(engine.GetOpenTrades()) != 0 {
			t.Errorf("expected position closed in engine")
		}
		if len(store.judgments) != 1 {
			t.Fatalf("expected 1 judgment recorded, got %d", len(store.judgments))
		}
		j := store.judgments[0]
		if j.Action != "error" {
			t.Errorf("expected action=error, got %q", j.Action)
		}
		if j.CloseError == "" {
			t.Errorf("expected non-empty CloseError")
		}
	})
}

// T011: Guardrails fail with exact reason strings in contracts §2 order.
func TestEarlyExitGuardsOrderAndReasons(t *testing.T) {
	cfg := config.EarlyExitConfig{
		Enabled:     true,
		MinHoldMin:  30,
		MaxPerDay:   3,
		CooldownMin: 60,
		ConfFloor:   0.75,
	}

	// 1. kill_switch
	disabledCfg := cfg
	disabledCfg.Enabled = false
	passed, reason := EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          disabledCfg,
		PositionAgeMin:  10, // would fail min_hold too
		LastExitAgeMin:  10, // would fail cooldown too
		HasPriorExit:    true,
		ExitsTodayCount: 5,  // would fail budget too
		Confidence:      0.5,
	})
	if passed || reason != "kill_switch" {
		t.Errorf("expected kill_switch first, got passed=%v reason=%q", passed, reason)
	}

	// 2. min_hold
	passed, reason = EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  15, // < 30
		LastExitAgeMin:  10, // would fail cooldown too
		HasPriorExit:    true,
		ExitsTodayCount: 5,  // would fail budget too
		Confidence:      0.5,
	})
	if passed || reason != "min_hold" {
		t.Errorf("expected min_hold second, got passed=%v reason=%q", passed, reason)
	}

	// 3. cooldown
	passed, reason = EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  45, // >= 30 OK
		LastExitAgeMin:  20, // < 60
		HasPriorExit:    true,
		ExitsTodayCount: 5, // would fail budget too
		Confidence:      0.5,
	})
	if passed || reason != "cooldown" {
		t.Errorf("expected cooldown third, got passed=%v reason=%q", passed, reason)
	}

	// 4. daily budget
	passed, reason = EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  45, // >= 30 OK
		LastExitAgeMin:  90, // >= 60 OK
		HasPriorExit:    true,
		ExitsTodayCount: 3, // >= 3
		Confidence:      0.5,
	})
	if passed || reason != "budget" {
		t.Errorf("expected budget fourth, got passed=%v reason=%q", passed, reason)
	}

	// 5. conf_floor
	passed, reason = EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  45, // >= 30 OK
		LastExitAgeMin:  90, // >= 60 OK
		HasPriorExit:    true,
		ExitsTodayCount: 1, // < 3 OK
		Confidence:      0.70, // < 0.75
	})
	if passed || reason != "conf_floor" {
		t.Errorf("expected conf_floor fifth, got passed=%v reason=%q", passed, reason)
	}

	// All pass
	passed, reason = EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  45,
		LastExitAgeMin:  90,
		HasPriorExit:    true,
		ExitsTodayCount: 1,
		Confidence:      0.85,
	})
	if !passed || reason != "" {
		t.Errorf("expected all pass, got passed=%v reason=%q", passed, reason)
	}
}

// T015: Failure-injection suite:
// 1. Telegram down -> close happens, telegram_error recorded, NO second close (SC-104)
// 2. Core timeout -> explicit error (quickstart V4)
func TestEarlyExitFailureInjection_TelegramDownNoSecondClose(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	cfg := config.EarlyExitConfig{
		Enabled:     true,
		MinHoldMin:  30,
		MaxPerDay:   3,
		CooldownMin: 60,
		ConfFloor:   0.75,
	}

	engine := NewExecutionEngine(10000.0)
	pos := &db.Trade{
		ID:           301,
		Symbol:       "BTC/USDT",
		Side:         "BUY",
		EntryPrice:   65000.0,
		EntryTime:    now.Add(-40 * time.Minute),
		PositionSize: 0.1,
		Status:       "OPEN",
	}
	engine.positions["BTC/USDT"] = pos

	store := newMockEarlyExitStore()
	tg := &mockTelegramSender{
		err: errors.New("telegram network timeout"),
	}

	noulVal := 0.90
	ans := ai.JevAnswer{
		Type:       "noul",
		Noul:       &noulVal,
		Confidence: 0.90,
	}
	cluster := &market.NewsCluster{
		ID:        "12345",
		Headline:  "Catastrophic regulatory ban passed",
		FirstSeen: now.Add(-5 * time.Minute),
	}

	// First cycle: close happens even though telegram fails
	err := ProcessEarlyExitVerdict(ctx, "cycle-fail-1", cfg, pos, cluster, ans, "jev_direct", engine, store, tg, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify position is closed
	if len(engine.GetOpenTrades()) != 0 {
		t.Fatalf("expected position to be closed despite telegram failure")
	}
	closedTrades := engine.GetClosedTrades()
	if len(closedTrades) != 1 {
		t.Fatalf("expected 1 closed trade, got %d", len(closedTrades))
	}
	if closedTrades[0].ExitReason != "NEWS_EARLY_EXIT" {
		t.Errorf("expected exit reason NEWS_EARLY_EXIT, got %q", closedTrades[0].ExitReason)
	}

	// Verify judgment row has telegram_error and action='closed'
	if len(store.judgments) != 1 {
		t.Fatalf("expected 1 judgment, got %d", len(store.judgments))
	}
	j := store.judgments[0]
	if j.Action != "closed" {
		t.Errorf("expected action=closed, got %q", j.Action)
	}
	if j.TelegramSent == nil || *j.TelegramSent != false {
		t.Errorf("expected TelegramSent=false")
	}
	if !strings.Contains(j.TelegramError, "telegram network timeout") {
		t.Errorf("expected telegram error recorded, got %q", j.TelegramError)
	}

	// Second cycle: re-delivering same cluster / position must NOT close again (SC-104)
	terminal, _ := store.HasTerminalEarlyExitJudgment(ctx, pos.ID, 12345)
	if !terminal {
		t.Fatalf("expected store to record terminal action for dedup")
	}

	// If ForceClosePosition is attempted again on already closed position, it returns false
	_, closedAgain := engine.ForceClosePosition("BTC/USDT", 0, "NEWS_EARLY_EXIT")
	if closedAgain {
		t.Fatalf("duplicate close must not succeed (SC-104)")
	}
	if len(engine.GetClosedTrades()) != 1 {
		t.Errorf("expected still exactly 1 closed trade, got %d", len(engine.GetClosedTrades()))
	}
}

func TestEarlyExitFailureInjection_CoreTimeoutExplicitError(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	engine := NewExecutionEngine(10000.0)
	pos := &db.Trade{
		ID:           302,
		Symbol:       "ETH/USDT",
		Side:         "BUY",
		EntryPrice:   3500.0,
		EntryTime:    now.Add(-40 * time.Minute),
		PositionSize: 1.0,
		Status:       "OPEN",
	}
	engine.positions["ETH/USDT"] = pos

	store := newMockEarlyExitStore()
	cluster := &market.NewsCluster{
		ID:        "54321",
		Headline:  "Breaking announcement pending",
		FirstSeen: now.Add(-2 * time.Minute),
	}

	timeoutErr := ai.WrapDecision("jev", "cycle-timeout-1", ai.ErrJevTimeout, "context deadline exceeded")
	RecordEarlyExitCoreError(ctx, "cycle-timeout-1", pos, cluster, timeoutErr, store)

	// Position must remain OPEN
	if len(engine.GetOpenTrades()) != 1 {
		t.Errorf("position must stay open on core timeout")
	}

	// Explicit error record created
	if len(store.judgments) != 1 {
		t.Fatalf("expected 1 error judgment, got %d", len(store.judgments))
	}
	j := store.judgments[0]
	if j.Status != "error" || j.Action != "error" {
		t.Errorf("expected status=error action=error, got status=%q action=%q", j.Status, j.Action)
	}
	if j.Verdict != "" {
		t.Errorf("expected empty verdict on error row, got %q", j.Verdict)
	}
	if !strings.Contains(j.Error, "component=early-exit") || !strings.Contains(j.Error, "cycle=cycle-timeout-1") {
		t.Errorf("expected explicit error with component and cycle, got %q", j.Error)
	}
}
