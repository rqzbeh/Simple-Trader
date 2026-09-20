package trader

import (
	"math"
	"sync"
	"time"
)

// MacroRegimeType categorizes the prevailing global macroeconomic state.
type MacroRegimeType string

const (
	RegimeCrisis          MacroRegimeType = "CRISIS"
	RegimeNormal          MacroRegimeType = "NORMAL"
	RegimeDovishExpansion MacroRegimeType = "DOVISH_EXPANSION"
)

// MacroIndicators represents the quantitative inputs for real-world stress calculation.
type MacroIndicators struct {
	GeopoliticalIndex float64  `json:"geopolitical_index"` // 0.0 (peace) to 1.0 (war/blockade)
	InflationIndex    float64  `json:"inflation_index"`    // 0.0 (sub-target) to 1.0 (hyperinflation/supply shock)
	InterestRateIndex float64  `json:"interest_rate_index"`// 0.0 (zero-rate dovish) to 1.0 (extreme hawkish tightening)
	ActiveConflicts   []string `json:"active_conflicts,omitempty"`
	InflationRateYoY  float64  `json:"inflation_rate_yoy,omitempty"`
	BenchmarkRate     float64  `json:"benchmark_rate,omitempty"`
}

// MacroRegimeState encapsulates the computed regime, stress score, and dynamic 3-tier allocations.
type MacroRegimeState struct {
	Score          float64         `json:"score"` // 0.0 to 1.0
	Regime         MacroRegimeType `json:"regime"`
	Description    string          `json:"description"`
	Indicators     MacroIndicators `json:"indicators"`
	TargetTier1Pct float64         `json:"target_tier1_pct"` // Liquid Cash Buffer (15% - 25%)
	TargetCorePct  float64         `json:"target_core_pct"`  // Core Wealth Preservation (Gold/Silver, 35% - 60%)
	TargetAlphaPct float64         `json:"target_alpha_pct"` // Tactical Alpha Trading (15% - 50%)
	LastUpdated    time.Time       `json:"last_updated"`
}

// MacroRegimeEngine manages and evaluates the dynamic macroeconomic stress scoring model.
type MacroRegimeEngine struct {
	mu           sync.RWMutex
	weights      struct{ geo, inf, rate float64 }
	currentState MacroRegimeState
}

// NewMacroRegimeEngine initializes the engine with institutional weights and default real-world indicators.
func NewMacroRegimeEngine() *MacroRegimeEngine {
	e := &MacroRegimeEngine{
		weights: struct{ geo, inf, rate float64 }{
			geo:  0.40, // 40% Geopolitical armed conflict & supply disruption
			inf:  0.35, // 35% Inflation trajectory relative to targets
			rate: 0.25, // 25% Central bank interest rate bias & yield curve
		},
	}

	// Real-world baseline: elevated geopolitical stress, moderate inflation, restrictive rates
	baseline := MacroIndicators{
		GeopoliticalIndex: 0.70, // Active Middle East & Eastern Europe conflicts
		InflationIndex:    0.55, // CPI elevated above central bank 2.0% target
		InterestRateIndex: 0.65, // Restrictive global benchmark interest rates ~5.25%
		ActiveConflicts:   []string{"Eastern Europe Conflict", "Middle East Regional Escalation", "Red Sea Maritime Disruption"},
		InflationRateYoY:  3.1,
		BenchmarkRate:     5.25,
	}

	e.currentState = e.CalculateRegime(baseline)
	return e
}

// CalculateRegime computes the Macro Stress Score S and calculates dynamic 3-tier targets.
// S = w_geo * I_geo + w_inf * I_inf + w_rate * I_rate in [0.0, 1.0]
func (e *MacroRegimeEngine) CalculateRegime(ind MacroIndicators) MacroRegimeState {
	// Clamp indicators between 0.0 and 1.0
	clamp := func(val float64) float64 {
		if val < 0.0 {
			return 0.0
		}
		if val > 1.0 {
			return 1.0
		}
		return val
	}

	geo := clamp(ind.GeopoliticalIndex)
	inf := clamp(ind.InflationIndex)
	rate := clamp(ind.InterestRateIndex)

	score := e.weights.geo*geo + e.weights.inf*inf + e.weights.rate*rate
	score = math.Round(score*1000) / 1000

	var regime MacroRegimeType
	var desc string
	var tier1, core, alpha float64

	switch {
	case score >= 0.65:
		regime = RegimeCrisis
		desc = "High Macroeconomic Stress & Geopolitical Conflict: Flight to Safe-Haven Commodities (Gold/Silver) and Liquidity Preservation"

		// Dynamic interpolation for crisis:
		// Core expands from 55% to 60%
		// Cash expands from 20% to 25%
		// Alpha contracts from 25% down to 15%
		t := (score - 0.65) / 0.35
		if t > 1.0 {
			t = 1.0
		}
		core = 0.55 + 0.05*t
		tier1 = 0.20 + 0.05*t
		alpha = 1.0 - (core + tier1)

	case score < 0.35:
		regime = RegimeDovishExpansion
		desc = "Dovish Economic Expansion & Low Geopolitical Risk: High Risk-Appetite Tactical Alpha Expansion"

		// Dynamic interpolation for expansion:
		// Cash remains baseline 15%
		// Core contracts from 40% down to 35%
		// Alpha expands from 45% up to 50%
		t := score / 0.35 // 0.0 (max expansion) to 1.0 (boundary)
		core = 0.35 + 0.05*t
		tier1 = 0.15
		alpha = 1.0 - (core + tier1)

	default:
		regime = RegimeNormal
		desc = "Balanced Economic Regime: Standard 3-Tier Allocation (15% Cash, 45% Core, 40% Tactical Alpha)"
		tier1 = 0.15
		core = 0.45
		alpha = 0.40
	}

	// Precision rounding to 4 decimal places
	round4 := func(v float64) float64 {
		return math.Round(v*10000) / 10000
	}

	tier1 = round4(tier1)
	core = round4(core)
	alpha = round4(alpha)

	// Ensure exact sum equals 1.0
	diff := round4(1.0 - (tier1 + core + alpha))
	alpha += diff

	return MacroRegimeState{
		Score:          score,
		Regime:         regime,
		Description:    desc,
		Indicators:     ind,
		TargetTier1Pct: tier1,
		TargetCorePct:  core,
		TargetAlphaPct: alpha,
		LastUpdated:    time.Now().UTC(),
	}
}

// UpdateIndicators evaluates new macroeconomic indicators, updates current state, and returns it.
func (e *MacroRegimeEngine) UpdateIndicators(ind MacroIndicators) MacroRegimeState {
	e.mu.Lock()
	defer e.mu.Unlock()

	newState := e.CalculateRegime(ind)
	e.currentState = newState
	return newState
}

// GetCurrentState returns the latest macro regime calculation.
func (e *MacroRegimeEngine) GetCurrentState() MacroRegimeState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentState
}
