package trader

import (
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// TestParamRegistry_AllOverride verifies FR-301 / Quickstart V1:
// When all 7 override keys are set in environment, the 6 phase-1 params
// report mode "user_override", their resolved values match configuration,
// and zero questions are built for Jev batch.
func TestParamRegistry_AllOverride(t *testing.T) {
	// Setup environment with all 7 overrides set
	envVars := map[string]string{
		"MIN_RISK_TO_REWARD_RATIO": "3.0",
		"DEFAULT_LEVERAGE":         "5",
		"MAX_RISK_PER_TRADE_PCT":   "0.015",
		"SL_ATR_MULT":              "1.8",
		"TP_ATR_MULT":              "3.6",
		"CLUSTER_DECAY_MODE":       "MACRO_THEMATIC",
		"CONFLUENCE_MIN":           "0.70",
	}
	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}

	reg := NewParamRegistry(cfg)
	if reg == nil {
		t.Fatal("NewParamRegistry returned nil")
	}

	expectedParams := []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"}
	for _, p := range expectedParams {
		spec, ok := reg.Get(p)
		if !ok {
			t.Errorf("param %q missing from registry", p)
			continue
		}
		if spec.Mode() != ModeOverride {
			t.Errorf("param %q mode = %q, want %q", p, spec.Mode(), ModeOverride)
		}
	}

	// Verify no questions are built when all are overridden
	state := map[string]interface{}{"symbol": "BTCUSDT"}
	questions := reg.BuildQuestions(state)
	if len(questions) != 0 {
		t.Errorf("expected 0 questions in all-override mode, got %d: %v", len(questions), questions)
	}

	// Verify resolved values equal configured values
	resolved, err := reg.ResolveAll(nil, "cycle-1")
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}
	if resolved["min_rr"].Value.(float64) != 3.0 {
		t.Errorf("min_rr value = %v, want 3.0", resolved["min_rr"].Value)
	}
	if resolved["leverage"].Value.(int) != 5 {
		t.Errorf("leverage value = %v, want 5", resolved["leverage"].Value)
	}
	if resolved["conviction"].Value.(float64) != 0.015 {
		t.Errorf("conviction value = %v, want 0.015", resolved["conviction"].Value)
	}
	if resolved["decay"].Value.(string) != "MACRO_THEMATIC" {
		t.Errorf("decay value = %v, want MACRO_THEMATIC", resolved["decay"].Value)
	}
	if resolved["confluence"].Value.(float64) != 0.70 {
		t.Errorf("confluence value = %v, want 0.70", resolved["confluence"].Value)
	}
}

// TestParamRegistry_AllUnset verifies FR-302 / Quickstart V2:
// When all override keys are unset/empty, all 6 params report "core_managed",
// and questions are built for Jev batch.
func TestParamRegistry_AllUnset(t *testing.T) {
	// Unset all 7 override keys
	keys := []string{
		"MIN_RISK_TO_REWARD_RATIO",
		"DEFAULT_LEVERAGE",
		"MAX_RISK_PER_TRADE_PCT",
		"SL_ATR_MULT",
		"TP_ATR_MULT",
		"CLUSTER_DECAY_MODE",
		"CONFLUENCE_MIN",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
		t.Setenv(k, "")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}

	reg := NewParamRegistry(cfg)
	if reg == nil {
		t.Fatal("NewParamRegistry returned nil")
	}

	expectedParams := []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"}
	for _, p := range expectedParams {
		spec, ok := reg.Get(p)
		if !ok {
			t.Errorf("param %q missing from registry", p)
			continue
		}
		if spec.Mode() != ModeManaged {
			t.Errorf("param %q mode = %q, want %q", p, spec.Mode(), ModeManaged)
		}
	}

	// Verify questions are built for managed params
	state := map[string]interface{}{"symbol": "BTCUSDT"}
	questions := reg.BuildQuestions(state)
	// Entry batch params (min_rr_accept, leverage, conviction, atr_regime, confluence)
	// and news decay question if built via reg
	if len(questions) < 5 {
		t.Errorf("expected at least 5 entry questions built in all-unset mode, got %d", len(questions))
	}

	// Mock answers for resolving managed params
	mockScore := 2.8
	mockConviction := 0.75
	mockConf := 0.82
	answers := map[string]ai.JevAnswer{
		"min_rr_accept": {Type: "score", Score: &mockScore},
		"leverage":      {Type: "choice", Choice: "8x"},
		"conviction":    {Type: "score", Score: &mockConviction},
		"atr_regime":    {Type: "choice", Choice: "NORMAL"},
		"confluence":    {Type: "score", Score: &mockConf},
		"decay":         {Type: "choice", Choice: "FAST_BREAKING"},
	}

	resolved, err := reg.ResolveAll(answers, "cycle-2")
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	for _, p := range expectedParams {
		res, ok := resolved[p]
		if !ok {
			t.Errorf("resolved missing param %q", p)
			continue
		}
		if res.Mode != ModeManaged {
			t.Errorf("param %q resolved mode = %q, want %q", p, res.Mode, ModeManaged)
		}
	}
}

