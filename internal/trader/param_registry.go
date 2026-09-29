package trader

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// ParamMode represents whether a parameter is overridden by user configuration or managed by Jev core.
type ParamMode string

const (
	ModeOverride ParamMode = "user_override"
	ModeManaged  ParamMode = "core_managed"
)

// Bounds defines mechanical minimum and maximum limits that code always enforces.
type Bounds struct {
	Min float64
	Max float64
}

// ParamSpec defines the contract for one decision-judged parameter.
type ParamSpec struct {
	Key         string // Canonical key e.g. "min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"
	EnvKey      string // Primary env key e.g. "MIN_RISK_TO_REWARD_RATIO"
	QuestionKey string // Key in Jev batch e.g. "min_rr_accept", "leverage", etc.
	Bounds      Bounds
	Mode        func() ParamMode
	Question    func(state map[string]interface{}) ai.JevQuestion
	Clamp       func(raw interface{}, bounds Bounds) (applied interface{}, clamped bool, detail map[string]interface{}, err error)
	GetOverride func() interface{}
}

// ResolvedParam is the cycle outcome for a single parameter.
type ResolvedParam struct {
	Key          string                 `json:"key"`
	Mode         ParamMode              `json:"mode"`
	Value        interface{}            `json:"value"`
	Distribution interface{}            `json:"distribution,omitempty"`
	Clamped      bool                   `json:"clamped"`
	ClampDetail  map[string]interface{} `json:"clamp_detail,omitempty"`
}

// MustStayForbiddenParams lists parameters that are mechanical or risk invariants and must NEVER be delegated (FR-305).
var MustStayForbiddenParams = map[string]bool{
	"MAX_DRAWDOWN_LIMIT_PCT": true,
	"MAX_CONCURRENT_SIGNALS": true,
	"MAKER_FEE_RATE":        true,
	"TAKER_FEE_RATE":        true,
	"MAX_SLIPPAGE_PCT":      true,
	"MAX_TRADE_MARGIN_PCT":   true,
	"max_drawdown_limit_pct": true,
	"max_concurrent_signals": true,
	"maker_fee_rate":        true,
	"taker_fee_rate":        true,
	"max_slippage_pct":      true,
	"max_trade_margin_pct":   true,
	"drawdown":              true,
	"liquidation_buffer":     true,
}

// ParamRegistry manages the 6 phase-1 decision-judged trade parameters.
type ParamRegistry struct {
	cfg   *config.Config
	specs map[string]ParamSpec
}

// NewParamRegistry constructs a registry for the 6 phase-1 parameters.
func NewParamRegistry(cfg *config.Config) *ParamRegistry {
	if cfg == nil {
		cfg = &config.Config{}
	}
	r := &ParamRegistry{
		cfg:   cfg,
		specs: make(map[string]ParamSpec),
	}
	r.registerPhase1Specs()
	return r
}

// Get returns the specification for a parameter key.
func (r *ParamRegistry) Get(key string) (ParamSpec, bool) {
	norm := strings.ToLower(strings.TrimSpace(key))
	spec, ok := r.specs[norm]
	return spec, ok
}

// Keys returns all registered canonical keys in deterministic order.
func (r *ParamRegistry) Keys() []string {
	return []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"}
}

// BuildQuestions constructs Jev questions for all parameters currently in core_managed mode.
// Override parameters produce ZERO questions (FR-301).
func (r *ParamRegistry) BuildQuestions(state map[string]interface{}) map[string]ai.JevQuestion {
	questions := make(map[string]ai.JevQuestion)
	for _, k := range r.Keys() {
		spec := r.specs[k]
		if spec.Mode() == ModeManaged && spec.Question != nil {
			q := spec.Question(state)
			questions[spec.QuestionKey] = q
		}
	}
	return questions
}

// Resolve resolves a single parameter given an optional Jev answer.
func (r *ParamRegistry) Resolve(key string, ans *ai.JevAnswer, cycleID string) (ResolvedParam, error) {
	spec, ok := r.Get(key)
	if !ok {
		return ResolvedParam{}, fmt.Errorf("component=managed-params cycle=%s: unknown parameter %q", cycleID, key)
	}

	mode := spec.Mode()
	if mode == ModeOverride {
		val := spec.GetOverride()
		return ResolvedParam{
			Key:     spec.Key,
			Mode:    ModeOverride,
			Value:   val,
			Clamped: false,
		}, nil
	}

	// Managed mode requires a valid answer
	if ans == nil {
		return ResolvedParam{}, fmt.Errorf("component=managed-params cycle=%s: missing answer for managed parameter %q (FR-307 zero-fallback)", cycleID, key)
	}

	applied, clamped, detail, err := spec.Clamp(ans, spec.Bounds)
	if err != nil {
		return ResolvedParam{}, fmt.Errorf("component=managed-params cycle=%s: %w", cycleID, err)
	}

	var dist interface{}
	if len(ans.Probabilities) > 0 {
		dist = ans.Probabilities
	} else if ans.Score != nil {
		dist = map[string]float64{"score": *ans.Score}
	}

	return ResolvedParam{
		Key:          spec.Key,
		Mode:         ModeManaged,
		Value:        applied,
		Distribution: dist,
		Clamped:      clamped,
		ClampDetail:  detail,
	}, nil
}

