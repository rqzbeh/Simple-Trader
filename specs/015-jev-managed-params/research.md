# Research: spec-015 (Phase 0)

Source: `/tmp/jev-offload-audit.md` (54 params: 13 jev_only, 8 prefilter, 15 hybrid, 18 stay-code).

- **Decision**: phase-1 scope = six user-named params; classification from audit:
  - `MIN_RISK_TO_REWARD_RATIO` → **hybrid**: Jev `Score` proposes acceptance (0.5–10 clamp); jev_prefilter where accuracy questioned.
  - `DEFAULT_LEVERAGE` → **hybrid**: Jev `Choice` (3x/5x/8x/10x/12x) → clamp to exchange max + margin ceiling (audit danger item: liquidation buffer stays code).
  - `MAX_RISK_PER_TRADE_PCT` → **hybrid**: Jev `Score` conviction 0–1 scales risk within hard cap [0.5%, 2.0%] (audit: half-Kelly within clamp).
  - ATR regime (`SLAtrMult`/`TP1AtrMult`) → **hybrid**: Jev `Choice` {TIGHT, NORMAL, WIDE} → multiplier set inside configured min/max stop % (audit top-5 #3).
  - Cluster decay → **jev_only**: Jev `Choice` {FAST_BREAKING, MACRO_THEMATIC} per cluster (audit top-5 #4).
  - Confluence acceptance → **jev_prefilter_9router**: Jev `Score` edge; escalate low-confidence (audit top-5 #5).
  - Rationale: judgments respond to regime; clamps preserve Constitution VIII + liquidation safety.
  - Alternative: jev_only for all — rejected for leverage/conviction (audit danger: hallucinated max = account wipeout without clamp).
- **Decision**: single `ParamRegistry` (one map) — resolution order: env present → override; else core. One code path reads all params (no scattered ifs).
  - Alternative: per-param feature flags — rejected (18 scattered toggles).
- **Decision**: managed questions ride EXISTING batches: entry batch (rr, leverage, conviction, atr_regime, confluence), news batch (decay per cluster). Zero extra round trips (FR-309, research.md of 016 proved batch works).
- **Decision**: invalid core answer → clamp to bound + record `clamped=true`; out-of-vocabulary → explicit schema error (fail loud, FR-307). No legacy-constant substitution.
- **Decision**: mode detection = raw env string empty/absent → managed (user's rule); SET (even "0"?) — invalid values still rejected by existing validators on PUT; direct-env-set invalid → startup error (existing validation pattern).
- **Decision**: must-stay params (audit class 18) structurally absent from registry — enforced by test list (drawdown, concurrent cap, fees, margin ceiling, liquidation, slippage).
- **Unknown resolved**: legacy static reads in signals.go (sizing `CalculatePositionSizingWithSlippage`, SL/TP `CalculateATRStop`/`CalculateStagedTargets`, confluence) — become: value := registry.Resolve(param, coreAnswer). Static path = override mode exactly as today.
