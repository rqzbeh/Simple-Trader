package trader

import (
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// TestScoreEV_DocsShape: TypeSafe Score answers are probability-weighted
// LEVEL INDEXES with probabilities keyed by level index (docs.typesafe.ai/api).
// The registry must convert them to semantic values via expected value —
// using the raw index as the value produced confluence thresholds of 1.0 and
// vetoed every BUY (2026-09-29).
func TestScoreEV_DocsShape(t *testing.T) {
	reg := NewParamRegistry(&config.Config{})

	// confluence EV: 0.5×level1(0.30) + 0.5×level2(0.42) = 0.36
	ans := &ai.JevAnswer{
		Type:          "score",
		Score:         ptr(1.5),
		Confidence:    0.7,
		Probabilities: map[string]float64{"1": 0.5, "2": 0.5},
	}
	res, err := reg.Resolve("confluence", ans, "ev-1")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if v := res.Value.(float64); v < 0.359 || v > 0.361 {
		t.Errorf("confluence EV = %v, want 0.36 (level-index probability weighted)", v)
	}

	// level key outside the rubric = schema error (zero-fallback)
	bad := &ai.JevAnswer{Type: "score", Probabilities: map[string]float64{"7": 1.0}}
	if _, err := reg.Resolve("confluence", bad, "ev-2"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("out-of-range level key must fail loud, got %v", err)
	}

	// probabilities that do not sum to 1 = schema error
	sum := &ai.JevAnswer{Type: "score", Probabilities: map[string]float64{"0": 0.5, "1": 0.3}}
	if _, err := reg.Resolve("confluence", sum, "ev-3"); err == nil || !strings.Contains(err.Error(), "sum") {
		t.Errorf("probability sum must be validated, got %v", err)
	}

	// conviction EV over risk-band fractions: 0.3×0.5 + 0.7×0.75 = 0.675
	// → risk = min + 0.675×(max−min) with cfg bounds 0.005..0.02
	cReg := NewParamRegistry(&config.Config{MinRiskPerTradePct: 0.005, MaxRiskPerTradePct: 0.02})
	cAns := &ai.JevAnswer{Type: "score", Probabilities: map[string]float64{"1": 0.3, "2": 0.7}}
	cRes, err := cReg.Resolve("conviction", cAns, "ev-4")
	if err != nil {
		t.Fatalf("conviction resolve failed: %v", err)
	}
	want := 0.005 + 0.675*(0.02-0.005)
	if v := cRes.Value.(float64); v < want-1e-9 || v > want+1e-9 {
		t.Errorf("conviction EV risk = %v, want %v", v, want)
	}
}

func ptr(f float64) *float64 { return &f }
