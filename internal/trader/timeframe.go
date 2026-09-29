package trader

import (
	"fmt"
	"math"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// standardCriteria embeds horizon minutes and catalyst-decay guidance (FR-208, contracts §1).
var standardCriteria = map[string]string{
	"15m": "Fast scalp: catalyst <30m old or volatile breakout; horizon 45m",
	"30m": "Momentum scalp: fresh catalyst 30m-1h old; horizon 90m",
	"1h":  "Standard: catalyst 1h-4h old; horizon 120m; news decay ≤6h",
	"2h":  "Intermediate: catalyst 4h-8h old; horizon 180m",
	"4h":  "Macro/structural: catalyst 8h-24h old; horizon 360m",
	"6h":  "Extended trend: multi-session catalyst >24h; horizon 540m",
	"12h": "Session-scale commodity catalyst >24h; horizon 720m",
	"1d":  "Multi-day structural trend: major macro regime shift; horizon 1440m",
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
// When bucketSet is empty/unrestricted, any key present in config.KnownIntervals is accepted (FR-602).
// Returns a typed DecisionError with component="timeframe" on failure (FR-202, SC-202).
func ValidateTimeframe(ans ai.JevAnswer, bucketSet []string, cycleID string) error {
	if ans.Type != "choice" || ans.Choice == "" {
		return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, "missing timeframe answer or invalid type")
	}

	if len(bucketSet) == 0 || len(bucketSet) == len(config.AllKnownIntervals) {
		if !config.KnownIntervals[ans.Choice] {
			return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("choice %q not in known intervals", ans.Choice))
		}
		if len(ans.Probabilities) == 0 {
			return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, "empty timeframe probability distribution")
		}
		var sum float64
		for k, prob := range ans.Probabilities {
			if !config.KnownIntervals[k] {
				return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("probability key %q outside known intervals", k))
			}
			sum += prob
		}
		if math.Abs(sum-1.0) > 0.05 {
			return ai.WrapDecision("timeframe", cycleID, ai.ErrJevSchema, fmt.Sprintf("probabilities sum %.4f != 1", sum))
		}
		return nil
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
