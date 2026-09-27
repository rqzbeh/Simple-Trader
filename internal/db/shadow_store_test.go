package db

import "testing"

// T042/T008: validation rules per data-model.md (FR-007 error rows).
func TestShadowDecisionValidation(t *testing.T) {
	ok := &ShadowDecision{JudgmentType: "entry", CycleID: "c1", Judge: "jev", Status: "ok", Choice: "LONG"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid row rejected: %v", err)
	}
	errNoMsg := &ShadowDecision{JudgmentType: "entry", CycleID: "c1", Judge: "jev", Status: "error"}
	if err := errNoMsg.Validate(); err == nil {
		t.Fatal("error row without message must be rejected (FR-007)")
	}
	badChoice := &ShadowDecision{JudgmentType: "entry", CycleID: "c1", Judge: "jev", Status: "ok", Choice: "BUY"}
	if err := badChoice.Validate(); err == nil {
		t.Fatal("BUY violates position-intent vocabulary (FR-005)")
	}
	errWithVal := &ShadowDecision{JudgmentType: "news", CycleID: "c1", Judge: "jev", Status: "error", Error: "x", Choice: "BULLISH"}
	if err := errWithVal.Validate(); err == nil {
		t.Fatal("error rows must carry no judgment values")
	}
	p := 0.5
	exitOK := &ShadowDecision{JudgmentType: "exit", CycleID: "c1", Judge: "jev", Status: "ok", Noul: &p}
	if err := exitOK.Validate(); err != nil {
		t.Fatalf("valid exit rejected: %v", err)
	}
}
