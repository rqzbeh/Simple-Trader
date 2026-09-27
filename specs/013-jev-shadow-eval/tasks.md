# Tasks: Jev + 9Router Decision Core (spec-013 v3.0)

**Input**: Design documents `specs/013-jev-shadow-eval/`

**Prerequisites**: plan.md, spec.md (v3.0, US1-US4), research.md (question set + prompt rules), data-model.md, contracts/shadow-jobs.md, quickstart.md

**Tests**: REQUIRED — Constitution VI (TDD & regression shielding) + spec SC-007 chaos tests. Test tasks precede implementation per story.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallel (different files, no dependencies)
- **[Story]**: user story (US1-US4)

---

## Phase 1: Setup

- [X] T001 Update `plan.md`, `research.md`, `data-model.md`, `contracts/shadow-jobs.md`, `quickstart.md` to v3.0 routing (jev_direct/escalated, one-final-writer, feeder demotion) — remove shadow-only language superseded by decision-core unification
- [X] T002 [P] Add env vars to `.env.example`: `TYPESAFE_API_KEY`, `ROUTING_CONFIDENCE_THRESHOLD` (no default in code — startup error if missing, FR-016), `SHADOW_REPORT_DAYS`, `SHADOW_MIN_PAIRS`
- [X] T003 [P] Create migration `migrations/0XX_shadow_decisions.sql` (ROOT `migrations/` only — dual-directory rule) implementing `shadow_decisions` table per data-model.md: fields `id, created_at, judgment_type CHECK IN ('entry','exit','news'), cycle_id, symbol, news_cluster_id, state_ref, judge CHECK IN ('jev','llm'), choice, noul, score, probabilities JSONB, confidence, baseline_choice, latency_ms, input_tokens, output_tokens, status CHECK IN ('ok','error'), error, outcome`; CHECK: `status='error'` ⇒ `error IS NOT NULL`; indexes on `(judgment_type, created_at)`, `(cycle_id)`, `(status, created_at)`

## Phase 2: Foundational (blocking — no story starts before these)

- [X] T004 Write failing tests: typed error contract `internal/ai/jev_errors_test.go` — `ErrJevAuth/ErrJevTimeout/ErrJevSchema/ErrJevRateLimit/ErrJevUnavailable` each carrying component+cycle_id+status; assert no default-valued answers ever returned (FR-007)
- [X] T005 [P] Write failing tests: deleted-symbol guard `internal/ai/deleted_test.go` — build-tag-free static assertions (grep-in-test) that `fallbackHeuristic`, `AnalyzeNewsSentiment`, `bullishTerms`, `bearishTerms` are absent from repo (FR-007a, FR-013; quickstart V1)
- [X] T006 Implement typed errors `internal/ai/jev_errors.go` per contracts/shadow-jobs.md §1 (satisfies T004)
- [X] T007 [P] Implement config loader for `ROUTING_CONFIDENCE_THRESHOLD` + shadow flags in `internal/trader/config.go` (or existing dynamic config path) with **startup error when missing** — no silent default (FR-016); tests first in T004 pattern: `internal/trader/config_test.go`
- [X] T008 Implement DB store `internal/db/shadow_store.go` + `internal/db/shadow_store_test.go`: insert validation per data-model.md rules (`status='error'` ⇒ non-empty error, type-specific value present, probabilities keys match allowed choice set), backfill `outcome` join helper (FR-002, FR-008)

## Phase 3: User Story 1 — Decision core owns entry (P1)

**Goal**: one State Object → Jev first → escalation → single final decision recorded with route.
**Independent test**: fixed state cycles produce exactly one `route` record; grep proves removed symbols gone; orders byte-identical with shadow on/off.

