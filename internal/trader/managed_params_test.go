package trader

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
)

// TestManagedParams_QuestionsOnlyForManaged verifies FR-301:
// When some params are overridden and others are unset, Jev entry batch
// questions are built ONLY for managed params; overridden params are absent.
func TestManagedParams_QuestionsOnlyForManaged(t *testing.T) {
	// Set override for min_rr and leverage, leave others unset
	t.Setenv("MIN_RISK_TO_REWARD_RATIO", "3.0")
	t.Setenv("DEFAULT_LEVERAGE", "10")
	_ = os.Unsetenv("MAX_RISK_PER_TRADE_PCT")
	t.Setenv("MAX_RISK_PER_TRADE_PCT", "")
	_ = os.Unsetenv("SL_ATR_MULT")
	t.Setenv("SL_ATR_MULT", "")
	_ = os.Unsetenv("TP_ATR_MULT")
	t.Setenv("TP_ATR_MULT", "")
	_ = os.Unsetenv("CONFLUENCE_MIN")
	t.Setenv("CONFLUENCE_MIN", "")
	_ = os.Unsetenv("CLUSTER_DECAY_MODE")
	t.Setenv("CLUSTER_DECAY_MODE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	state := map[string]interface{}{
		"symbol":  "ETHUSDT",
		"atr_pct": 2.1,
		"regime":  "EXPANSION",
	}

	questions := BuildManagedEntryQuestions(cfg, state)

	// min_rr_accept and leverage MUST NOT be in questions (user_override)
	if _, ok := questions["min_rr_accept"]; ok {
		t.Errorf("FR-301 violation: min_rr_accept present in batch questions despite user override")
	}
	if _, ok := questions["leverage"]; ok {
		t.Errorf("FR-301 violation: leverage present in batch questions despite user override")
	}

	// conviction, atr_regime, confluence MUST be in questions (core_managed)
	if _, ok := questions["conviction"]; !ok {
		t.Errorf("conviction question missing from batch for managed param")
	}
	if _, ok := questions["atr_regime"]; !ok {
		t.Errorf("atr_regime question missing from batch for managed param")
	}
	if _, ok := questions["confluence"]; !ok {
		t.Errorf("confluence question missing from batch for managed param")
	}
}

