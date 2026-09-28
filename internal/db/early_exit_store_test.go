package db

import (
	"testing"
)

func TestEarlyExitJudgmentValidation(t *testing.T) {
	noul := 0.85
	conf := 0.82
	guardsPassed := true

	valid := &EarlyExitJudgment{
		CycleID:         "cycle-123",
		PositionID:      101,
		Symbol:          "BTC/USDT",
		Verdict:         "DO_NOT_HOLD",
		Noul:            &noul,
		Confidence:      &conf,
		Route:           "jev_direct",
		GuardsPassed:    &guardsPassed,
		Action:          "closed",
		Status:          "ok",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid judgment rejected: %v", err)
	}

	// Status = error requires non-empty Error
	errNoMsg := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "error",
	}
	if err := errNoMsg.Validate(); err == nil {
		t.Fatal("error status without error message must be rejected (FR-105)")
	}

	// Status = error with verdict must be rejected
	errWithVerdict := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "error",
		Error:      "jev timeout",
		Verdict:    "DO_NOT_HOLD",
	}
	if err := errWithVerdict.Validate(); err == nil {
		t.Fatal("error status with non-empty verdict must be rejected")
	}

	// Status = ok must not have error
	okWithErr := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "ok",
		Verdict:    "HOLD",
		Error:      "some err",
	}
	if err := okWithErr.Validate(); err == nil {
		t.Fatal("status=ok with error string must be rejected")
	}

	// Invalid verdict: must be HOLD or DO_NOT_HOLD only (FR-107)
	badVerdict := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "ok",
		Verdict:    "SELL",
	}
	if err := badVerdict.Validate(); err == nil {
		t.Fatal("SELL violates position-intent vocabulary HOLD/DO_NOT_HOLD (FR-107)")
	}

	// Bad action
	badAction := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "ok",
		Verdict:    "HOLD",
		Action:     "liquidated",
	}
	if err := badAction.Validate(); err == nil {
		t.Fatal("invalid action must be rejected")
	}

	// Bad route
	badRoute := &EarlyExitJudgment{
		CycleID:    "cycle-123",
		PositionID: 101,
		Symbol:     "BTC/USDT",
		Status:     "ok",
		Verdict:    "HOLD",
		Action:     "guarded_skip",
		Route:      "unknown_route",
	}
	if err := badRoute.Validate(); err == nil {
		t.Fatal("invalid route must be rejected")
	}
}

func TestEarlyExitReport_NilPool(t *testing.T) {
	store := &Store{Pool: nil}
	_, err := store.EarlyExitReport(t.Context(), 14)
	if err == nil {
		t.Fatalf("expected error when pool is nil")
	}
}

func TestEarlyExitBackfill_NilPool(t *testing.T) {
	store := &Store{Pool: nil}
	err := store.BackfillEarlyExitOutcome(t.Context(), 101, 2.5)
	if err != nil {
		t.Fatalf("expected nil error on nil pool for backfill, got: %v", err)
	}
}

