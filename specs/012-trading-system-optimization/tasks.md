---
description: "Task list for Trading System Optimization (G1-G8)"
---

# Tasks: Trading System Optimization

**Input**: Design documents from `/specs/012-trading-system-optimization/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included — required by constitution VI (test-first) and research R10 (replay-simulation gate).

**Organization**: Tasks grouped by user story (US1–US5 from spec.md) for independent implementation.

## Format: `[ID] [P?] [Story] Description`

## Path Conventions

Backend: `internal/<pkg>/`, migrations: `internal/db/migrations/`, frontend: `web/src/`.

---

## Phase 1: Setup

- [X] T001 Create working branch `012-trading-system-optimization` from `main`
- [X] T002 Verify baseline green: `go build ./... && go test ./internal/... && cd web && bun test`
- [X] T003 [P] Add risk-profile config schema and defaults (research R2/R3/R4/R8 values) in `internal/config/risk_profiles.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**⚠️ CRITICAL**: No user story work begins until this phase completes

- [X] T004 DB migration per data-model.md §1/§2/§4: create `catalyst_events`, `risk_profiles`, `entry_filter_log`; `ALTER futures_trade_signals` ADD `profile`, `model_confidence`, `atr_at_entry`, `tp1_close_fraction`, `recomputed`, `decay_state`, `catalyst_event_id`, `rejected_reason` in `internal/db/migrations/0000XX_signal_optimization.up.sql`
- [X] T005 [P] Store methods + Go structs for new tables/columns with constraints from data-model.md (enum rules `profile ∈ {CRYPTO,COMMODITY}`, `rule ∈ 9-value set`, `recomputed bool default false`) in `internal/db/signal_store.go`, `internal/db/models.go`
- [X] T006 [P] Risk-profile loader: effective params resolution CRYPTO/COMMODITY + validation (`sl_min_pct ≤ … ≤ sl_max_pct`, `horizon_min < decay_flat_at_min < horizon_min`) in `internal/trader/riskprofile.go`
- [X] T007b [P] `GET /api/v1/risk-profiles` endpoint per contracts/api.md §4 in `internal/server/signal_handlers.go`, `internal/server/server.go`
- [X] T007 [P] Effective risk profiles seeded into `risk_profiles` at startup from config in `internal/trader/riskprofile.go`

**Checkpoint**: schema, models, and profile config ready — all stories unblocked

---

## Phase 3: User Story 5 — Honest performance reporting (Priority: P1)

**Goal**: every closed trade shows true return; summary shows win rate/payoff/expectancy per class

**Independent Test**: quickstart §2 and §6 — zero non-zero-move rows report `realized_roi_pct = 0`; summary reconciles with SQL

### Tests first

- [X] T008 [P] [US5] Unit tests: PnL/ROI sign for LONG and SHORT, exit==entry ⇒ 0, in `internal/trader/futures_math_test.go`
- [X] T009 [P] [US5] Idempotent-backfill test (run twice ⇒ second run `UPDATE 0`) in `internal/db/signal_store_test.go`

### Implementation

- [X] T010 [US5] Idempotent true-PnL backfill migration per research R9 in `internal/db/migrations/0000XX_true_pnl_backfill.up.sql`
- [X] T011 [US5] Performance summary aggregation (closed, wins, avg_win, avg_loss, payoff_ratio, expectancy_pct, stop_out_rate, tp1_hit_rate) per contracts/api.md §2 in `internal/server/signal_handlers.go`
- [X] T012 [US5] Web performance summary panel (per-profile + aggregate) in `web/src/components/PerformanceSummary.tsx`

**Checkpoint**: reporting trustworthy — validates every later story's metrics

---

## Phase 4: User Story 1 — Confirmed entries (Priority: P1) 🎯 MVP

**Goal**: entries gated by trend, volume, anti-chase; every rejection auditable

**Independent Test**: quickstart §3 — fixture scenarios each produce exactly one `entry_filter_log` row and no signal

### Tests first