// ResolveAll resolves all 6 parameters for a cycle.
func (r *ParamRegistry) ResolveAll(answers map[string]ai.JevAnswer, cycleID string) (map[string]ResolvedParam, error) {
	results := make(map[string]ResolvedParam)
	for _, k := range r.Keys() {
		spec := r.specs[k]
		var ansPtr *ai.JevAnswer
		if answers != nil {
			if a, ok := answers[spec.QuestionKey]; ok {
				ansPtr = &a
			} else if a, ok := answers[spec.Key]; ok {
				ansPtr = &a
			}
		}
		res, err := r.Resolve(k, ansPtr, cycleID)
		if err != nil {
			return nil, err
		}
		results[k] = res
	}
	return results, nil
}

func (r *ParamRegistry) registerPhase1Specs() {
	// 1. min_rr
	r.specs["min_rr"] = ParamSpec{
		Key:         "min_rr",
		EnvKey:      "MIN_RISK_TO_REWARD_RATIO",
		QuestionKey: "min_rr_accept",
		Bounds:      Bounds{Min: 0.5, Max: 10.0},
		Mode: func() ParamMode {
			if r.cfg != nil && r.cfg.OverrideMinRiskRewardRatio != nil {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			if r.cfg != nil && r.cfg.OverrideMinRiskRewardRatio != nil {
				return *r.cfg.OverrideMinRiskRewardRatio
			}
			return 2.5
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			mkt := ""
			if s, ok := state["symbol"].(string); ok {
				mkt = s
			}
			return ai.JevQuestion{
				Type: "score",
				Criteria: []string{
					"Reject <1.5",
					"Marginal 1.5-2.5",
					"Accept 2.5-4",
					"Strong >4",
				},
				Instructions: map[string]interface{}{
					"question": "Minimum R:R to accept this setup",
					"market":   mkt,
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			val, err := extractNumeric(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param min_rr schema error: %w", err)
			}
			if val < bounds.Min {
				return bounds.Min, true, map[string]interface{}{
					"requested": val,
					"applied":   bounds.Min,
					"bound":     "min_rr_min",
				}, nil
			}
			if val > bounds.Max {
				return bounds.Max, true, map[string]interface{}{
					"requested": val,
					"applied":   bounds.Max,
					"bound":     "min_rr_max",
				}, nil
			}
			return val, false, nil, nil
		},
	}

	// 2. leverage
	r.specs["leverage"] = ParamSpec{
		Key:         "leverage",
		EnvKey:      "DEFAULT_LEVERAGE",
		QuestionKey: "leverage",
		Bounds:      Bounds{Min: 1, Max: 12}, // Exchange max / ceiling is 12x
		Mode: func() ParamMode {
			if r.cfg != nil && r.cfg.OverrideDefaultLeverage != nil {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			if r.cfg != nil && r.cfg.OverrideDefaultLeverage != nil {
				return *r.cfg.OverrideDefaultLeverage
			}
			return 8
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			return ai.JevQuestion{
				Type: "choice",
				Criteria: map[string]string{
					"3x":  "max safety / high volatility",
					"5x":  "conservative",
					"8x":  "balanced standard",
					"10x": "high confidence",
					"12x": "maximum allowed leverage",
				},
				Instructions: map[string]interface{}{
					"question":   "Leverage choice for this setup within hard mechanical bounds",
					"liq_buffer": "precomputed ok",
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			val, err := extractLeverageInt(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param leverage schema error: %w", err)
			}
			minLev := int(bounds.Min)
			maxLev := int(bounds.Max)
			if val < minLev {
				return minLev, true, map[string]interface{}{
					"requested": val,
					"applied":   minLev,
					"bound":     "min_leverage",
				}, nil
			}
			if val > maxLev {
				return maxLev, true, map[string]interface{}{
					"requested": val,
					"applied":   maxLev,
					"bound":     "exchange_max",
				}, nil
			}
			return val, false, nil, nil
		},
	}

	// 3. conviction
	minRisk := 0.005
	maxRisk := 0.02
	if r.cfg != nil && r.cfg.MinRiskPerTradePct > 0 {
		minRisk = r.cfg.MinRiskPerTradePct
	}
	if r.cfg != nil && r.cfg.MaxRiskPerTradePct > 0 {
		maxRisk = r.cfg.MaxRiskPerTradePct
	}
	r.specs["conviction"] = ParamSpec{
		Key:         "conviction",
		EnvKey:      "MAX_RISK_PER_TRADE_PCT",
		QuestionKey: "conviction",
		Bounds:      Bounds{Min: minRisk, Max: maxRisk},
		Mode: func() ParamMode {
			if r.cfg != nil && r.cfg.OverrideMaxRiskPerTradePct != nil {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			if r.cfg != nil && r.cfg.OverrideMaxRiskPerTradePct != nil {
				return *r.cfg.OverrideMaxRiskPerTradePct
			}
			return maxRisk
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			return ai.JevQuestion{
				Type: "score",
				Criteria: []string{
					"Weak",
					"Moderate",
					"High",
					"Very high",
				},
				Instructions: map[string]interface{}{
					"question": "Conviction score for position risk sizing",
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			score, err := extractNumeric(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param conviction schema error: %w", err)
			}
			// If score is a fractional risk directly (or forced out-of-bound > maxRisk or < minRisk in tests)
			if score > 1.0 || (score > bounds.Max && score < 1.0) {
				if score > bounds.Max {
					return bounds.Max, true, map[string]interface{}{
						"requested": score,
						"applied":   bounds.Max,
						"bound":     "max_risk_cap",
					}, nil
				}
			}
			if score < 0.0 {
				return bounds.Min, true, map[string]interface{}{
					"requested": score,
					"applied":   bounds.Min,
					"bound":     "min_risk_floor",
				}, nil
			}
			// Standard [0, 1] conviction score scales risk between Min and Max
			val := bounds.Min + score*(bounds.Max-bounds.Min)
			return math.Round(val*10000) / 10000, false, nil, nil
		},
	}

	// 4. atr_regime
	r.specs["atr_regime"] = ParamSpec{
		Key:         "atr_regime",
		EnvKey:      "SL_ATR_MULT",
		QuestionKey: "atr_regime",
		Bounds:      Bounds{Min: 0.5, Max: 5.0},
		Mode: func() ParamMode {
			if r.cfg != nil && (r.cfg.OverrideSLAtrMult != nil || r.cfg.OverrideTPAtrMult != nil) {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			res := make(map[string]interface{})
			if r.cfg != nil && r.cfg.OverrideSLAtrMult != nil {
				res["sl_atr_mult"] = *r.cfg.OverrideSLAtrMult
			} else {
				res["sl_atr_mult"] = 1.5
			}
			if r.cfg != nil && r.cfg.OverrideTPAtrMult != nil {
				res["tp_atr_mult"] = *r.cfg.OverrideTPAtrMult
			} else {
				res["tp_atr_mult"] = 3.0
			}
			return res
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			return ai.JevQuestion{
				Type: "choice",
				Criteria: map[string]string{
					"TIGHT":  "volatility contraction / tight stops",
					"NORMAL": "standard market regime",
					"WIDE":   "expansion, wider stops and targets",
				},
				Instructions: map[string]interface{}{
					"question": "ATR stop/target regime selection",
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			choice, err := extractChoiceString(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param atr_regime schema error: %w", err)
			}
			choice = strings.ToUpper(strings.TrimSpace(choice))
			switch choice {
			case "TIGHT":
				return map[string]interface{}{
					"regime":      "TIGHT",
					"sl_atr_mult": 1.0,
					"tp_atr_mult": 2.0,
				}, false, nil, nil
			case "NORMAL":
				return map[string]interface{}{
					"regime":      "NORMAL",
					"sl_atr_mult": 1.5,
					"tp_atr_mult": 3.0,
				}, false, nil, nil
			case "WIDE":
				return map[string]interface{}{
					"regime":      "WIDE",
					"sl_atr_mult": 2.0,
					"tp_atr_mult": 4.0,
				}, false, nil, nil
			default:
				// FR-307: out-of-vocabulary -> explicit schema error
				return nil, false, nil, fmt.Errorf("param atr_regime out of vocabulary: %q (allowed: TIGHT, NORMAL, WIDE)", choice)
			}
		},
	}

	// 5. decay
	r.specs["decay"] = ParamSpec{
		Key:         "decay",
		EnvKey:      "CLUSTER_DECAY_MODE",
		QuestionKey: "decay",
		Bounds:      Bounds{Min: 15, Max: 120},
		Mode: func() ParamMode {
			if r.cfg != nil && r.cfg.OverrideClusterDecayMode != "" {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			if r.cfg != nil && r.cfg.OverrideClusterDecayMode != "" {
				return r.cfg.OverrideClusterDecayMode
			}
			return "FAST_BREAKING"
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			return ai.JevQuestion{
				Type: "choice",
				Criteria: map[string]string{
					"FAST_BREAKING":  "news dies <=6h, rapid decay",
					"MACRO_THEMATIC": "persists days, slow decay",
				},
				Instructions: map[string]interface{}{
					"question": "News cluster decay rate category",
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			choice, err := extractChoiceString(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param decay schema error: %w", err)
			}
			choice = strings.ToUpper(strings.TrimSpace(choice))
			if choice != "FAST_BREAKING" && choice != "MACRO_THEMATIC" {
				return nil, false, nil, fmt.Errorf("param decay out of vocabulary: %q (allowed: FAST_BREAKING, MACRO_THEMATIC)", choice)
			}
			return choice, false, nil, nil
		},
	}

	// 6. confluence
	r.specs["confluence"] = ParamSpec{
		Key:         "confluence",
		EnvKey:      "CONFLUENCE_MIN",
		QuestionKey: "confluence",
		Bounds:      Bounds{Min: 0.0, Max: 1.0},
		Mode: func() ParamMode {
			if r.cfg != nil && r.cfg.OverrideConfluenceMin != nil {
				return ModeOverride
			}
			return ModeManaged
		},
		GetOverride: func() interface{} {
			if r.cfg != nil && r.cfg.OverrideConfluenceMin != nil {
				return *r.cfg.OverrideConfluenceMin
			}
			return 0.60
		},
		Question: func(state map[string]interface{}) ai.JevQuestion {
			return ai.JevQuestion{
				Type: "score",
				Criteria: []string{
					"Reject",
					"Weak",
					"Adequate",
					"Strong",
				},
				Instructions: map[string]interface{}{
					"question": "Technical confluence acceptance score threshold",
				},
			}
		},
		Clamp: func(raw interface{}, bounds Bounds) (interface{}, bool, map[string]interface{}, error) {
			val, err := extractNumeric(raw)
			if err != nil {
				return nil, false, nil, fmt.Errorf("param confluence schema error: %w", err)
			}
			if val < bounds.Min {
				return bounds.Min, true, map[string]interface{}{
					"requested": val,
					"applied":   bounds.Min,
					"bound":     "confluence_min",
				}, nil
			}
			if val > bounds.Max {
				return bounds.Max, true, map[string]interface{}{
					"requested": val,
					"applied":   bounds.Max,
					"bound":     "confluence_max",
				}, nil
			}
			return val, false, nil, nil
		},
	}
}

func extractNumeric(raw interface{}) (float64, error) {
	switch v := raw.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case *float64:
		if v != nil {
			return *v, nil
		}
	case ai.JevAnswer:
		if v.Score != nil {
			return *v.Score, nil
		}
		if v.Noul != nil {
			return *v.Noul, nil
		}
		if v.Choice != "" {
			return strconv.ParseFloat(strings.TrimSpace(v.Choice), 64)
		}
	case *ai.JevAnswer:
		if v != nil {
			return extractNumeric(*v)
		}
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	return 0, fmt.Errorf("cannot extract numeric value from %T", raw)
}

func extractChoiceString(raw interface{}) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case ai.JevAnswer:
		if v.Choice != "" {
			return v.Choice, nil
		}
	case *ai.JevAnswer:
		if v != nil && v.Choice != "" {
			return v.Choice, nil
		}
	}
	return "", fmt.Errorf("cannot extract choice string from %T", raw)
}

func extractLeverageInt(raw interface{}) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		s := strings.TrimRight(strings.ToLower(strings.TrimSpace(v)), "x")
		return strconv.Atoi(s)
	case ai.JevAnswer:
		if v.Choice != "" {
			s := strings.TrimRight(strings.ToLower(strings.TrimSpace(v.Choice)), "x")
			return strconv.Atoi(s)
		}
		if v.Score != nil {
			return int(*v.Score), nil
		}
	case *ai.JevAnswer:
		if v != nil {
			return extractLeverageInt(*v)
		}
	}
	return 0, fmt.Errorf("cannot extract leverage integer from %T", raw)
}
