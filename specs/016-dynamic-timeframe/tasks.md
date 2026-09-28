# Tasks: Dynamic Trade Timeframe Selection (spec-016)

**Input**: `specs/016-dynamic-timeframe/` · **Prerequisites**: plan.md, spec.md (US1-US3), research.md, data-model.md, contracts/timeframe.md, quickstart.md

**Tests**: REQUIRED (Constitution VI TDD). Tests precede implementation per phase.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [X] T001 Create migration `migrations/000009_signal_timeframe.up.sql` + `.down.sql` (ROOT only): add `timeframe text`, `timeframe_distribution jsonb`, `timeframe_confidence double precision` columns (NULL = legacy) per data-model.md
- [X] T002 [P] Config keys: `.env.example` += `TIMEFRAME_SET_ALPHA=15m,1h,4h`, `TIMEFRAME_SET_CORE=1h,4h,12h`

## Phase 2: Foundational

- [X] T003 Failing tests `internal/config/timeframe_config_test.go`: empty set → startup error; option not in known-interval set {15m,30m,1h,2h,4h,6h,12h,1d} → startup error; valid sets parse (FR-203, quickstart V1)
- [X] T004 Implement `internal/config/timeframe_config.go`: parse/validate bucket sets + `TimeframeProfile` map {HorizonMin,BEOffsetMin,FlatOffsetMin} seeded per data-model.md (satisfies T003)
- [X] T005 [P] Add `TIMEFRAME_SET_ALPHA`/`TIMEFRAME_SET_CORE` to PUT allowlist in `internal/server/system_handlers.go` with same whitelist+validation path (contracts §5)

## Phase 3: User Story 1 — Core chooses timeframe at entry (P1)

**Goal**: batched `timeframe` Choice rides entry request; validated ∈ set; persisted with signal; horizon from profile.
**Independent test**: forced answer → signal row carries timeframe+distribution; invalid answer → entry aborts loud.

- [X] T006 [US1] Failing tests `internal/trader/timeframe_test.go`: question builder — criteria keys == configured set exactly; ALPHA never offers 12h; criteria text includes horizon+decay per FR-208 (quickstart V2)
- [X] T007 [US1] Implement `internal/trader/timeframe.go`: `TimeframeQuestion(bucket string)` builder + `ValidateTimeframe(answer, bucketSet)` returning typed error (component=`timeframe`, FR-202) (satisfies T006)
- [X] T008 [US1] Failing tests: entry path — mocked batch response incl. `timeframe:"1h"` → signal persisted with timeframe+distribution+confidence AND horizon == profile["1h"].HorizonMin (not static 60/240); `timeframe:"2h"` ∉ set → typed error, no signal row (FR-201/202/204/205, quickstart V3/V4, SC-202)
- [X] T009 [US1] Wire into `internal/trader/signals.go`: add question to batch (same Evaluate call), validate, persist columns atomically with signal insert, horizon from TimeframeProfile (satisfies T008)
- [X] T010 [US1] DB store support: `internal/db` signal insert/select includes timeframe columns (satisfies T009 persistence)

## Phase 4: User Story 2 — Bucket choice sets (P2)

**Goal**: sets config-driven (env → settings UI); question generated from sets.
**Independent test**: PUT new set → next entry offers only new options (quickstart V6).

- [X] T011 [US2] Failing test: `TimeframeQuestion` reads live config (set mutated at runtime → criteria keys follow) (quickstart V6)
- [X] T012 [US2] Implement config accessor used by question builder + `system_handlers.go` PUT applies set changes in-process (satisfies T011; depends T005)

## Phase 5: User Story 3 — Timeframe-aware exits (P3)

**Goal**: decay/reconciler per stored timeframe; legacy rows mapped+logged.
**Independent test**: 15m signal decays on 15m cadence; NULL row mapped once with log (quickstart V5).

- [X] T013 [US3] Failing tests `internal/server` decay tests: `applyDecayState` with signal.timeframe=15m → offsets from profile; timeframe NULL → legacy map (CRYPTO→1h, COMMODITY→4h) written + `legacy_timeframe_mapped` logged; second touch uses stored value (FR-206)
- [X] T014 [US3] Implement in `internal/server/signal_handlers.go:~1093 applyDecayState`: read signal.Timeframe via profile map, legacy migration path (satisfies T013)
- [X] T015 [US3] Candle fetch for chosen interval: verify per-interval provider path works for signal's timeframe; missing interval data → explicit error (FR-207)

## Phase 6: Polish & Cross-Cutting

- [X] T016 [P] Live smoke `-tags=liveapi`: real batch answers both `entry`+`timeframe`; latency delta vs entry-only ≤ noise (SC-205, quickstart V7)
- [X] T017 Regression: `go test ./...` + `go vet ./...` green; full `docker compose build backend` (tsc gate)
- [X] T018 Distribution evidence: query helper counts chosen-timeframe distribution per bucket (SC-203 input)

## Dependencies
Phase 1 → 2 → US1 → US2 → US3; T005 before T012; T003/T004 before T006.

**Parallel**: T001/T002; T006 before T007; T016/T017.

**MVP**: Phase 1+2+US1 (T001-T010).
**Strategy**: tests first, one story at a time, deletion of static-horizon-at-entry gated by T008 assertions.
