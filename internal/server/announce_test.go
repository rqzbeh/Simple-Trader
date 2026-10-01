package server

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// TestNoTelegramWithoutPersist (spec-022 FR-803 / gate G44): a signal that
// never reached the database (ID==0) must never be announced — Telegram
// claiming trades the Terminal cannot show was the 2026-10-01 defect.
func TestNoTelegramWithoutPersist(t *testing.T) {
	unpersisted := &db.FuturesTradeSignal{Symbol: "BTC/USDT", Status: "ACTIVE", TelegramDispatched: false}
	if shouldAnnounceSignal(unpersisted) {
		t.Errorf("unpersisted signal (ID=0) must not be announced")
	}
	persisted := &db.FuturesTradeSignal{Symbol: "BTC/USDT", Status: "ACTIVE", ID: 155, TelegramDispatched: false}
	if !shouldAnnounceSignal(persisted) {
		t.Errorf("persisted fresh signal must be announced")
	}
	dispatched := &db.FuturesTradeSignal{Symbol: "BTC/USDT", Status: "ACTIVE", ID: 155, TelegramDispatched: true}
	if shouldAnnounceSignal(dispatched) {
		t.Errorf("already-dispatched signal must not re-announce")
	}
	if shouldAnnounceSignal(nil) {
		t.Errorf("nil signal must not be announced")
	}
}
