# Feature Specification: Jev-Managed Trade Parameters with Optional Overrides

**Feature Branch**: `015-jev-managed-params`

**Created**: 2026-09-28

**Status**: Draft

**Input**: User description: "spec-015: Jev-managed trade parameters with optional .env overrides — offload decision-judged params (R:R acceptance, leverage, conviction, ATR regime, cluster decay, confluence) to the decision core; every param remains settable in .env: value set = user override wins, value unset/empty = LLM manages dynamically"

## Context

Today these judgment-like parameters are static configuration: minimum risk-reward acceptance, leverage choice, position conviction sizing, ATR stop/target regime, news-cluster decay, indicator confluence cutoffs. They were tuned once and do not respond to the current state of the world (volatility, catalyst strength, regime).

The parameter offload audit (54 params) classified which values are **judgments** (should be decided per-cycle by the Jev + 9Router core) vs **mechanics** (exchange math, fees, liquidation buffer, drawdown breaker — never delegated).

**Override law (user-decided):** every offloaded parameter REMAINS settable. Unset/empty = decision core manages it dynamically. Set = user value wins, enforced exactly as today. Delegation is opt-in per parameter by leaving it unset — nothing is forced, nothing is deleted.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Unset parameter = core manages it (P1)

When a judgment parameter has no value in configuration, the decision core evaluates it each cycle from the same state object (news, indicators, regime) alongside the entry/exit judgment, and the chosen value flows into the trade with its distribution recorded.

**Why this priority**: the core value — parameters that track reality instead of stale constants.

**Independent Test**: With all target parameters unset, produce a trade cycle → the record shows core-chosen values with distributions, and no default constant was substituted.

**Acceptance Scenarios**:
1. **Given** minimum risk-reward is unset, **When** the core judges an entry whose expected R:R is below a stale fixed constant would have been, **Then** the acceptance value comes from the core's answer (recorded), not a hardcoded number.
2. **Given** leverage unset, **When** entry is judged, **Then** leverage comes from the core within hard mechanical bounds (exchange max, margin ceiling — clamps stay).
3. **Given** conviction unset, **When** sizing runs, **Then** risk fraction scales with the core's conviction score within the existing safety cap.
4. **Given** ATR regime unset, **When** SL/TP levels compute, **Then** the multiplier set (tight/normal/wide) is the core's choice, still bounded by configured min/max stop percentages.

### User Story 2 - Set parameter = user value wins, untouched (P1)

Any parameter with a value in configuration behaves exactly as it does today: the core does not decide it, is not asked about it, and cannot influence it. Changing the value in the UI/settings takes effect as before.

**Why this priority**: the override contract — user control is absolute and immediate.

**Independent Test**: Set a parameter, run cycles → trade records show the configured value, no core judgment field for that parameter; unset it → core manages on next cycle.

**Acceptance Scenarios**:
1. **Given** `MIN_RISK_TO_REWARD_RATIO=2.5` is set, **When** trades run, **Then** the fixed value is used with zero core questions about R:R.
2. **Given** a parameter is set then later cleared, **When** the next cycle runs, **Then** the core takes over (mode recorded per trade).

### User Story 3 - Mode visibility & evidence (P2)

Every trade records, per parameter, whether the value was `user_override` or `core_managed` plus the core's distribution when managed — so overrides vs core decisions can be compared and tuned.

**Why this priority**: Constitution VIII — you tune what you can measure.

**Independent Test**: Mixed configuration (some set, some unset) → report shows per-parameter mode counts and outcome comparison.

**Acceptance Scenarios**:
1. **Given** any trade, **When** its record is read, **Then** each target parameter shows mode + value + (if core) distribution.
2. **Given** 14 days of mixed-mode trades, **When** the report runs, **Then** outcomes compare user-override vs core-managed per parameter.

### Edge Cases

- Core answer invalid (e.g. leverage out of hard bounds): value clamped to bound, clamp event recorded explicitly — never rejected silently, never allowed past mechanical limits.
- Core failure on a managed parameter while entry proceeds: explicit error on that parameter; trade does NOT silently fall back to the old constant (fails loud per zero-fallback law) unless parameter is set (then override path unaffected).
- Parameter set to invalid value: startup/settings error as today (validation unchanged).
- Mixed modes within one trade: each parameter independent; record carries per-parameter mode.

