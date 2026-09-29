# Tasks: Jev-Managed Trade Parameters (spec-015)

**Input**: `specs/015-jev-managed-params/` · **Prerequisites**: plan.md, spec.md (US1-US3), research.md, data-model.md, contracts/managed-params.md, quickstart.md

**Tests**: REQUIRED (Constitution VI TDD). Tests precede implementation per phase.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [X] T001 Create migration `migrations/000010_signal_parameter_modes.up.sql` + `.down.sql` (ROOT only): `parameter_modes JSONB NOT NULL DEFAULT '{}'`, `parameter_values JSONB NOT NULL DEFAULT '{}'`, `parameter_distributions JSONB`, `parameter_clamps JSONB` per data-model.md
- [X] T002 [P] `.env.example`: document the 7 optional override keys with comment `# empty/unset = decision core manages; set value = user override` (keys per contracts §4: MIN_RISK_TO_REWARD_RATIO, DEFAULT_LEVERAGE, MAX_RISK_PER_TRADE_PCT, SL_ATR_MULT, TP_ATR_MULT, CLUSTER_DECAY_MODE, CONFLUENCE_MIN — present existing ones, add SL/TP_ATR_MULT + CLUSTER_DECAY_MODE + CONFLUENCE_MIN if absent)

## Phase 2: Foundational

- [X] T003 Failing tests `internal/trader/param_registry_test.go`: (a) all-override mode → 6 params report `user_override`, value==config, NO questions built (FR-301, quickstart V1); (b) all-unset → `core_managed`, questions built (FR-302, V2); (c) must-stay guard: registry contains NONE of MAX_DRAWDOWN_LIMIT_PCT/MAX_CONCURRENT_SIGNALS/MAKER_FEE_RATE/TAKER_FEE_RATE/MAX_SLIPPAGE_PCT/MAX_TRADE_MARGIN_PCT (FR-305, V3)
- [X] T004 Implement `internal/trader/param_registry.go`: `ParamSpec{Key, Mode(), Bounds, Question, Clamp}` + `Resolve()` per contracts §1; 6 phase-1 entries (research.md classification) (satisfies T003)
- [X] T005 [P] Config: load the 7 override keys as OPTIONAL in `internal/config/config.go` (empty string allowed = Managed; invalid values still error) + mode getter helpers

## Phase 3: User Story 1 — Unset = core manages (P1)

**Goal**: managed params judged in batched core requests, clamped, recorded.
**Independent test**: all-unset cycle → core values + distributions in signal record, zero legacy constants (quickstart V4, SC-301).

- [X] T006 [US1] Failing tests `internal/trader/managed_params_test.go`: entry batch gains questions ONLY for managed params (override param question absent — FR-301); mocked answers → clamped per contracts §3 (leverage 25x→12x recorded in clamps — V8); missing managed answer → explicit error, no constant substitution (FR-307, SC-301)
- [X] T007 [US1] Question builders per contracts §2 (min_rr_accept Score, leverage Choice, conviction Score, atr_regime Choice, confluence Score, decay Choice) with pre-computed data in instructions (research rule) (satisfies T006 builders)
- [X] T008 [US1] Wire into `internal/trader/signals.go`: build questions for managed params in the SAME batched call as entry/timeframe; consume answers through Resolve() → sizing (`CalculatePositionSizingWithSlippippage` input), R:R gate, ATR multipliers, confluence gate (FR-302, FR-309 — zero extra round trips)
- [X] T009 [US1] News decay: wire `decay` question for managed `CLUSTER_DECAY_MODE` into cluster path (`internal/server/signal_handlers.go` cluster section or market clusterer input) — FAST_BREAKING/MACRO_THEMATIC drives freshness half-life (FR-302)
- [X] T010 [US1] DB: extend `internal/db/signal_store.go` insert/select with parameter_modes/values/distributions/clamps columns; atomic with signal insert (same tx pattern as timeframe) (data-model validation: all 6 mode keys always present)

## Phase 4: User Story 2 — Set = user wins untouched (P1)

**Goal**: override mode bypasses core entirely; live flip via settings.
**Independent test**: set param → trade records configured value, no question asked; clear → core takes over next cycle (quickstart V5/V6, SC-302/306).

- [X] T011 [US2] Failing tests: set MIN_RISK_TO_REWARD_RATIO=2.5 → question absent from batch, record mode=user_override, value=2.5 exactly; runtime clear (config mutated) → next Resolve returns core_managed (FR-301, FR-308)
- [X] T012 [US2] Add 7 keys to PUT allowlist `internal/server/system_handlers.go` with empty-string = delete-key semantics (contracts §4); GET config returns per-param mode map (contracts §5) (satisfies T011 settings path)

## Phase 5: User Story 3 — Mode visibility & evidence (P2)

**Goal**: per-param modes visible in UI + reportable.
**Independent test**: mixed config → report/UI shows modes + outcomes (quickstart V5, SC-305).

- [X] T013 [US3] Failing tests: report query counts per parameter mode (`user_override` vs `core_managed`) joined to outcomes (SC-305 input, FR-306)
- [X] T014 [US3] Implement report aggregation in `internal/db/shadow_store.go` (or signal store) + surface via `GET /api/admin/shadow/report?type=params` (FR-306)
- [X] T015 [US3] GUI: SystemStats Decision Core card lists 6 params with mode badges ("core-managed" vs "= value") from GET config (FR-308 visibility) `web/src/components/SystemStatsView.tsx`

## Phase 6: Polish & Cross-Cutting

- [X] T016 [P] Live smoke `-tags=liveapi`: real batch answers managed param questions; latency delta ≤ noise (SC-303, quickstart V7)
- [X] T017 Regression: `go test ./...` + `go vet ./...` + `go test -race` green; `docker compose build backend` (tsc) clean
- [X] T018 [P] Clamp drill integration: forced out-of-bound core answers across all 6 → every value applied within bounds + clamp record present (SC-304, V8)
- [X] T019 Chaos: core failure mid-batch with managed param → explicit error, no legacy constant leaks (FR-307); override params unaffected (V4 negative)

## Dependencies
Phase 1 → 2 → US1 → US2 (settings) → US3 (visibility). T003/T004 before T006-T009.

**Parallel**: T001/T002; T005; T016/T018.

**MVP**: Phase 1+2+US1 (T001-T010).
**Strategy**: tests first; registry single source (no scattered ifs); batch-only questions (no latency); clamps enforced in code always.
