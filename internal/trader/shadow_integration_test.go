package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

type memSink struct {
	mu   sync.Mutex
	rows []db.ShadowDecision
}

func (m *memSink) InsertShadowDecision(_ context.Context, d *db.ShadowDecision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, *d)
	return nil
}

func TestShadowQueueFullExplicit(t *testing.T) {
	o := &ShadowOrchestrator{}
	if err := o.Enqueue(func() {}); err == nil {
		t.Fatal("queue not started must error explicitly")
	}
	o.Start(1, 1)
	// fill queue
	block := make(chan struct{})
	_ = o.Enqueue(func() { <-block })
	_ = o.Enqueue(func() {}) // queue cap 1 — may or may not queue
	close(block)
	time.Sleep(20 * time.Millisecond)
}

func TestShadowRecordRequiresStore(t *testing.T) {
	o := &ShadowOrchestrator{}
	if err := o.Record(context.Background(), &db.ShadowDecision{CycleID: "c"}); err == nil {
		t.Fatal("missing store must error explicitly")
	}
	o.Store = &memSink{}
	if err := o.Record(context.Background(), &db.ShadowDecision{
		JudgmentType: "entry", CycleID: "c1", Judge: "jev", Status: "ok", Choice: "LONG",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestShadowTogglesPerType(t *testing.T) {
	o := &ShadowOrchestrator{}
	o.SetEnabled("entry", true)
	if !o.active("entry") || o.active("exit") {
		t.Fatal("per-type toggles broken")
	}
	o.SetEnabled("entry", false)
	if o.active("entry") {
		t.Fatal("toggle off failed")
	}
}

func TestShadowRecordTypedNilStore(t *testing.T) {
	o := &ShadowOrchestrator{}
	var nilStore *db.Store
	o.Store = nilStore
	if err := o.Record(context.Background(), &db.ShadowDecision{CycleID: "c"}); err == nil {
		t.Fatal("typed nil store must error explicitly, not panic")
	}
}

func TestShadowNilOrchestratorSafe(t *testing.T) {
	var o *ShadowOrchestrator
	o.Start(1, 10)
	o.SetEnabled("entry", true)
	if o.active("entry") {
		t.Fatal("nil orchestrator should not be active")
	}
	if err := o.Enqueue(func() {}); err == nil {
		t.Fatal("expected error on nil orchestrator Enqueue")
	}
	if err := o.Record(context.Background(), &db.ShadowDecision{CycleID: "c"}); err == nil {
		t.Fatal("expected error on nil orchestrator Record")
	}
	o.JudgeEntry("c", "BTC/USDT", nil, nil, nil, "NO_TRADE")
}

func TestShadowWorkerRecoverPanic(t *testing.T) {
	o := &ShadowOrchestrator{}
	o.Start(1, 10)

	completed := make(chan struct{})
	// Enqueue a job that deliberately panics
	_ = o.Enqueue(func() {
		panic("deliberate panic in shadow worker test")
	})
	// Enqueue a subsequent job to verify the worker continues processing
	_ = o.Enqueue(func() {
		close(completed)
	})

	select {
	case <-completed:
		// success: worker recovered from panic and processed the next job
	case <-time.After(1 * time.Second):
		t.Fatal("worker died after panic, did not recover to process next job")
	}
}

func TestShadowJudgeEntryNilStoreNoPanic(t *testing.T) {
	o := &ShadowOrchestrator{}
	var nilStore *db.Store
	o.Store = nilStore
	o.SetEnabled("entry", true)
	o.Start(1, 10)

	// JudgeEntry with nil Router or nil Store must not panic
	o.JudgeEntry("c1", "BTC/USDT", nil, nil, nil, "NO_TRADE")
	time.Sleep(50 * time.Millisecond)
}