## Requirements *(mandatory)*

- **FR-301**: For each target parameter, configuration value present ⇒ user override wins; core is neither consulted nor able to influence it.
- **FR-302**: Configuration value unset/empty ⇒ decision core determines the parameter each cycle, batched into the existing core request (no extra round trip).
- **FR-303**: Target parameter set (phase 1): minimum risk-reward acceptance, leverage choice, position conviction (risk fraction scaling), ATR stop/target regime selection, news-cluster decay category, indicator-confluence acceptance.
- **FR-304**: Mechanical bounds remain hard code regardless of mode: exchange leverage max, liquidation buffer invariant, max margin per trade, drawdown breaker, fees/slippage. Core values are clamped to them; clamp events recorded.
- **FR-305**: Parameters classified must-stay-code in the offload audit MUST NOT be delegated (drawdown limit, concurrent-cap, fee/slippage, margin ceiling, liquidation math).
- **FR-306**: Every trade records per-parameter: mode (`user_override` | `core_managed`), final value, and (core mode) distribution + confidence.
- **FR-307**: Core failure on a managed parameter yields an explicit error naming parameter + cycle; no silent substitution of the legacy constant (zero-fallback law).
- **FR-308**: Mode switches take effect on the next cycle after settings change; no restart required for override edits; observable in the settings UI (each parameter shows current mode: "managed by core" vs value).
- **FR-309**: Latency: managed parameters ride the existing batched request — no measurable entry-latency increase.
- **FR-310**: Promotion safety: core-managed mode is togglable per parameter at any time via settings (set value = immediate revert to override).

### Key Entities

- **Managed Parameter**: name, override value (nullable), core question definition, hard bounds, mode.
- **Parameter Decision Record**: per-trade per-parameter — mode, value, distribution (core mode), clamp flag.
- **Hard Bound**: mechanical limit always enforced in code (never delegated).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-301**: With all target parameters unset, 100% of new trades carry core-managed values with recorded distributions; zero legacy constants leak into managed fields.
- **SC-302**: With a parameter set, 100% of trades use the exact configured value — verified byte-equal in records; core never asked.
- **SC-303**: Entry latency unchanged within measurement noise (parameter questions batched).
- **SC-304**: Zero mechanical-bound violations across all modes (clamps hold under core decisions).
- **SC-305**: After 14 days, report answers per parameter: core-managed vs user-override outcome rates — sufficient to decide defaults (Constitution VIII).
- **SC-306**: Mode switch (set↔unset) reflected within one cycle, visible in UI.

## Assumptions

- Offload audit (`/tmp/jev-offload-audit.md`) is the classification authority; phase-1 scope = six user-named judgment params (FR-303); remaining audit items are follow-up phases.
- Existing validation of set values stays unchanged; "unset" = empty string or key absent.
- Jev-first/9Router-escalated routing (spec-013) applies to parameter questions; jev_only vs prefilter classification per audit where accuracy requires.
- Settings persistence (PUT → .env) handles new override keys via the existing allowlist path.
- Paper/simulated flow during evidence window (SC-305) before any live default changes.

## Convergence (2026-09-29) — managed-params entry defect

**Defect**: `ResolveAll` demanded an answer for ALL 6 registry keys at entry, but the entry batch only asks 5 — `decay` is a news-path question (research.md). Real Jev never returns `decay` for entry batches → every managed entry failed `missing answer for managed parameter "decay" (FR-307)` from 2026-09-29 09:23. The unit tests masked this: `mockJevServerWithBatch` answered every key blindly instead of only the questions asked.

**Fix (FR-302/FR-307)**:
- `ParamRegistry.ResolveEntry` resolves the 5 `EntryParamKeys` strictly (managed-without-answer still fails loud), then folds the `decay` record: `user_override` ⇒ env value recorded; `core_managed` ⇒ mode only (value applied per cluster by the news path).
- `data-model.md` corrected: `parameter_distributions` for decay arrives with the news batch, not the entry record.
- Mock server now answers ONLY requested question IDs (mirrors the real API) — this class of drift fails tests again.
- Regression: `TestResolveEntry_DefersDecayAnswer`.

**Evidence**: full suite 11/11, `-race` clean, targeted tests 5 PASS.