// TestParamRegistry_MustStayGuard verifies FR-305 / Quickstart V3:
// Parameters classified must-stay-code in the offload audit MUST NOT appear in registry.
func TestParamRegistry_MustStayGuard(t *testing.T) {
	reg := NewParamRegistry(&config.Config{})
	if reg == nil {
		t.Fatal("NewParamRegistry returned nil")
	}

	forbidden := []string{
		"MAX_DRAWDOWN_LIMIT_PCT",
		"MAX_CONCURRENT_SIGNALS",
		"MAKER_FEE_RATE",
		"TAKER_FEE_RATE",
		"MAX_SLIPPAGE_PCT",
		"MAX_TRADE_MARGIN_PCT",
		"max_drawdown_limit_pct",
		"max_concurrent_signals",
		"maker_fee_rate",
		"taker_fee_rate",
		"max_slippage_pct",
		"max_trade_margin_pct",
		"drawdown",
		"liquidation_buffer",
	}

	for _, f := range forbidden {
		if _, exists := reg.Get(f); exists {
			t.Errorf("CRITICAL VIOLATION: must-stay parameter %q found in ParamRegistry (FR-305)", f)
		}
	}
}

// TestParamRegistry_RuntimeFlip_OverrideToManaged verifies FR-301 / FR-308 / US2 / Quickstart V6:
// set MIN_RISK_TO_REWARD_RATIO=2.5 → question absent from batch, record mode=user_override, value=2.5 exactly;
// runtime clear (config mutated) → next Resolve returns core_managed.
func TestParamRegistry_RuntimeFlip_OverrideToManaged(t *testing.T) {
	val := 2.5
	cfg := &config.Config{
		OverrideMinRiskRewardRatio: &val,
	}

	reg := NewParamRegistry(cfg)

	// Step 1: Override is active
	spec, ok := reg.Get("min_rr")
	if !ok {
		t.Fatal("min_rr not found")
	}
	if spec.Mode() != ModeOverride {
		t.Fatalf("expected mode user_override, got %q", spec.Mode())
	}

	questions := reg.BuildQuestions(nil)
	if _, present := questions["min_rr_accept"]; present {
		t.Errorf("FR-301: question should be absent when override is set")
	}

	res, err := reg.Resolve("min_rr", nil, "cycle-override")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if res.Mode != ModeOverride {
		t.Errorf("expected mode user_override, got %q", res.Mode)
	}
	if res.Value.(float64) != 2.5 {
		t.Errorf("expected value 2.5, got %v", res.Value)
	}

	// Step 2: Runtime clear (config mutated in place without restart - FR-308)
	cfg.OverrideMinRiskRewardRatio = nil

	if spec.Mode() != ModeManaged {
		t.Errorf("FR-308: expected mode to immediately reflect core_managed after clear, got %q", spec.Mode())
	}

	questionsAfter := reg.BuildQuestions(nil)
	if _, present := questionsAfter["min_rr_accept"]; !present {
		t.Errorf("FR-308: question should be present after clear")
	}

	// Resolving without answer must now fail loud (managed mode requires answer)
	_, err = reg.Resolve("min_rr", nil, "cycle-cleared")
	if err == nil {
		t.Errorf("expected error when resolving managed param without answer")
	}

	// Resolving with answer returns core_managed
	mockScore := 3.2
	resManaged, err := reg.Resolve("min_rr", &ai.JevAnswer{Type: "score", Score: &mockScore}, "cycle-cleared")
	if err != nil {
		t.Fatalf("Resolve with answer failed: %v", err)
	}
	if resManaged.Mode != ModeManaged {
		t.Errorf("expected mode core_managed, got %q", resManaged.Mode)
	}
	if resManaged.Value.(float64) != 3.2 {
		t.Errorf("expected value 3.2, got %v", resManaged.Value)
	}
}

// TestConfluenceQuestion_AnchoredToLiveScale: the acceptance threshold must
// ride the ACTUAL indicator-confluence scale (0.25-0.50 in live markets) —
// label-based 0.6-0.75 answers vetoed 100% of BUY decisions on 2026-09-29.
func TestConfluenceQuestion_AnchoredToLiveScale(t *testing.T) {
	reg := NewParamRegistry(&config.Config{})
	spec, ok := reg.Get("confluence")
	if !ok {
		t.Fatalf("confluence spec missing")
	}
	q := spec.Question(map[string]interface{}{"confluence": 0.34})
	instr, ok := q.Instructions.(map[string]interface{})
	if !ok {
		t.Fatalf("instructions type = %T", q.Instructions)
	}
	if cur, ok := instr["current_confluence"].(float64); !ok || cur != 0.34 {
		t.Errorf("question must carry the live confluence score, got %v", instr["current_confluence"])
	}
	if _, ok := instr["scale_note"]; !ok {
		t.Errorf("question must explain the 0-1 scale (pre-computed inputs rule)")
	}
}
