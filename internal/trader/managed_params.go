package trader

import (
	"encoding/json"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// BuildManagedEntryQuestions returns the subset of entry questions for parameters
// that are currently in core_managed mode. Override parameters are omitted (FR-301).
func BuildManagedEntryQuestions(cfg *config.Config, state map[string]interface{}) map[string]ai.JevQuestion {
	reg := NewParamRegistry(cfg)
	questions := make(map[string]ai.JevQuestion)

	entryKeys := []string{"min_rr", "leverage", "conviction", "atr_regime", "confluence"}
	for _, k := range entryKeys {
		spec, ok := reg.Get(k)
		if !ok {
			continue
		}
		if spec.Mode() == ModeManaged && spec.Question != nil {
			questions[spec.QuestionKey] = spec.Question(state)
		}
	}
	return questions
}

// BuildManagedNewsDecayQuestion returns the decay question if CLUSTER_DECAY_MODE is managed.
func BuildManagedNewsDecayQuestion(cfg *config.Config, state map[string]interface{}) (ai.JevQuestion, bool) {
	reg := NewParamRegistry(cfg)
	spec, ok := reg.Get("decay")
	if !ok || spec.Mode() != ModeManaged || spec.Question == nil {
		return ai.JevQuestion{}, false
	}
	return spec.Question(state), true
}

// PackageParamRecord serializes the resolved parameters into the 4 JSONB column payloads
// required for futures_trade_signals (parameter_modes, parameter_values, parameter_distributions, parameter_clamps).
func PackageParamRecord(resolved map[string]ResolvedParam) (modesJSON, valuesJSON, distsJSON, clampsJSON []byte, err error) {
	modes := make(map[string]string)
	values := make(map[string]interface{})
	dists := make(map[string]interface{})
	clamps := make(map[string]interface{})

	for k, res := range resolved {
		modes[k] = string(res.Mode)
		if res.Value != nil {
			values[k] = res.Value
		}
		if res.Distribution != nil {
			dists[k] = res.Distribution
		}
		if res.Clamped && res.ClampDetail != nil {
			clamps[k] = res.ClampDetail
		}
	}

	modesBytes, err := json.Marshal(modes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	valuesBytes, err := json.Marshal(values)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var distsBytes []byte
	if len(dists) > 0 {
		distsBytes, err = json.Marshal(dists)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	var clampsBytes []byte
	if len(clamps) > 0 {
		clampsBytes, err = json.Marshal(clamps)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	return modesBytes, valuesBytes, distsBytes, clampsBytes, nil
}