- [X] T013 [P] [US1] Unit tests for gate rules (`CHASE_BLOCKED` z>2.0, `NO_VOLUME` ratio<2.5, `NO_TREND`, `LIQ_BUFFER`) in `internal/trader/entry_gate_test.go`
- [X] T014 [P] [US1] Integration test: three rejection fixtures + one pass-through in `internal/server/gate_integration_test.go`

### Implementation

- [X] T015 [P] [US1] EntryGate pure functions (VWAP/SuperTrend alignment, volume ≥2.5× 20-period 5m SMA, anti-chase z-score >2.0 ⇒ block) in `internal/trader/entry_gate.go`
- [X] T016 [US1] OI-delta fetcher, 500ms bounded timeout, neutral on failure (research R1) in `internal/market/open_interest.go`
- [X] T017 [US1] Wire gate into signal evaluation; write `entry_filter_log` with `rule` + `detail` jsonb (FR-020) in `internal/server/signal_handlers.go`
- [X] T018 [US1] `GET /api/v1/signals/filters` endpoint per contracts/api.md §3 in `internal/server/signal_handlers.go`
- [X] T019 [US1] SSE `filter_rejected` event per contracts/api.md §5 in `internal/server/` stream handler
- [X] T020 [US1] Entry-filter log panel (rules, symbol, detail) in `web/src/components/EntryFilterLog.tsx`

**Checkpoint**: MVP — gate active, auditable, independently deployable

---

## Phase 5: User Story 2 — Evidence-based exits (Priority: P1)

**Goal**: ATR stops, reachable staged targets, breakeven+decay exits, vol-target leverage, risk-based sizing

**Independent Test**: quickstart §1 — replay harness asserts TP1 hit ∈[40%,80%], stop-rate<20%, expectancy>0; unit tests pass

### Tests first

- [X] T021 [P] [US2] Unit tests: ATR stop with swing offset, TP1/TP2 bands inside empirical MFE, leverage buffer ≥4.0 rule, sizing = risk/(sl+0.1%) in `internal/trader/futures_math_test.go`
- [X] T022 [P] [US2] Unit tests: decay state machine NONE→BREAKEVEN→CLOSED at profile checkpoints in `internal/trader/signals_test.go`

### Implementation

- [X] T023 [P] [US2] ATR-based stop: `sl =2.0×ATR(1h)` behind 15m swing ±0.25 ATR, clamps [0.6%,2.5%], store `atr_at_entry` (FR-002) in `internal/trader/signals.go`
- [X] T024 [US2] Staged exits: TP1 `1.0×ATR` close 60%, auto breakeven+fees on TP1 fill, TP2 `1.8×ATR` runner; drop hard RR-floor (FR-003/004) in `internal/trader/signals.go`, `internal/server/signal_handlers.go`
- [X] T025 [US2] Active time-decay: BE at 30m if <+0.5R, flat at 40m if ≤0, profile-driven (FR-005) in `internal/server/signal_handlers.go`
- [X] T026 [US2] Vol-target leverage `min(8, 3.5/ATR%)` + liquidation-buffer invariant ≥4.0 (FR-006) in `internal/trader/signals.go`
- [X] T027 [US2] Sizing: slippage buffer +0.1%, tier budget as cap not fixed slot (FR-007) in `internal/trader/futures_math.go`
- [X] T028 [US2] Replay-simulation harness over trailing 30d klines with SC-002/003/004 assertions, wired into CI `test` job (R10) in `internal/trader/replay_test.go`, `.github/workflows/ci-cd.yml`
- [X] T029 [US2] Web: show staged targets, `atr_at_entry`, decay state on signal cards in `web/src/components/FuturesSignalsView.tsx`

**Checkpoint**: exit architecture validated by simulation evidence (constitution VIII)

---

## Phase 6: User Story 3 — News fusion (Priority: P2)

**Goal**: clustered catalysts with freshness decay, source weights, polarization veto; confidence ≠ sentiment

**Independent Test**: quickstart §3 — 5 identical headlines ⇒ 1 event `story_count=5`; polarized set ⇒ zero trades, `POLARIZED` log row

### Tests first

