package trader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
		ID:         "cluster-abc-1",
		Headline:   "SEC files lawsuit against exchange",
		StoryCount: 5,
		FirstSeen:  now.Add(-10 * time.Minute),
		LastSeen:   now.Add(-5 * time.Minute),
		Headlines:  []string{"SEC files lawsuit against exchange", "Exchange sued by SEC"},
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
		ExitsTodayCount: 5, // would fail budget too
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
		ExitsTodayCount: 5, // would fail budget too
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
		ExitsTodayCount: 1,    // < 3 OK
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

func TestEarlyExitGuardsManagedWhenUnset(t *testing.T) {
	origEnabled := os.Getenv("EARLY_EXIT_ENABLED")
	origMinHold := os.Getenv("EARLY_EXIT_MIN_HOLD_MIN")
	origMaxPerDay := os.Getenv("EARLY_EXIT_MAX_PER_DAY")
	origCooldown := os.Getenv("EARLY_EXIT_COOLDOWN_MIN")
	origConfFloor := os.Getenv("EARLY_EXIT_CONF_FLOOR")
	defer func() {
		os.Setenv("EARLY_EXIT_ENABLED", origEnabled)
		os.Setenv("EARLY_EXIT_MIN_HOLD_MIN", origMinHold)
		os.Setenv("EARLY_EXIT_MAX_PER_DAY", origMaxPerDay)
		os.Setenv("EARLY_EXIT_COOLDOWN_MIN", origCooldown)
		os.Setenv("EARLY_EXIT_CONF_FLOOR", origConfFloor)
	}()

	// 1. Absent env -> EarlyExitConfig marks managed
	os.Unsetenv("EARLY_EXIT_ENABLED")
	os.Unsetenv("EARLY_EXIT_MIN_HOLD_MIN")
	os.Unsetenv("EARLY_EXIT_MAX_PER_DAY")
	os.Unsetenv("EARLY_EXIT_COOLDOWN_MIN")
	os.Unsetenv("EARLY_EXIT_CONF_FLOOR")

	cfg, err := config.LoadEarlyExitConfig()
	if err != nil {
		t.Fatalf("expected LoadEarlyExitConfig to succeed when unset: %v", err)
	}
	for _, k := range []string{"ENABLED", "MIN_HOLD_MIN", "MAX_PER_DAY", "COOLDOWN_MIN", "CONF_FLOOR"} {
		if !cfg.Managed[k] {
			t.Errorf("expected Managed[%q] to be true", k)
		}
	}
	if !cfg.IsEnabled() {
		t.Errorf("expected IsEnabled() to be true (absent = enabled toggle convention)")
	}

	// 2. Batch questions built for managed guards
	questions := BuildEarlyExitGuardQuestions(cfg, nil)
	expectedKeys := []string{
		QuestionEarlyExitMinHold,
		QuestionEarlyExitCooldown,
		QuestionEarlyExitMaxPerDay,
		QuestionEarlyExitConfFloor,
	}
	for _, k := range expectedKeys {
		if _, ok := questions[k]; !ok {
			t.Errorf("expected managed guard question %q in batch questions", k)
		}
	}

	// Also verify BuildEarlyExitQuestions includes them when cfg is provided
	pos := &db.Trade{ID: 10, Symbol: "BTC/USDT", Status: "OPEN", Side: "BUY", EntryPrice: 50000}
	batched := BuildEarlyExitQuestions([]*db.Trade{pos}, nil, time.Now(), cfg)
	if _, ok := batched["close_now:10"]; !ok {
		t.Errorf("expected close_now:10 in batched questions")
	}
	for _, k := range expectedKeys {
		if _, ok := batched[k]; !ok {
			t.Errorf("expected guard question %q in batched questions", k)
		}
	}

	// 3. Resolved values clamp (e.g. max_per_day answer 25 -> 10, conf_floor 0.3 -> 0.5)
	maxPerDayVal := 25.0
	confFloorVal := 0.3
	minHoldVal := -10.0
	cooldownVal := 2000.0

	answers := map[string]ai.JevAnswer{
		QuestionEarlyExitMaxPerDay: {Type: "score", Score: &maxPerDayVal},
		QuestionEarlyExitConfFloor: {Type: "score", Score: &confFloorVal},
		QuestionEarlyExitMinHold:   {Type: "score", Score: &minHoldVal},
		QuestionEarlyExitCooldown:  {Type: "score", Score: &cooldownVal},
	}

	resolved, err := ResolveEarlyExitGuards(cfg, answers, "cycle-clamp-test")
	if err != nil {
		t.Fatalf("expected ResolveEarlyExitGuards to succeed: %v", err)
	}

	if resolved.MaxPerDay != 10 {
		t.Errorf("expected MaxPerDay clamped to 10, got %d", resolved.MaxPerDay)
	}
	if !resolved.Clamped["max_per_day"] {
		t.Errorf("expected Clamped[max_per_day] = true")
	}

	if resolved.ConfFloor != 0.5 {
		t.Errorf("expected ConfFloor clamped to 0.5, got %f", resolved.ConfFloor)
	}
	if !resolved.Clamped["conf_floor"] {
		t.Errorf("expected Clamped[conf_floor] = true")
	}

	if resolved.MinHoldMin != 0 {
		t.Errorf("expected MinHoldMin clamped to 0, got %d", resolved.MinHoldMin)
	}
	if !resolved.Clamped["min_hold"] {
		t.Errorf("expected Clamped[min_hold] = true")
	}

	if resolved.CooldownMin != 1440 {
		t.Errorf("expected CooldownMin clamped to 1440, got %d", resolved.CooldownMin)
	}
	if !resolved.Clamped["cooldown"] {
		t.Errorf("expected Clamped[cooldown] = true")
	}

	// 4. Override present -> question omitted + value verbatim
	overrideCfg := config.EarlyExitConfig{
		Enabled:     true,
		MinHoldMin:  45,
		MaxPerDay:   5,
		CooldownMin: 120,
		ConfFloor:   0.85,
		Managed:     map[string]bool{}, // all overrides, none managed
	}
	overrideQuestions := BuildEarlyExitGuardQuestions(overrideCfg, nil)
	if len(overrideQuestions) != 0 {
		t.Errorf("expected 0 guard questions when override present, got %d", len(overrideQuestions))
	}

	resolvedOverride, err := ResolveEarlyExitGuards(overrideCfg, map[string]ai.JevAnswer{}, "cycle-override-test")
	if err != nil {
		t.Fatalf("expected ResolveEarlyExitGuards with overrides to succeed without answers: %v", err)
	}
	if resolvedOverride.MinHoldMin != 45 {
		t.Errorf("expected verbatim MinHoldMin 45, got %d", resolvedOverride.MinHoldMin)
	}
	if resolvedOverride.MaxPerDay != 5 {
		t.Errorf("expected verbatim MaxPerDay 5, got %d", resolvedOverride.MaxPerDay)
	}
	if resolvedOverride.CooldownMin != 120 {
		t.Errorf("expected verbatim CooldownMin 120, got %d", resolvedOverride.CooldownMin)
	}
	if resolvedOverride.ConfFloor != 0.85 {
		t.Errorf("expected verbatim ConfFloor 0.85, got %f", resolvedOverride.ConfFloor)
	}
	if len(resolvedOverride.Clamped) != 0 {
		t.Errorf("expected zero clamped fields on override, got %v", resolvedOverride.Clamped)
	}

	// 5. Kill switch test: explicit false disables immediately
	killCfg := config.EarlyExitConfig{
		Enabled: false,
		Managed: map[string]bool{"ENABLED": false},
	}
	resolvedKill, _ := ResolveEarlyExitGuards(killCfg, map[string]ai.JevAnswer{}, "cycle-kill")
	if resolvedKill.Enabled {
		t.Errorf("expected killCfg.Enabled = false")
	}
	passed, reason := EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:         killCfg,
		ResolvedGuards: &resolvedKill,
	})
	if passed || reason != "kill_switch" {
		t.Errorf("expected EvaluateEarlyExitGuards to fail with kill_switch, got passed=%v reason=%q", passed, reason)
	}
}

