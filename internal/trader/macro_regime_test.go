package trader_test

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/trader"
)

func TestMacroRegimeEngine_Calculations(t *testing.T) {
	engine := trader.NewMacroRegimeEngine()

	t.Run("Default Baseline is Crisis or Elevated Stress", func(t *testing.T) {
		state := engine.GetCurrentState()
		if state.Score <= 0 {
			t.Fatalf("expected positive macro score, got %f", state.Score)
		}
		totalPct := state.TargetTier1Pct + state.TargetCorePct + state.TargetAlphaPct
		if totalPct < 0.999 || totalPct > 1.001 {
			t.Fatalf("expected allocations to sum to 1.0, got %f", totalPct)
		}
	})

	t.Run("Extreme Geopolitical Crisis Regime (Score >= 0.65)", func(t *testing.T) {
		crisisIndicators := trader.MacroIndicators{
			GeopoliticalIndex: 0.95, // Widespread regional war
			InflationIndex:    0.80, // High inflation
			InterestRateIndex: 0.70, // Hawkish rates
		}

		state := engine.CalculateRegime(crisisIndicators)

		if state.Regime != trader.RegimeCrisis {
			t.Fatalf("expected regime CRISIS, got %s (score: %f)", state.Regime, state.Score)
		}
		if state.Score < 0.65 {
			t.Fatalf("expected score >= 0.65, got %f", state.Score)
		}

		// In Crisis: Core Preservation expands to >= 55%, Cash expands to >= 20%, Tactical Alpha contracts to <= 25%
		if state.TargetCorePct < 0.55 {
			t.Errorf("expected Core target >= 0.55 in crisis, got %f", state.TargetCorePct)
		}
		if state.TargetTier1Pct < 0.20 {
			t.Errorf("expected Tier 1 Cash target >= 0.20 in crisis, got %f", state.TargetTier1Pct)
		}
		if state.TargetAlphaPct > 0.25 {
			t.Errorf("expected Tactical Alpha target <= 0.25 in crisis, got %f", state.TargetAlphaPct)
		}

		sum := state.TargetTier1Pct + state.TargetCorePct + state.TargetAlphaPct
		if sum < 0.999 || sum > 1.001 {
			t.Errorf("expected sum of targets = 1.0, got %f", sum)
		}
	})

	t.Run("Normal Balanced Regime (0.35 <= Score < 0.65)", func(t *testing.T) {
		normalIndicators := trader.MacroIndicators{
			GeopoliticalIndex: 0.40,
			InflationIndex:    0.50,
			InterestRateIndex: 0.50,
		}

		state := engine.CalculateRegime(normalIndicators)

		if state.Regime != trader.RegimeNormal {
			t.Fatalf("expected regime NORMAL, got %s (score: %f)", state.Regime, state.Score)
		}
		if state.Score < 0.35 || state.Score >= 0.65 {
			t.Fatalf("expected score between 0.35 and 0.65, got %f", state.Score)
		}

		// Standard 3-Tier Allocation (15% Cash, 45% Core, 40% Alpha)
		if state.TargetTier1Pct != 0.15 {
			t.Errorf("expected Tier 1 Cash = 0.15, got %f", state.TargetTier1Pct)
		}
		if state.TargetCorePct != 0.45 {
			t.Errorf("expected Core = 0.45, got %f", state.TargetCorePct)
		}
		if state.TargetAlphaPct != 0.40 {
			t.Errorf("expected Tactical Alpha = 0.40, got %f", state.TargetAlphaPct)
		}
	})

	t.Run("Dovish Expansion Regime (Score < 0.35)", func(t *testing.T) {
		expansionIndicators := trader.MacroIndicators{
			GeopoliticalIndex: 0.10, // Global stability
			InflationIndex:    0.20, // Low inflation
			InterestRateIndex: 0.15, // Accommodative rate cuts
		}

		state := engine.CalculateRegime(expansionIndicators)

		if state.Regime != trader.RegimeDovishExpansion {
			t.Fatalf("expected regime DOVISH_EXPANSION, got %s (score: %f)", state.Regime, state.Score)
		}
		if state.Score >= 0.35 {
			t.Fatalf("expected score < 0.35, got %f", state.Score)
		}

		// In Dovish Expansion: Tactical Alpha expands to >= 45%, Core <= 40%, Cash = 15%
		if state.TargetAlphaPct < 0.45 {
			t.Errorf("expected Tactical Alpha target >= 0.45 in expansion, got %f", state.TargetAlphaPct)
		}
		if state.TargetCorePct > 0.40 {
			t.Errorf("expected Core target <= 0.40 in expansion, got %f", state.TargetCorePct)
		}
		if state.TargetTier1Pct != 0.15 {
			t.Errorf("expected Cash target = 0.15, got %f", state.TargetTier1Pct)
		}

		sum := state.TargetTier1Pct + state.TargetCorePct + state.TargetAlphaPct
		if sum < 0.999 || sum > 1.001 {
			t.Errorf("expected sum of targets = 1.0, got %f", sum)
		}
	})

	t.Run("Indicator Clamping and Update", func(t *testing.T) {
		outOfBounds := trader.MacroIndicators{
			GeopoliticalIndex: 5.0,  // should clamp to 1.0
			InflationIndex:    -2.0, // should clamp to 0.0
			InterestRateIndex: 1.5,  // should clamp to 1.0
		}

		updated := engine.UpdateIndicators(outOfBounds)
		if updated.Score <= 0 || updated.Score > 1.0 {
			t.Fatalf("expected score clamped between 0 and 1, got %f", updated.Score)
		}

		// Verify state was saved to engine
		current := engine.GetCurrentState()
		if current.Score != updated.Score {
			t.Fatalf("expected engine state to match updated score")
		}
	})
}
