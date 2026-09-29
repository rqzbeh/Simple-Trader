package trader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
)

// DecisionRouter: Jev-first, 9Router-escalated (spec-013 FR-001/FR-003).
// Exactly one final writer per decision.
type DecisionRouter struct {
	mu        sync.RWMutex
	Jev       *ai.JevClient
	Threshold float64 // ROUTING_CONFIDENCE_THRESHOLD — no default; startup error if unset
	Escalate  func(ctx context.Context, state interface{}) (DecisionOutcome, error)
}

// SetThreshold dynamically updates the routing confidence threshold live.
func (r *DecisionRouter) SetThreshold(thr float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Threshold = thr
}

// GetThreshold returns the current routing confidence threshold.
func (r *DecisionRouter) GetThreshold() float64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Threshold
}

// DecisionOutcome is the core's final typed answer.
type DecisionOutcome struct {
	Choice     string
	Confidence float64
	ProbDist   map[string]float64
	Route      string // jev_direct | escalated
	Noul       *float64
	JevLatency time.Duration
	LLMLatency time.Duration
	JevUsage   ai.JevUsage
	Baseline   string
}

// ErrThresholdMissing returned when config absent (FR-016 — startup error, no default).
var ErrThresholdMissing = ai.ErrConfigMissing

// EscalationPayload normalizes the value handed to Escalate into a real
// DecisionRequest. Escalation judges the SAME evidence as the entry cycle —
// never an empty body (spec-013 FR-003/FR-007). Unknown payloads are an
// explicit error, not a degraded request.
func EscalationPayload(payload interface{}) (ai.DecisionRequest, error) {
	switch v := payload.(type) {
	case ai.DecisionRequest:
		return v, nil
	case StateObject:
		return stateToRequest(v), nil
	default:
		return ai.DecisionRequest{}, ai.WrapDecision("router", "escalation", ai.ErrLLMClassify,
			fmt.Sprintf("unsupported escalation payload type %T — refusing empty request", payload))
	}
}

// stateToRequest rebuilds the numeric indicator snapshot from a StateObject
// (inverse of snapToMap) so shadow-side escalation sees the same numbers the
// entry cycle saw.
func stateToRequest(v StateObject) ai.DecisionRequest {
	return ai.DecisionRequest{
		Symbol: v.Symbol,
		IndicatorSnap: cache.IndicatorSnapshot{
			Symbol:          v.Symbol,
			RSI:             v.Indicators["rsi"],
			MACD:            v.Indicators["macd"],
			Signal:          v.Indicators["macd_signal"],
			Histogram:       v.Indicators["macd_histogram"],
			UpperBand:       v.Indicators["bb_upper"],
			MiddleBand:      v.Indicators["bb_middle"],
			LowerBand:       v.Indicators["bb_lower"],
			ConfluenceScore: v.Indicators["confluence"],
			OBI:             v.Indicators["obi"],
		},
	}
}

// Route runs Jev first; escalates when confidence < Threshold.
func (r *DecisionRouter) Route(ctx context.Context, cycleID string, state interface{}, questions map[string]ai.JevQuestion, allowed map[string]bool) (DecisionOutcome, error) {
	thr := r.GetThreshold()
	if thr <= 0 {
		return DecisionOutcome{}, ai.WrapDecision("config", cycleID, ErrThresholdMissing, "ROUTING_CONFIDENCE_THRESHOLD")
	}
	answers, usage, err := r.Jev.Evaluate(ctx, cycleID, state, questions)
	if err != nil {
		return DecisionOutcome{}, err
	}
	// Primary answer = "entry"/"direction"/"news_impact" choice, first present.
	var primaryID string
	for id := range questions {
		if id == "entry" || id == "direction" || id == "news_impact" {
			primaryID = id
			break
		}
	}
	if primaryID == "" {
		for id := range questions {
			primaryID = id
			break
		}
	}
	ans := answers[primaryID]
	if err := ai.ValidateChoice(ans, allowed, cycleID); err != nil {
		return DecisionOutcome{}, err
	}

	out := DecisionOutcome{
		Choice:     ans.Choice,
		Confidence: ans.Confidence,
		ProbDist:   ans.Probabilities,
		Route:      "jev_direct",
		JevLatency: usage.Latency,
		JevUsage:   usage,
	}
	if ans.Noul != nil {
		out.Noul = ans.Noul
	}

	// One final writer: escalate ONLY when below threshold. Never both.
	if ans.Confidence >= thr {
		return out, nil
	}
	if r.Escalate == nil {
		return DecisionOutcome{}, ai.WrapDecision("router", cycleID, ai.ErrLLMClassify, "confidence below threshold but no escalation path configured")
	}
	start := time.Now()
	esc, err := r.Escalate(ctx, state)
	if err != nil {
		// Explicit failure: do NOT downgrade to Jev guess (edge case FR-007).
		return DecisionOutcome{}, ai.WrapDecision("router", cycleID, ai.ErrLLMClassify, "escalation failed: "+err.Error())
	}
	esc.Route = "escalated"
	esc.LLMLatency = time.Since(start)
	// Record both steps: Jev's low-confidence answer retained as baseline evidence.
	esc.Baseline = ans.Choice
	if esc.JevLatency == 0 {
		esc.JevLatency = usage.Latency
	}
	if len(esc.ProbDist) == 0 {
		esc.ProbDist = ans.Probabilities
	}
	if esc.Confidence == 0 {
		esc.Confidence = ans.Confidence
	}
	return esc, nil
}
