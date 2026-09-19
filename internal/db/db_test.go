package db_test

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

func TestParseDatabaseURL(t *testing.T) {
	err := db.ValidateURL("postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable")
	if err != nil {
		t.Fatalf("expected valid url, got error: %v", err)
	}

	err = db.ValidateURL("invalid-url-without-scheme")
	if err == nil {
		t.Fatalf("expected error on invalid url, got nil")
	}
}

func TestSignalModelValidation(t *testing.T) {
	sig := db.Signal{
		Symbol:          "BTC/USD",
		Side:            "BUY",
		Bucket:          "ALPHA",
		EntryPrice:      65000.0,
		StopLoss:        63500.0,
		TakeProfit:      68000.0,
		Confidence:      0.88,
		ConfluenceScore: 0.92,
		Status:          "OPEN",
		CreatedAt:       time.Now(),
	}
	if err := sig.Validate(); err != nil {
		t.Errorf("expected valid signal, got: %v", err)
	}

	invalidSig := sig
	invalidSig.Side = "INVALID_SIDE"
	if err := invalidSig.Validate(); err == nil {
		t.Errorf("expected error on invalid side, got nil")
	}

	invalidBucketSig := sig
	invalidBucketSig.Bucket = "IRAN"
	if err := invalidBucketSig.Validate(); err == nil {
		t.Errorf("expected error on non-allowed bucket, got nil")
	}
}

func TestTradeModelCalculations(t *testing.T) {
	tr := db.Trade{
		Symbol:       "XAU/USD",
		Side:         "BUY",
		Bucket:       "CORE",
		PositionSize: 10.0,
		EntryPrice:   2600.0,
		ExitPrice:    2652.0, // +2%
		Status:       "CLOSED",
	}

	pnl, retPct := tr.CalculatePnL()
	expectedPnL := (2652.0 - 2600.0) * 10.0      // 520.0
	expectedRet := float32(((2652.0 - 2600.0) / 2600.0) * 100) // 2.0%

	if pnl != expectedPnL {
		t.Errorf("expected PnL %f, got %f", expectedPnL, pnl)
	}
	if retPct != expectedRet {
		t.Errorf("expected Return %f%%, got %f%%", expectedRet, retPct)
	}
}
