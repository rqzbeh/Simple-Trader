package server_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// fakeAnalyzer returns a fixed BUY decision so the entry gate is the only
// thing that can reject a candidate.
type fakeAnalyzer struct {
	resp *ai.DecisionResponse
}

func (f *fakeAnalyzer) Analyze(_ context.Context, _ ai.DecisionRequest) (*ai.DecisionResponse, error) {
	// Copy so GateRejected mutations never leak across evaluations.
	out := *f.resp
	return &out, nil
}

// recordingStore captures filter-log rows and signal inserts, and never
// reports an existing active signal.
type recordingStore struct {
	inserts    []*db.FuturesTradeSignal
	filterLogs []struct {
		symbol string
		rule   string
		detail json.RawMessage
	}
}

func (r *recordingStore) InsertFuturesSignal(_ context.Context, sig *db.FuturesTradeSignal) (*db.FuturesTradeSignal, error) {
	stored := *sig
	stored.ID = int64(len(r.inserts) + 1)
	r.inserts = append(r.inserts, &stored)
	return &stored, nil
}

func (r *recordingStore) ListFuturesSignals(context.Context, string, int) ([]db.FuturesTradeSignal, error) {
	return nil, nil
}

func (r *recordingStore) GetActiveFuturesSignalBySymbol(context.Context, string) (*db.FuturesTradeSignal, error) {
	return nil, nil
}

func (r *recordingStore) CloseFuturesSignal(context.Context, int64, float64, string, float64, float64) error {
	return nil
}

func (r *recordingStore) MarkSignalDispatched(context.Context, int64) error { return nil }

func (r *recordingStore) MarkSignalResolved(context.Context, int64) error { return nil }

func (r *recordingStore) InsertEntryFilterLog(_ context.Context, symbol, _ string, _ *int64, rule string, detail json.RawMessage) error {
	r.filterLogs = append(r.filterLogs, struct {
		symbol string
		rule   string
		detail json.RawMessage
	}{symbol, rule, detail})
	return nil
}

func buyDecision() *ai.DecisionResponse {
	return &ai.DecisionResponse{
		Decision:               "BUY",
		Confidence:             0.75,
		Reasoning:              "integration fixture",
		Catalyst:               "Fixture catalyst headline",
		Leverage:               5,
		SuggestedStopLossPct:   1.0,
		SuggestedTakeProfitPct: 3.0,
	}
}

func newGateService(store *recordingStore) *trader.SignalService {
	return trader.NewSignalService(store, &fakeAnalyzer{resp: buyDecision()}, trader.DefaultSignalConfig())
}

// runGate evaluates one fixture snapshot and returns the outcome fields the
// assertions need.
func runGate(t *testing.T, store *recordingStore, snap cache.IndicatorSnapshot, price float64) (*db.FuturesTradeSignal, *ai.DecisionResponse) {
	t.Helper()
	svc := newGateService(store)
	sig, decision, err := svc.EvaluateMarketSignal(
		context.Background(),
		"BTC/USDT",
		"ALPHA",
		cache.TickerQuote{Symbol: "BTC/USDT", Price: price},
		snap,
		nil,
		nil,
		100000.0,
		40000.0,
	)
	if err != nil {
		t.Fatalf("EvaluateMarketSignal failed: %v", err)
	}
	return sig, decision
}

func expectRejection(t *testing.T, store *recordingStore, sig *db.FuturesTradeSignal, decision *ai.DecisionResponse, wantRule string) {
	t.Helper()
	if sig != nil {
		t.Fatalf("rejected candidate persisted signal #%d", sig.ID)
	}
	if decision == nil || decision.GateRejected != wantRule {
		t.Fatalf("GateRejected = %v, want %q", decision, wantRule)
	}
	if len(store.filterLogs) != 1 {
		t.Fatalf("filter log rows = %d, want exactly 1 (FR-020)", len(store.filterLogs))
	}
	if store.filterLogs[0].rule != wantRule {
		t.Errorf("logged rule = %q, want %q", store.filterLogs[0].rule, wantRule)
	}
	if len(store.inserts) != 0 {
		t.Errorf("signal inserts = %d, want 0", len(store.inserts))
	}
	if len(store.filterLogs[0].detail) == 0 {
		t.Errorf("filter log detail empty; audit must carry rule metrics")
	}
}

// TestFilterLogEntryGateRejections drives three rejection fixtures and one
// pass-through through the full evaluation path (spec 012 US1, T014;
// quickstart §3): each rejection writes exactly one entry_filter_log row and
// persists no signal; a conforming candidate is persisted once.
func TestFilterLogEntryGateRejections(t *testing.T) {
	t.Run("NO_TREND: long entry below VWAP while SuperTrend says BULL", func(t *testing.T) {
		store := &recordingStore{}
		snap := cache.IndicatorSnapshot{
			SuperTrend: "BULL",
			VWAP:       105, // price 100 < VWAP → trend disagrees
		}
		sig, decision := runGate(t, store, snap, 100)
		expectRejection(t, store, sig, decision, "NO_TREND")
	})

	t.Run("NO_VOLUME: catalyst candle below 2.5x average volume", func(t *testing.T) {
		store := &recordingStore{}
		snap := cache.IndicatorSnapshot{
			SuperTrend:  "BULL",
			VWAP:        99, // price above VWAP → trend passes
			VolumeRatio: 1.0,
		}
		sig, decision := runGate(t, store, snap, 100)
		expectRejection(t, store, sig, decision, "NO_VOLUME")
	})

	t.Run("CHASE_BLOCKED: price beyond +2 sigma of the band", func(t *testing.T) {
		store := &recordingStore{}
		snap := cache.IndicatorSnapshot{
			SuperTrend:  "BULL",
			VWAP:        100,
			VolumeRatio: 3.0, // volume passes
			MiddleBand:  100,
			UpperBand:   104,
			LowerBand:   96, // sigma = 2 → z = (105-100)/2 = 2.5 > 2.0
		}
		sig, decision := runGate(t, store, snap, 105)
		expectRejection(t, store, sig, decision, "CHASE_BLOCKED")
	})

	t.Run("PASS: conforming candidate is persisted once", func(t *testing.T) {
		store := &recordingStore{}
		snap := cache.IndicatorSnapshot{
			SuperTrend:  "BULL",
			VWAP:        99,
			VolumeRatio: 3.0,
			MiddleBand:  100,
			UpperBand:   104,
			LowerBand:   96,
		}
		sig, decision := runGate(t, store, snap, 100)
		if sig == nil {
			t.Fatalf("conforming candidate rejected: %+v", decision)
		}
		if decision != nil && decision.GateRejected != "" {
			t.Errorf("GateRejected = %q, want empty", decision.GateRejected)
		}
		if len(store.filterLogs) != 0 {
			t.Errorf("filter log rows = %d, want 0 for a pass", len(store.filterLogs))
		}
		if len(store.inserts) != 1 {
			t.Fatalf("signal inserts = %d, want 1", len(store.inserts))
		}
		if sig.IndicatorSnapshot == nil {
			t.Errorf("persisted signal missing decision-time indicator snapshot (US7 FR-022)")
		}
	})
}