- [X] T030 [P] [US3] Unit tests: title normalization, 3-gram Jaccard ≥0.82 clustering, merge window 45m, decay weights, polarization >0.40 veto in `internal/market/news_cluster_test.go`

### Implementation

- [X] T031 [P] [US3] Clusterer: normalize → token 3-gram Jaccard → rolling 45m window in Redis (FR-008) in `internal/market/news_cluster.go`
- [X] T032 [US3] Source tier weights (1.0/0.6/0.2) + half-life decay 15m crypto / 60m commodity, discard >2× half-life (FR-009/010) in `internal/market/news_cluster.go`
- [X] T033 [US3] Polarization veto `P>0.40` → `POLARIZED` rejection (FR-011) in `internal/market/news_cluster.go`, `internal/trader/entry_gate.go`
- [X] T034 [US3] Persist `catalyst_events`; enforce one open position per symbol per event (FR-008) in `internal/db/signal_store.go`, `internal/server/signal_handlers.go`
- [X] T035 [US3] Split semantics: store fused score in `catalyst_sentiment`, AI confidence in `model_confidence` (FR-012) in `internal/trader/signals.go`, `internal/ai/client.go`
- [X] T036 [US3] Prompt payload: send clustered event (headline, story_count, fused score, freshness) instead of flat headline list in `internal/ai/client.go`
- [X] T037 [US3] Web: catalyst badge (story count, sources, freshness) on signal cards in `web/src/components/AISignalFeed.tsx`

**Checkpoint**: duplicate-story failures eliminated; AI sees whole story

---

## Phase 7: User Story 4 — Commodities section (Priority: P2)

**Goal**: diagnose zero-signal Core path, fix it, commodity profile with longer horizon/blackouts, dedicated UI

**Independent Test**: quickstart §5 — gold catalyst in open window ⇒ COMMODITY signal with 4h horizon; blackout ⇒ `EVENT_BLACKOUT` log; crypto unaffected

### Tests first

- [X] T038 [P] [US4] Unit tests: blackout window math (NFP/CPI/FOMC ±30m, EIA ±15m, weekend-gap Fri 16:45 ET), flat-before-close deadline in `internal/trader/commodity_session_test.go`

### Implementation

- [X] T039 [US4] Diagnose Core zero-signal root cause with per-stage dry-run logging (hypotheses H1–H4, research R7, FR-013); record finding in `docs/RESEARCH-trading-system-optimization.md` appendix in `internal/server/signal_handlers.go` (debug logging only)
- [X] T040 [US4] Fix root cause per T039 finding (scan order / regime gate / catalyst scarcity / prompt HOLD) in `internal/server/signal_handlers.go`
- [X] T041 [US4] COMMODITY risk profile seed: horizon 240/720m, SL 2.5×ATR, half-life 60m, risk 1.0% (FR-014, data-model §3) in `internal/config/risk_profiles.go`, `internal/trader/riskprofile.go`
- [X] T042 [US4] Event blackout calendar + weekend-gap guard + forced flat-before-close (FR-014) in `internal/trader/commodity_session.go`
- [X] T043 [US4] `profile=COMMODITY` filter on signals endpoint + commodities list API in `internal/server/signal_handlers.go`
- [X] T044 [US4] Commodities section UI: separate list, horizon/blackout status, own performance block (FR-015) in `web/src/pages/Commodities.tsx`, `web/src/components/CommoditySignalsList.tsx`

**Checkpoint**: reserved commodity capital now has a live, safe channel

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T045 [P] Full quickstart.md validation run (§1–§6) with results appended to spec checklists
- [X] T046 [P] Docs: update `docs/RESEARCH-trading-system-optimization.md` with implemented defaults + T039 diagnosis; strip Sync Impact Report comment from `.specify/memory/constitution.md` before commit
- [X] T047 Code cleanup: remove debug dry-run logging from T039, dead RR-floor remnants
- [ ] T048 Security pass: no secrets in new config, API additions are additive-only (contracts/api.md §1 compat rule), review blackout calendar timezone handling
- [X] T049 Performance: gate + clustering latency within plan budget (entry path <500ms worst case with OI timeout)
- [ ] T050 Run CI on PR: `go test ./...` + `bun test` green; merge to `main` triggers image build