// TestEarlyExitBatchStateNonNull proves the real state payload sent to Jev is non-null (T004b).
func TestEarlyExitBatchStateNonNull(t *testing.T) {
	now := time.Now()
	pos := &db.Trade{
		ID:           401,
		Symbol:       "ETH/USDT",
		Side:         "BUY",
		EntryPrice:   3000.0,
		StopLoss:     2850.0,
		TakeProfit:   3300.0,
		PositionSize: 2.0,
		Leverage:     5,
		EntryTime:    now.Add(-25 * time.Minute),
		Status:       "OPEN",
	}
	cluster := &market.NewsCluster{
		ID:             "9988",
		Headline:       "Major institutional adoption announcement",
		StoryCount:     4,
		FusedSentiment: 0.72,
		FreshWeight:    0.95,
		Sources:        []string{"Bloomberg", "Reuters"},
		FirstSeen:      now.Add(-10 * time.Minute),
	}

	// 1. BuildEarlyExitState produces real non-null state
	state := BuildEarlyExitState([]*db.Trade{pos}, cluster, now)
	if state.Timestamp == "" {
		t.Errorf("expected non-empty timestamp")
	}
	if state.Cluster == nil {
		t.Fatalf("expected non-nil catalyst cluster fact")
	}
	if state.Cluster.Headline != cluster.Headline {
		t.Errorf("expected headline %q, got %q", cluster.Headline, state.Cluster.Headline)
	}
	if state.Cluster.FusedSentiment != cluster.FusedSentiment {
		t.Errorf("expected fused sentiment %f, got %f", cluster.FusedSentiment, state.Cluster.FusedSentiment)
	}
	if len(state.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(state.Positions))
	}
	if state.Positions[0].Symbol != "ETH/USDT" {
		t.Errorf("expected position symbol ETH/USDT, got %s", state.Positions[0].Symbol)
	}

	// 2. Verify JSON marshaling produces non-null state object
	rawBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if string(rawBytes) == "null" || len(rawBytes) == 0 {
		t.Fatalf("marshaled state must not be null")
	}

	// 3. Test jev.Evaluate request over mock HTTP server: state must be a non-null JSON object
	var receivedState interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		receivedState = req["state"]
		if req["state"] == nil {
			// Fail with 422 exactly as TypeSafe does when state is null
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"type":"missing","loc":["body","state"],"msg":"Field required"}`))
			return
		}
		// Return valid answers
		resp := map[string]interface{}{
			"answers": map[string]interface{}{
				"close_now:401": map[string]interface{}{
					"type":       "noul",
					"confidence": 0.20,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	jev := ai.NewJevClient(server.URL, "test-key", 5*time.Second)
	questions := BuildEarlyExitQuestions([]*db.Trade{pos}, cluster, now)
	answers, _, err := jev.Evaluate(context.Background(), "cycle-test-state", state, questions)
	if err != nil {
		t.Fatalf("jev.Evaluate failed (server returned 422 if state was null): %v", err)
	}
	if len(answers) == 0 {
		t.Fatalf("expected answers from Jev, got empty")
	}
	if receivedState == nil {
		t.Errorf("mock server received null state")
	}
	stateMap, ok := receivedState.(map[string]interface{})
	if !ok || len(stateMap) == 0 {
		t.Errorf("expected received state to be non-empty JSON object, got %v", receivedState)
	}
}