- [X] T009 [US1] Failing tests for Jev client `internal/ai/jev_test.go`: mock `POST /v1/systemone` → assert request shape (`state`, `model:"jev-latest"`, questions per research.md §Proposed question set), parse `choice/probabilities/confidence/noul/score`, schema-mismatch ⇒ `ErrJevSchema` (contracts §1, research.md rules: pre-computed math, filtered state, concrete criteria w/ negative boundaries)
- [X] T010 [US1] Implement Jev client `internal/ai/jev.go`: single-shot batched questions (independent questions in ONE request), bounded timeout, typed errors only (satisfies T009)
- [X] T011 [US1] Failing tests for router `internal/trader/router_test.go`: confidence ≥ threshold ⇒ `route='jev_direct'`; < threshold ⇒ 9Router invoked, its answer final, `route='escalated'`, both steps recorded; escalation failure ⇒ explicit error, decision NOT downgraded to Jev guess (FR-001, FR-003, edge case "one final writer")
- [X] T012 [US1] Implement decision router `internal/trader/router.go` with `ROUTING_CONFIDENCE_THRESHOLD` (satisfies T011)
- [X] T013 [US1] Failing tests for state builder `internal/trader/state_test.go`: assembles candles, indicators (values), news labels, LSTM stats, ATR levels, gates-as-fields into one object; feeder error ⇒ field marked error, not fabricated value; empty/degenerate state ⇒ explicit error naming missing inputs (FR-014, US edge cases)
- [X] T014 [US1] Implement state builder `internal/trader/state.go` (satisfies T013)
- [X] T015 [US1] Delete `fallbackHeuristic` + all 11 call sites in `internal/ai/client.go` (parse hacks at lines ~310-328 and error paths ~216-305); route all former callers to typed errors; verify `internal/ai/deleted_test.go` passes (FR-007a)
- [X] T016 [US1] Wire post-decision hook `internal/trader/shadow.go`: async goroutine + bounded queue (queue-full ⇒ error record), never blocks live path; calls router, persists via shadow_store (FR-001 non-interference)
- [X] T017 [US1] Integration test `internal/trader/shadow_integration_test.go`: orders + live decision records byte-identical with shadow on/off (quickstart V3, SC-001)
- [X] T018 [US1] Regression: run full `go test ./...` — no existing behavior broken (Constitution VI)

## Phase 4: User Story 2 — News feeds the core (P2)

**Goal**: clusters classified by core (Jev Choice first → escalation); lexicon deleted; labels are context fields + standalone records.
**Independent test**: negation/slang clusters labeled; failure = explicit error; zero lexicon references.

- [X] T019 [US2] Failing tests `internal/ai/newsclassify_test.go`: structured-output contract per contracts §2 — schema field order `evidence, reasoning, label` (research.md: label-last prevents rationalization), balanced few-shot in prompt (research.md), negation/slang cases, label ∈ {BULLISH,BEARISH,NEUTRAL,MIXED}, explicit error on any failure (FR-007, FR-013)
- [X] T020 [US2] Implement LLM news classifier `internal/ai/newsclassify.go` via existing 9Router client + JSON schema response_format (satisfies T019)
- [X] T021 [US2] Delete `AnalyzeNewsSentiment`, `bullishTerms`, `bearishTerms` from `internal/market/news_sentiment.go`; rewrite callers (`internal/ai/sentiment.go`, `internal/market/news_crawler.go`, `internal/trader/signals.go`, `internal/server/signal_handlers.go`, `internal/server/server.go`) to core-classification path; deletion guard T005 must pass (FR-013)
- [X] T022 [US2] Failing tests `internal/trader/news_test.go`: cluster → core news questions (Jev `news_impact` Choice + escalation) → label stored as context field in state + standalone record with cycle_id (FR-002, US2)
- [X] T023 [US2] Implement news hook `internal/trader/news_hook.go` wiring clusterer output → core → state field + shadow_decisions row (satisfies T022)
- [X] T024 [US2] Chaos test: kill classifier (mock 500/timeout) ⇒ surfaced error names component; no neutral default, no keyword path reachable (SC-007, quickstart V4)

## Phase 5: User Story 3 — Exit owned by core (P3)

**Goal**: exit-now decided by core (Jev noul → escalation); ATR levels are context fields, not triggers.
**Independent test**: open positions + forced low confidence ⇒ escalated exit recorded; no non-core trigger fires.

- [X] T025 [US3] Failing tests `internal/trader/exit_test.go`: exit cycle invokes core with `exit_now` question + ATR levels as data; route recorded; outcome joinable; ATR stop alone never executes without core decision (US3 acceptance, FR-014)
- [X] T026 [US3] Implement exit hook `internal/trader/exit_hook.go`: replace independent SL/TP trigger authority with core judgment carrying ATR bounds as state (satisfies T025; constitution VIII — ATR bounds remain evidence-derived values, now non-authoritative)
- [X] T027 [US3] Integration test: manual + tick-driven exit paths produce exactly one core-decided exit record (FR-001 one-writer)

## Phase 6: User Story 4 — Evidence report (P4)

**Goal**: report from stored data only: agreement, latency, cost, routing distribution, calibration.
**Independent test**: generate records ⇒ report fields computable.

