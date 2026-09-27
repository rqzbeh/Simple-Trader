package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

type memSink struct {
	mu sync.Mutex
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
