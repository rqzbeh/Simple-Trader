package trader

import (
	"errors"
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

func TestTimeframeQuestion_Alpha(t *testing.T) {
	q := TimeframeQuestion("ALPHA")
	if q.Type != "choice" {
		t.Fatalf("expected question type 'choice', got %q", q.Type)
	}

	criteria, ok := q.Criteria.(map[string]string)
	if !ok {
		t.Fatalf("expected q.Criteria to be map[string]string, got %T", q.Criteria)
	}

	expectedSet := config.GetBucketTimeframeSet("ALPHA")
	if len(criteria) != len(expectedSet) {
		t.Fatalf("expected %d criteria keys, got %d", len(expectedSet), len(criteria))
	}

	for _, opt := range expectedSet {
		text, ok := criteria[opt]
		if !ok {
			t.Errorf("expected criteria key %q not found", opt)
			continue
		}
		// FR-208: criteria include horizon minutes
		prof, _ := config.GetTimeframeProfile(opt)
		if !strings.Contains(text, "horizon") {
			t.Errorf("criteria for %s missing 'horizon': %s", opt, text)
		}
		_ = prof
	}

	// ALPHA never offers 12h
	if _, ok := criteria["12h"]; ok {
		t.Errorf("ALPHA criteria must never include 12h")
	}

	// FR-208: ALPHA 1h includes catalyst decay guidance
	text1h := criteria["1h"]
	if !strings.Contains(text1h, "≤6h") && !strings.Contains(text1h, "<=6h") && !strings.Contains(text1h, "6h") {
		t.Errorf("criteria for 1h missing decay guidance (<=6h): %s", text1h)
	}
}

func TestTimeframeQuestion_Core(t *testing.T) {
	q := TimeframeQuestion("CORE")
	criteria, ok := q.Criteria.(map[string]string)
	if !ok {
		t.Fatalf("expected q.Criteria to be map[string]string, got %T", q.Criteria)
	}
	expectedSet := config.GetBucketTimeframeSet("CORE")
	if len(criteria) != len(expectedSet) {
		t.Fatalf("expected %d criteria keys, got %d", len(expectedSet), len(criteria))
	}
	for _, opt := range expectedSet {
		if _, ok := criteria[opt]; !ok {
			t.Errorf("expected criteria key %q not found in CORE", opt)
		}
	}
	if _, ok := criteria["12h"]; !ok {
		t.Errorf("CORE criteria should include 12h")
	}
}

func TestValidateTimeframe_Success(t *testing.T) {
	bucketSet := []string{"15m", "1h", "4h"}
	ans := ai.JevAnswer{
		Type:       "choice",
		Choice:     "1h",
		Confidence: 0.85,
		Probabilities: map[string]float64{
			"15m": 0.10,
			"1h":  0.75,
			"4h":  0.15,
		},
	}

	err := ValidateTimeframe(ans, bucketSet, "cycle-101")
	if err != nil {
		t.Fatalf("expected valid timeframe answer to pass, got: %v", err)
	}
}

func TestValidateTimeframe_Errors(t *testing.T) {
	bucketSet := []string{"15m", "1h", "4h"}

	// 1. Missing / invalid choice
	ansInvalidChoice := ai.JevAnswer{
		Type:       "choice",
		Choice:     "2h", // not in bucketSet
		Confidence: 0.8,
		Probabilities: map[string]float64{
			"15m": 0.1, "1h": 0.7, "4h": 0.2,
		},
	}
	err := ValidateTimeframe(ansInvalidChoice, bucketSet, "cycle-err-1")
	if err == nil {
		t.Fatalf("expected error for choice '2h' not in bucket set")
	}
	var decErr *ai.DecisionError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *ai.DecisionError, got %T: %v", err, err)
	}
	if decErr.Component != "timeframe" {
		t.Errorf("expected component='timeframe', got %q", decErr.Component)
	}
	if !errors.Is(err, ai.ErrJevSchema) {
		t.Errorf("expected ErrJevSchema, got %v", err)
	}

	// 2. Missing answer
	ansEmpty := ai.JevAnswer{}
	err = ValidateTimeframe(ansEmpty, bucketSet, "cycle-err-2")
	if err == nil {
		t.Fatalf("expected error for empty answer")
	}
	if !errors.As(err, &decErr) || decErr.Component != "timeframe" {
		t.Errorf("expected *ai.DecisionError with component=timeframe, got: %v", err)
	}

	// 3. Distribution key outside set
	ansBadDist := ai.JevAnswer{
		Type:       "choice",
		Choice:     "1h",
		Confidence: 0.9,
		Probabilities: map[string]float64{
			"15m": 0.1, "1h": 0.7, "12h": 0.2, // 12h not in bucketSet
		},
	}
	err = ValidateTimeframe(ansBadDist, bucketSet, "cycle-err-3")
	if err == nil {
		t.Fatalf("expected error for distribution key outside set")
	}
	if !errors.As(err, &decErr) || decErr.Component != "timeframe" {
		t.Errorf("expected *ai.DecisionError with component=timeframe, got: %v", err)
	}

	// 4. Missing distribution key from set
	ansMissingDistKey := ai.JevAnswer{
		Type:       "choice",
		Choice:     "1h",
		Confidence: 0.9,
		Probabilities: map[string]float64{
			"15m": 0.2, "1h": 0.8, // 4h missing
		},
	}
	err = ValidateTimeframe(ansMissingDistKey, bucketSet, "cycle-err-4")
	if err == nil {
		t.Fatalf("expected error for missing distribution key from set")
	}
	if !errors.As(err, &decErr) || decErr.Component != "timeframe" {
		t.Errorf("expected *ai.DecisionError with component=timeframe, got: %v", err)
	}
}

func TestTimeframeQuestion_LiveConfigMutation(t *testing.T) {
	defer func() {
		_ = config.UpdateTimeframeSet("TIMEFRAME_SET_ALPHA", "15m,1h,4h")
	}()

	// 1. Initial set contains 15m, 1h, 4h
	q1 := TimeframeQuestion("ALPHA")
	crit1 := q1.Criteria.(map[string]string)
	if _, ok := crit1["15m"]; !ok {
		t.Fatalf("expected 15m in initial ALPHA criteria")
	}

	// 2. Mutate config at runtime (quickstart V6)
	err := config.UpdateTimeframeSet("TIMEFRAME_SET_ALPHA", "1h,4h")
	if err != nil {
		t.Fatalf("failed updating timeframe set: %v", err)
	}

	// 3. Question builder reflects new set immediately
	q2 := TimeframeQuestion("ALPHA")
	crit2 := q2.Criteria.(map[string]string)
	if len(crit2) != 2 {
		t.Fatalf("expected 2 criteria keys after mutation, got %d", len(crit2))
	}
	if _, ok := crit2["15m"]; ok {
		t.Errorf("15m should have been removed from criteria after mutation")
	}
	if _, ok := crit2["1h"]; !ok {
		t.Errorf("1h should be in criteria")
	}
	if _, ok := crit2["4h"]; !ok {
		t.Errorf("4h should be in criteria")
	}
}
