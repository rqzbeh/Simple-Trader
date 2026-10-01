package ai

import (
	"errors"
	"fmt"
)

// Typed decision-core errors (spec-013 FR-007). Every failure names the
// component and the cycle it broke. No default, heuristic, or substituted
// answer is ever returned in place of these errors.

var (
	ErrJevAuth        = errors.New("jev: authentication failed")
	ErrJevTimeout     = errors.New("jev: request timeout")
	ErrJevSchema      = errors.New("jev: answer schema mismatch")
	ErrJevRateLimit   = errors.New("jev: rate limited")
	ErrJevUnavailable = errors.New("jev: service unavailable")

	ErrLLMClassify = errors.New("llm-classifier: classification failed")
	ErrLLMTimeout  = errors.New("llm-classifier: timeout")

	ErrConfigMissing = errors.New("config: required decision-core setting missing")
)

// DecisionError carries component + cycle context for explicit surfacing.
type DecisionError struct {
	Component string
	CycleID   string
	Cause     error
	Detail    string
}

func (e *DecisionError) Error() string {
	return fmt.Sprintf("component=%s cycle=%s: %v (%s)", e.Component, e.CycleID, e.Cause, e.Detail)
}

func (e *DecisionError) Unwrap() error { return e.Cause }

// WrapDecision attaches component and cycle id to a sentinel cause.
func WrapDecision(component, cycleID string, cause error, detail string) error {
	return &DecisionError{Component: component, CycleID: cycleID, Cause: cause, Detail: detail}
}
