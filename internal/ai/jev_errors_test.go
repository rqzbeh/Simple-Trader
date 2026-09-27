package ai

import (
	"errors"
	"testing"
)

// T004: typed error contract — every error names component + cycle, no default answers (FR-007).
func TestDecisionErrorCarriesComponentAndCycle(t *testing.T) {
	cases := []struct {
		comp string
		cyc  string
		base error
	}{
		{"jev", "cyc-1", ErrJevTimeout},
		{"jev", "cyc-2", ErrJevSchema},
		{"jev", "cyc-3", ErrJevAuth},
		{"llm", "cyc-4", ErrLLMClassify},
		{"llm", "cyc-5", ErrLLMTimeout},
		{"config", "boot", ErrConfigMissing},
	}
	for _, c := range cases {
		err := WrapDecision(c.comp, c.cyc, c.base, "detail")
		if !errors.Is(err, c.base) {
			t.Fatalf("errors.Is(%s) failed", c.base)
		}
		msg := err.Error()
		if msg == "" || !containsAll(msg, c.comp, c.cyc) {
			t.Fatalf("error message must contain component and cycle: got %q", msg)
		}
	}
}

// Analyze must return an error (never a heuristic answer) when unconfigured.
func TestAnalyzeUnconfiguredReturnsExplicitError(t *testing.T) {
	c := NewClient(ClientConfig{})
	resp, err := c.Analyze(t.Context(), DecisionRequest{Symbol: "BTCUSDT"})
	if err == nil {
		t.Fatalf("expected explicit error, got resp=%v", resp)
	}
	if resp != nil {
		t.Fatalf("expected nil response on error, got %v", resp)
	}
	if !containsAll(err.Error(), "component=llm", "unconfigured") {
		t.Fatalf("error not explicit: %q", err.Error())
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
