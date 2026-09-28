package trader

import (
	"fmt"
	"math"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// standardCriteria embeds horizon minutes and catalyst-decay guidance (FR-208, contracts §1).
var standardCriteria = map[string]string{
	"15m": "Scalp: catalyst <1h old or volatile breakout; horizon 45m",
	"30m": "Momentum scalp: fresh catalyst <2h; horizon 90m",
	"1h":  "Standard: fresh news 1-3h; horizon 120m; news dies ≤6h",
	"2h":  "Intermediate: catalyst persists 2-4h; horizon 180m",
	"4h":  "Macro/structural: catalyst persists; horizon 360m",
	"6h":  "Extended trend: multi-session catalyst; horizon 540m",
	"12h": "Session-scale commodity catalyst; horizon 720m",
	"1d":  "Multi-day structural trend: macro catalyst; horizon 1440m",
}

// TimeframeQuestion generates the dynamic timeframe Choice question for a given bucket.
// Criteria keys are generated FROM the configured set (config owns options, FR-203, contracts §1).
func TimeframeQuestion(bucket string) ai.JevQuestion {
	set := config.GetBucketTimeframeSet(bucket)
	criteria := make(map[string]string, len(set))
	for _, opt := range set {
		if text, ok := standardCriteria[opt]; ok {
			criteria[opt] = text
		} else if prof, ok := config.GetTimeframeProfile(opt); ok {
			criteria[opt] = fmt.Sprintf("Execution timeframe %s; horizon %dm", opt, prof.HorizonMin)
		} else {
			criteria[opt] = fmt.Sprintf("Execution timeframe %s", opt)
		}
	}

	return ai.JevQuestion{
		Type: "choice",
		Instructions: map[string]interface{}{
			"question": "Best execution timeframe for this trade given catalyst age and structure?",
			"not_for":  "interval plumbing; code fetches candles for the chosen label",
		},
		Criteria: criteria,
	}
}

// ValidateTimeframe checks that the timeframe answer choice is in bucketSet and distribution matches.
// Returns a typed DecisionError with component="timeframe" on failure (FR-202, SC-202).
func ValidateTimeframe(ans ai.JevAnswer, bucketSet []string, cycleID string) error {
	if ans.Type != "choice" || ans.Choice == "" {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, "missing timeframe answer or invalid type")
	}

	setMap := make(map[string]bool, len(bucketSet))
	for _, opt := range bucketSet {
		setMap[opt] = true
	}

	if !setMap[ans.Choice] {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("choice %q not in bucket set %v", ans.Choice, bucketSet))
	}

	if len(ans.Probabilities) == 0 {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, "empty timeframe probability distribution")
	}

	if len(ans.Probabilities) != len(bucketSet) {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("distribution key count %d != set size %d", len(ans.Probabilities), len(bucketSet)))
	}

	var sum float64
	for k, prob := range ans.Probabilities {
		if !setMap[k] {
			return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("probability key %q outside bucket set %v", k, bucketSet))
		}
		sum += prob
	}

	for _, opt := range bucketSet {
		if _, ok := ans.Probabilities[opt]; !ok {
			return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("missing probability key %q from distribution", opt))
		}
	}

	if math.Abs(sum-1.0) > 0.05 {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("probabilities sum %.4f != 1", sum))
	}

	return nil
}
