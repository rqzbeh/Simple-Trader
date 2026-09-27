package trader

import (
	"context"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

// DecisionRouter: Jev-first, 9Router-escalated (spec-013 FR-001/FR-003).
// Exactly one final writer per decision.
type DecisionRouter struct {
	Jev       *ai.JevClient
	Threshold float64 // ROUTING_CONFIDENCE_THRESHOLD — no default; startup error if unset
	Escalate  func(ctx context.Context, state interface{}) (DecisionOutcome, error)
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

// Route runs Jev first; escalates when confidence < Threshold.
func (r *DecisionRouter) Route(ctx context.Context, cycleID string, state interface{}, questions map[string]ai.JevQuestion, allowed map[string]bool) (DecisionOutcome, error) {
	if r.Threshold <= 0 {
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
	if ans.Confidence >= r.Threshold {
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