// TestManagedParams_ClampsEnforced_LeverageDrill verifies FR-304 & Quickstart V8:
// Mocked answer leverage 25x is clamped to 12x (exchange ceiling) and recorded in parameter_clamps.
func TestManagedParams_ClampsEnforced_LeverageDrill(t *testing.T) {
	// Unset leverage override so it is core_managed
	_ = os.Unsetenv("DEFAULT_LEVERAGE")
	t.Setenv("DEFAULT_LEVERAGE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	reg := NewParamRegistry(cfg)
	ans := ai.JevAnswer{
		Type:   "choice",
		Choice: "25x",
	}

	res, err := reg.Resolve("leverage", &ans, "drill-cycle")
	if err != nil {
		t.Fatalf("Resolve leverage failed: %v", err)
	}

	if res.Value.(int) != 12 {
		t.Errorf("leverage applied = %v, want 12 (exchange max clamp)", res.Value)
	}
	if !res.Clamped {
		t.Errorf("expected res.Clamped = true for 25x")
	}
	if res.ClampDetail == nil || res.ClampDetail["bound"] != "exchange_max" {
		t.Errorf("expected clamp detail with bound=exchange_max, got: %v", res.ClampDetail)
	}

	// Check helper packaging for signal DB record
	resolvedMap := map[string]ResolvedParam{"leverage": res}
	modes, vals, dists, clamps, err := PackageParamRecord(resolvedMap)
	if err != nil {
		t.Fatalf("PackageParamRecord failed: %v", err)
	}

	var clampObj map[string]interface{}
	if err := json.Unmarshal(clamps, &clampObj); err != nil {
		t.Fatalf("unmarshal clamps JSON failed: %v", err)
	}
	if _, ok := clampObj["leverage"]; !ok {
		t.Errorf("parameter_clamps missing leverage record: %s", string(clamps))
	}
	_ = modes
	_ = vals
	_ = dists
}

// TestManagedParams_MissingManagedAnswer_FailsLoud verifies FR-307 / SC-301 / Quickstart V4:
// A missing answer for a managed parameter MUST return an explicit error and NEVER substitute a constant.
func TestManagedParams_MissingManagedAnswer_FailsLoud(t *testing.T) {
	// All unset
	_ = os.Unsetenv("MIN_RISK_TO_REWARD_RATIO")
	t.Setenv("MIN_RISK_TO_REWARD_RATIO", "")
	_ = os.Unsetenv("DEFAULT_LEVERAGE")
	t.Setenv("DEFAULT_LEVERAGE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	reg := NewParamRegistry(cfg)

	// Batch answers omit "min_rr_accept"
	answers := map[string]ai.JevAnswer{
		"leverage": {Type: "choice", Choice: "8x"},
	}

	_, err = reg.ResolveAll(answers, "fail-loud-cycle")
	if err == nil {
		t.Fatal("expected explicit error on missing managed answer, got nil (zero-fallback violation)")
	}

	if !strings.Contains(err.Error(), "component=managed-params") || !strings.Contains(err.Error(), "min_rr") {
		t.Errorf("expected error mentioning component=managed-params and min_rr, got: %v", err)
	}
}

// TestManagedParams_AllSixModesAlwaysPresent verifies Data Model validation:
// parameter_modes must cover exactly the 6 phase-1 keys in every persisted record.
func TestManagedParams_AllSixModesAlwaysPresent(t *testing.T) {
	cfg, _ := config.Load()
	reg := NewParamRegistry(cfg)

	score := 2.5
	conf := 0.7
	conv := 0.8
	answers := map[string]ai.JevAnswer{
		"min_rr_accept": {Type: "score", Score: &score},
		"leverage":      {Type: "choice", Choice: "8x"},
		"conviction":    {Type: "score", Score: &conv},
		"atr_regime":    {Type: "choice", Choice: "NORMAL"},
		"confluence":    {Type: "score", Score: &conf},
		"decay":         {Type: "choice", Choice: "FAST_BREAKING"},
	}

	resolved, err := reg.ResolveAll(answers, "six-keys-cycle")
	if err != nil {
		t.Fatalf("ResolveAll failed: %v", err)
	}

	modesJSON, valsJSON, _, _, err := PackageParamRecord(resolved)
	if err != nil {
		t.Fatalf("PackageParamRecord failed: %v", err)
	}

	var modes map[string]string
	if err := json.Unmarshal(modesJSON, &modes); err != nil {
		t.Fatalf("unmarshal modes: %v", err)
	}
	var vals map[string]interface{}
	if err := json.Unmarshal(valsJSON, &vals); err != nil {
		t.Fatalf("unmarshal vals: %v", err)
	}

	requiredKeys := []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"}
	for _, k := range requiredKeys {
		if _, ok := modes[k]; !ok {
			t.Errorf("required key %q missing from parameter_modes JSON: %s", k, string(modesJSON))
		}
		if _, ok := vals[k]; !ok {
			t.Errorf("required key %q missing from parameter_values JSON: %s", k, string(valsJSON))
		}
	}
}

// TestClampDrillIntegration verifies T018 / SC-304 / Quickstart V8:
// Forced out-of-bound core answers across all clamped parameters:
// every value is clamped within mechanical bounds and clamp records are present.
func TestClampDrillIntegration(t *testing.T) {
	cfg := &config.Config{
		MinRiskPerTradePct: 0.005,
		MaxRiskPerTradePct: 0.02,
	}
	reg := NewParamRegistry(cfg)

	// 1. Min R:R low clamp: raw semantic answer -0.5 (outside level-index
	// range) < 0.5 -> clamp to 0.5 (in-range scores are level indices per
	// TypeSafe docs and convert via EV, never directly clamped)
	lowRR := -0.5
	resLowRR, err := reg.Resolve("min_rr", &ai.JevAnswer{Type: "score", Score: &lowRR}, "drill-1")
	if err != nil {
		t.Fatalf("resolve min_rr low failed: %v", err)
	}
	if !resLowRR.Clamped || resLowRR.Value.(float64) != 0.5 || resLowRR.ClampDetail["bound"] != "min_rr_min" {
		t.Errorf("expected min_rr clamped to 0.5 with bound=min_rr_min, got: %+v", resLowRR)
	}

	// 2. Min R:R high clamp: answer 15.0 > 10.0 -> clamp to 10.0
	highRR := 15.0
	resHighRR, err := reg.Resolve("min_rr", &ai.JevAnswer{Type: "score", Score: &highRR}, "drill-2")
	if err != nil {
		t.Fatalf("resolve min_rr high failed: %v", err)
	}
	if !resHighRR.Clamped || resHighRR.Value.(float64) != 10.0 || resHighRR.ClampDetail["bound"] != "min_rr_max" {
		t.Errorf("expected min_rr clamped to 10.0 with bound=min_rr_max, got: %+v", resHighRR)
	}

	// 3. Leverage high clamp: answer 25x > 12 -> clamp to 12
	resLevHigh, err := reg.Resolve("leverage", &ai.JevAnswer{Type: "choice", Choice: "25x"}, "drill-3")
	if err != nil {
		t.Fatalf("resolve leverage high failed: %v", err)
	}
	if !resLevHigh.Clamped || resLevHigh.Value.(int) != 12 || resLevHigh.ClampDetail["bound"] != "exchange_max" {
		t.Errorf("expected leverage clamped to 12 with bound=exchange_max, got: %+v", resLevHigh)
	}

	// 4. Leverage low clamp: answer 0x < 1 -> clamp to 1
	resLevLow, err := reg.Resolve("leverage", &ai.JevAnswer{Type: "choice", Choice: "0x"}, "drill-4")
	if err != nil {
		t.Fatalf("resolve leverage low failed: %v", err)
	}
	if !resLevLow.Clamped || resLevLow.Value.(int) != 1 || resLevLow.ClampDetail["bound"] != "min_leverage" {
		t.Errorf("expected leverage clamped to 1 with bound=min_leverage, got: %+v", resLevLow)
	}

	// 5. Conviction high clamp: answer 5.0 > 0.02 -> clamp to 0.02
	highConv := 5.0
	resConvHigh, err := reg.Resolve("conviction", &ai.JevAnswer{Type: "score", Score: &highConv}, "drill-5")
	if err != nil {
		t.Fatalf("resolve conviction high failed: %v", err)
	}
	if !resConvHigh.Clamped || resConvHigh.Value.(float64) != 0.02 || resConvHigh.ClampDetail["bound"] != "max_risk_cap" {
		t.Errorf("expected conviction clamped to 0.02 with bound=max_risk_cap, got: %+v", resConvHigh)
	}

	// 6. Conviction low clamp: answer -0.5 < 0 -> clamp to 0.005
	lowConv := -0.5
	resConvLow, err := reg.Resolve("conviction", &ai.JevAnswer{Type: "score", Score: &lowConv}, "drill-6")
	if err != nil {
		t.Fatalf("resolve conviction low failed: %v", err)
	}
	if !resConvLow.Clamped || resConvLow.Value.(float64) != 0.005 || resConvLow.ClampDetail["bound"] != "min_risk_floor" {
		t.Errorf("expected conviction clamped to 0.005 with bound=min_risk_floor, got: %+v", resConvLow)
	}

	// 7. Confluence low clamp: answer -0.2 < 0.0 -> clamp to 0.0
	lowConf := -0.2
	resConfLow, err := reg.Resolve("confluence", &ai.JevAnswer{Type: "score", Score: &lowConf}, "drill-7")
	if err != nil {
		t.Fatalf("resolve confluence low failed: %v", err)
	}
	if !resConfLow.Clamped || resConfLow.Value.(float64) != 0.0 || resConfLow.ClampDetail["bound"] != "confluence_min" {
		t.Errorf("expected confluence clamped to 0.0 with bound=confluence_min, got: %+v", resConfLow)
	}

	// 8. Confluence high clamp: answer 1.5 > 1.0 -> clamp to 1.0
	highConf := 1.5
	resConfHigh, err := reg.Resolve("confluence", &ai.JevAnswer{Type: "score", Score: &highConf}, "drill-8")
	if err != nil {
		t.Fatalf("resolve confluence high failed: %v", err)
	}
	if !resConfHigh.Clamped || resConfHigh.Value.(float64) != 1.0 || resConfHigh.ClampDetail["bound"] != "confluence_max" {
		t.Errorf("expected confluence clamped to 1.0 with bound=confluence_max, got: %+v", resConfHigh)
	}

	// Verify all clamp records serialize cleanly into parameter_clamps JSONB
	resolvedMap := map[string]ResolvedParam{
		"min_rr":     resLowRR,
		"leverage":   resLevHigh,
		"conviction": resConvHigh,
		"confluence": resConfLow,
	}
	_, _, _, clampsJSON, err := PackageParamRecord(resolvedMap)
	if err != nil {
		t.Fatalf("package params failed: %v", err)
	}
	var clamps map[string]interface{}
	if err := json.Unmarshal(clampsJSON, &clamps); err != nil {
		t.Fatalf("unmarshal clamps failed: %v", err)
	}
	if len(clamps) != 4 {
		t.Errorf("expected 4 clamped entries in JSON, got %d: %s", len(clamps), string(clampsJSON))
	}
}

// TestChaos_CoreFailureMidBatch verifies T019 / FR-307 / V4 negative:
// Core failure mid-batch with managed param yields an explicit error,
// NO legacy constants leak into managed fields, while override params remain unaffected.
func TestChaos_CoreFailureMidBatch(t *testing.T) {
	// Set override for min_rr, leave leverage and conviction managed
	rrOverride := 3.5
	cfg := &config.Config{
		OverrideMinRiskRewardRatio: &rrOverride,
	}
	reg := NewParamRegistry(cfg)

	// Step 1: Override parameter resolves successfully even with nil answers
	resRR, err := reg.Resolve("min_rr", nil, "chaos-cycle")
	if err != nil {
		t.Fatalf("override param failed: %v", err)
	}
	if resRR.Mode != ModeOverride || resRR.Value.(float64) != 3.5 {
		t.Errorf("expected override min_rr=3.5, got %+v", resRR)
	}

	// Step 2: Core failure mid-batch (e.g. missing leverage answer)
	brokenBatch := map[string]ai.JevAnswer{
		"conviction": {Type: "score", Score: new(float64)},
		// "leverage" is missing!
	}

	_, err = reg.ResolveAll(brokenBatch, "chaos-cycle")
	if err == nil {
		t.Fatal("expected explicit error on missing managed answer mid-batch, got nil (zero-fallback violation)")
	}

	if !strings.Contains(err.Error(), "component=managed-params") {
		t.Errorf("expected error with component=managed-params, got: %v", err)
	}
	if !strings.Contains(err.Error(), "leverage") {
		t.Errorf("expected error naming missing parameter 'leverage', got: %v", err)
	}

	// Step 3: Out of vocabulary choice fails loud (FR-307)
	badChoiceBatch := map[string]ai.JevAnswer{
		"conviction": {Type: "score", Score: new(float64)},
		"leverage":   {Type: "choice", Choice: "10x"},
		"atr_regime": {Type: "choice", Choice: "INVALID_VOCAB"},
	}
	_, err = reg.ResolveAll(badChoiceBatch, "chaos-cycle")
	if err == nil {
		t.Fatal("expected explicit schema error on invalid choice, got nil")
	}
	if !strings.Contains(err.Error(), "out of vocabulary") {
		t.Errorf("expected out of vocabulary error, got: %v", err)
	}
}

// TestResolveEntry_DefersDecayAnswer covers the 2026-09-29 convergence fix:
// the entry batch never asks "decay" (news path asks per cluster), so
// ResolveEntry must not demand its answer — while every entry-batch managed
// parameter without an answer still fails loud (FR-307).
func TestResolveEntry_DefersDecayAnswer(t *testing.T) {
	for _, k := range []string{
		"MIN_RISK_TO_REWARD_RATIO", "DEFAULT_LEVERAGE", "MAX_RISK_PER_TRADE_PCT",
		"SL_ATR_MULT", "TP_ATR_MULT", "CLUSTER_DECAY_MODE", "CONFLUENCE_MIN",
	} {
		t.Setenv(k, "")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	reg := NewParamRegistry(cfg)

	score, conv, conf := 2.8, 0.8, 0.7
	entryAnswers := map[string]ai.JevAnswer{
		"min_rr_accept": {Type: "score", Score: &score},
		"leverage":      {Type: "choice", Choice: "8x"},
		"conviction":    {Type: "score", Score: &conv},
		"atr_regime":    {Type: "choice", Choice: "NORMAL"},
		"confluence":    {Type: "score", Score: &conf},
		// NOTE: no "decay" answer — production batches never request it.
	}

	resolved, err := reg.ResolveEntry(entryAnswers, "entry-deferred-decay")
	if err != nil {
		t.Fatalf("ResolveEntry failed without decay answer (should defer): %v", err)
	}
	if len(resolved) != 6 {
		t.Errorf("expected 6 recorded params (decay mode folded in), got %d", len(resolved))
	}
	decay, ok := resolved["decay"]
	if !ok {
		t.Fatalf("decay key missing from resolved record")
	}
	if decay.Mode != ModeManaged {
		t.Errorf("decay mode = %q, want core_managed", decay.Mode)
	}
	if decay.Value != nil {
		t.Errorf("managed decay must carry no value at entry (news path applies it), got %v", decay.Value)
	}

	modesJSON, _, _, _, err := PackageParamRecord(resolved)
	if err != nil {
		t.Fatalf("PackageParamRecord failed: %v", err)
	}
	var modes map[string]string
	if err := json.Unmarshal(modesJSON, &modes); err != nil {
		t.Fatalf("unmarshal modes: %v", err)
	}
	if len(modes) != 6 {
		t.Errorf("parameter_modes must cover 6 keys, got %d: %s", len(modes), string(modesJSON))
	}

	// Entry-batch managed param without answer still fails loud (FR-307).
	bad := make(map[string]ai.JevAnswer, len(entryAnswers))
	for k, v := range entryAnswers {
		bad[k] = v
	}
	delete(bad, "confluence")
	_, err = reg.ResolveEntry(bad, "entry-missing-confluence")
	if err == nil {
		t.Fatalf("expected explicit error for missing entry-batch managed answer")
	}
	if !strings.Contains(err.Error(), "confluence") {
		t.Errorf("error must name the missing parameter, got: %v", err)
	}
}
