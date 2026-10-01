package trader

import (
	"context"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// isNil reports whether interface i is nil or holds a typed nil pointer.
func isNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// ShadowOrchestrator runs core judgments asynchronously AFTER the live
// decision is committed — never blocks or alters the live path (FR-001).
type ShadowOrchestrator struct {
	Router *DecisionRouter
	Store  ShadowDecisionSink

	mu      sync.RWMutex
	enabled map[string]bool // entry|exit|news toggles (FR-016)
	queue   chan func()
	once    sync.Once
}

// ShadowDecisionSink abstracts persistence for testability.
type ShadowDecisionSink interface {
	InsertShadowDecision(ctx context.Context, d *db.ShadowDecision) error
}

// SetEnabled toggles judgment types at runtime (FR-010).
func (o *ShadowOrchestrator) SetEnabled(judgmentType string, on bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.enabled == nil {
		o.enabled = map[string]bool{}
	}
	o.enabled[judgmentType] = on
}

func (o *ShadowOrchestrator) active(judgmentType string) bool {
	if o == nil {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.enabled[judgmentType]
}

// Start spawns the bounded worker (queue-full ⇒ explicit error record).
func (o *ShadowOrchestrator) Start(workers, queueSize int) {
	if o == nil {
		return
	}
	o.once.Do(func() {
		if queueSize <= 0 {
			queueSize = 64
		}
		o.queue = make(chan func(), queueSize)
		for i := 0; i < workers && i < 4; i++ {
			go func(workerID int) {
				for fn := range o.queue {
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("[ERROR] shadow worker %d recovered from panic: %v", workerID, r)
							}
						}()
						fn()
					}()
				}
			}(i)
		}
	})
}

// Enqueue runs fn off the caller's goroutine. Queue-full returns an explicit
// error — never silently drops (FR-007).
func (o *ShadowOrchestrator) Enqueue(fn func()) error {
	if o == nil || o.queue == nil {
		return ai.WrapDecision("shadow", "none", ai.ErrConfigMissing, "orchestrator not started")
	}
	select {
	case o.queue <- fn:
		return nil
	default:
		return ai.WrapDecision("shadow", "none", ai.ErrConfigMissing, "shadow queue full")
	}
}

// Record persists one judgment outcome; error rows carry the failure text.
func (o *ShadowOrchestrator) Record(ctx context.Context, d *db.ShadowDecision) error {
	cycleID := ""
	if d != nil {
		cycleID = d.CycleID
	}
	if o == nil {
		return ai.WrapDecision("shadow", cycleID, ai.ErrConfigMissing, "orchestrator is nil")
	}
	if o.Store == nil || isNil(o.Store) {
		return ai.WrapDecision("shadow", cycleID, ai.ErrConfigMissing, "store not configured")
	}
	if d == nil {
		return ai.WrapDecision("shadow", "", ai.ErrConfigMissing, "decision is nil")
	}
	return o.Store.InsertShadowDecision(ctx, d)
}

// JudgeEntry fires a post-commit entry judgment (async).
func (o *ShadowOrchestrator) JudgeEntry(cycleID, symbol string, state interface{}, questions map[string]ai.JevQuestion, allowed map[string]bool, baseline string) {
	if o == nil || !o.active("entry") {
		return
	}
	_ = o.Enqueue(func() {
		if o.Router == nil {
			log.Printf("[WARN] shadow JudgeEntry %s %s: router is nil, skipping", cycleID, symbol)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d := &db.ShadowDecision{
			JudgmentType: "entry", CycleID: cycleID, Symbol: symbol,
			Judge: "jev", StateRef: symbol, BaselineChoice: baseline,
		}
		out, err := o.Router.Route(ctx, cycleID, state, questions, allowed)
		if err != nil {
			d.Status, d.Error, d.Judge = "error", err.Error(), "jev"
		} else {
			d.Status, d.Choice, d.Route = "ok", out.Choice, out.Route
			conf := out.Confidence
			d.Confidence = &conf
			d.Probabilities = out.ProbDist
			d.LatencyMS = int(out.JevLatency / time.Millisecond)
			d.InputTokens, d.OutputTokens = out.JevUsage.InputTokens, out.JevUsage.OutputTokens
		}
		if o.Store == nil || isNil(o.Store) {
			log.Printf("[WARN] shadow JudgeEntry %s %s: store is nil, skipping persistence", cycleID, symbol)
			return
		}
		recCtx, recCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer recCancel()
		if err := o.Record(recCtx, d); err != nil {
			log.Printf("[WARN] shadow JudgeEntry %s %s: record failed: %v", cycleID, symbol, err)
		}
	})
}