---

## Phase 9: User Story 6 — Alert price precision (Priority: P1)

**Goal**: micro-cap prices stay distinct in Telegram alerts (FR-021, SC-009)

**Independent Test**: quickstart §8 — entry alert for a sub-1.0 signal shows three distinct prices, none degenerate

### Tests first

- [X] T051 [P] [US6] Unit tests: adaptive precision table (0, ≥100, ≥1, <1.0), micro-cap entry/resolution alerts show distinct prices, in `internal/telegram/formatter_test.go`

### Implementation

- [X] T052 [US6] Adaptive `FormatPrice` (7 significant digits below 1.0, cap 18 dp, conventional above) wired into entry + resolution alerts in `internal/telegram/formatter.go`

**Checkpoint**: alerts actionable for any asset price

---

## Phase 10: User Story 7 — Weight learning attribution (Priority: P2)

**Goal**: indicator weights learn from recorded decision-time measurements, direction-aware, unknown-safe (FR-022/023/024, SC-010/011)

**Independent Test**: quickstart §8 — win/loss batch updates only aligned indicators; snapshotless trades update nothing; saved slider weights are served

### Tests first

- [X] T053 [P] [US7] Unit tests: RecordOutcome alignment matrix (win/loss × aligned/unaligned × each indicator), unknown-zero guard, microstructure direction-aware, in `internal/ai/bayesian_test.go`

### Implementation

- [X] T054 [US7] DB migration: `futures_trade_signals.indicator_snapshot JSONB` (nullable) in `internal/db/migrations/000006_signal_indicator_snapshot.up.sql` + store struct/insert/scan roundtrip test in `internal/db/signal_store.go`, `internal/db/signal_store_test.go`
- [X] T055 [US7] Persist decision-time snapshot at signal assembly (RSI, MACD histogram, SuperTrend, CMF, KER, OBI, divergence) in `internal/trader/signals.go`
- [X] T056 [US7] Close paths populate `TradeOutcome` indicator fields from the recorded snapshot (both manual close and tick resolution) in `internal/server/signal_handlers.go`
- [X] T057 [US7] `RecordOutcome`: unknown-zero guards (RSI, KER), direction-aware microstructure via `EvaluateMicrostructure`, no update on missing data in `internal/ai/bayesian.go`
- [X] T058 [US7] Persist manual weight edits: save endpoint updates served posteriors, UI posts on save, in `internal/server/`, `web/src/components/AIWeightMatrix.tsx` (or mark controls read-only)

**Checkpoint**: closed trades actually sharpen (or damp) the indicators that were bold in their decisions

---

## Dependencies & Execution Order

### Phase Dependencies
- **Setup (P1)**: none — start immediately
- **Foundational (P2)**: after Setup — BLOCKS all stories
- **US5 → US1 → US2** (P1 order): reporting first so every later metric is trustworthy; US1/US2 otherwise independent
- **US3 (P2)**: after Foundational; integrates with US1's `entry_filter_log` (POLARIZED rows) — start after US1's T017 if serial
- **US4 (P2)**: after Foundational + US2 (commodity profile reuses exit math) ; T039 diagnosis must precede T040
- **Polish**: after all desired stories

### Parallel Opportunities
- Phase 2: T005, T006, T007 parallel (different files)
- US5: T008 ∥ T009
- US1: T013 ∥ T014; T015 ∥ T016
- US2: T021 ∥ T022; T023 can start once T021 written
- US3: T030 tests ∥ T031 implementation start; T031 ∥ T032 chain inside clusterer
- US4: T038 ∥ T039
- Cross-story after Foundational: US5 ∥ US1 ∥ US2 if staffed

## Implementation Strategy

**MVP (single deployable)**: Phase 1 → 2 → 3 (US5) → 4 (US1) → STOP/VALIDATE — honest numbers + gated entries ship together.
**Incremental**: US2 adds validated exits → US3 news quality → US4 commodities.
**Parallel team**: after Foundational, Dev A = US2, Dev B = US3 (post-T017), Dev C = US4 diagnosis.