- [X] T028 [US4] Failing tests `internal/server/shadow_handlers_test.go`: `GET /api/admin/shadow/report?days=14&type=entry|exit|news` returns pairs, agreement_rate, latency p50/p95 per judge, cost_per_1k, calibration buckets, route distribution; admin auth only; read-only (FR-002, FR-017, contracts §4)
- [X] T029 [US4] Implement report handler `internal/server/shadow_handlers.go` + SQL aggregation in `internal/db/shadow_store.go` (satisfies T028)
- [X] T030 [US4] Calibration query test: bucket confidence, compare vs realized `outcome` frequency (SC-006)

## Phase 7: Polish & Cross-Cutting

- [X] T031 [P] Chaos suite `internal/trader/chaos_test.go`: inject failure at every component (Jev, 9Router, each feeder, DB) ⇒ each yields surfaced explicit error with component+cycle_id; zero silent degradation (SC-007)
- [X] T032 [P] Live-guarded smoke test `internal/ai/jev_live_test.go` behind `-tags=liveapi`: real key, HTTP 200, valid choice set, probabilities sum≈1, latency <1s (quickstart V2)
- [X] T033 [P] Update `internal/telegram/formatter.go` text path: thesis generation via 9Router only (Jev never generates text); decision fields rendered from core record (research.md: 9Router-first for free text)
- [X] T034 Config toggle tests: `shadow:type:news=false` stops news records only; global off ⇒ zero records, live unaffected (FR-016, quickstart V6)
- [X] T035 Run quickstart V1-V6 end-to-end on docker compose stack; record results in `specs/013-jev-shadow-eval/quickstart.md` checklist
- [X] T036 Sync root `migrations/` from `internal/db/migrations/` if dual copies exist (memory: schema silently skips otherwise)
- [X] T037 Load omni-skills `prompt-engineering-patterns` + `llm-evaluation` (via get_skill, never save to disk) and re-audit classifier prompts/judge question set against their eval guidance before closing
- [X] T038 Final `go test ./...` + `go vet ./...`; update README health version string check (`3.0.0-decision-core`)

---

## Dependencies

- Phase 1 → Phase 2 (env/migration/error contract)
- Phase 2 → all stories (typed errors, store, config)
- US1 → US2, US3 (router + state builder reused)
- US4 needs US1-US3 records (report on data)
- T005 deletion guards run continuously (fail if anyone re-adds lexicon/heuristic)

**Parallel opportunities**: T002/T003; T004/T005; T006/T007/T008; within US1 tests before impls; T019/T022 parallel; T028/T030/T032/T033 in polish.

**MVP scope**: Phase 1+2+US1 (T001-T018) — decision core with entry routing, non-interference proven.

**Implementation strategy**: tests first, one story at a time in priority order, deletion guards green at all times, full regression after each story (T018 pattern).

## Phase 8: Convergence

- [X] T039 CRITICAL Wire DecisionRouter into live entry path: EvaluateMarketSignal must route via Jev-first/9Router-escalated core (SignalService holds legacy aiClient.Analyze only) per FR-001, FR-003 (missing)
- [X] T040 CRITICAL Wire ShadowOrchestrator: Start()+JudgeEntry never invoked anywhere in cmd/ or server/ — shadow store unreachable at runtime per FR-002, US1/AC4 (missing)
- [X] T041 CRITICAL Wire JudgeExit into exit-evaluation cycle of ExecutionEngine per FR-003, US3 (missing)
- [X] T042 HIGH Create missing test files marked done in tasks: internal/ai/jev_test.go, internal/ai/newsclassify_test.go, internal/trader/exit_test.go, internal/trader/news_test.go, internal/trader/config_test.go, internal/db/shadow_store_test.go — contracts per T009/T019/T025/T022/T007/T008 (partial)
- [X] T043 HIGH Create internal/trader/news_hook.go + SetNewsClassifier on SignalService call-site wiring (server currently only wires crawler; signal path classifier injected but cluster→shadow record missing) per FR-002 news rows (partial)
- [X] T044 HIGH ShadowReport SQL bug: GROUP BY includes outcome (row per outcome) making totals/agreement wrong; also route column selected but absent from GROUP BY — fix query per FR-017 (contradicts)
- [X] T045 HIGH Startup gate: ROUTING_CONFIDENCE_THRESHOLD parse (Config.RoutingThreshold) never called at boot — FR-016 startup error unreachable (partial)
- [X] T046 MEDIUM Config toggles (SHADOW_REPORT_DAYS/MIN_PAIRS, per-type shadow flags) not read from env/config — FR-010 unenforced (missing)
- [X] T047 MEDIUM Calibration bucketing per SC-006 absent from report (confidence buckets vs realized outcome frequency) (missing)
