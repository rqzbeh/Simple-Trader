# Tasks: Jev News-Driven Early Trade Exit (spec-014)

**Input**: `specs/014-jev-news-early-exit/` · **Prerequisites**: plan.md, spec.md (US1-US3), research.md, data-model.md, contracts/early-exit.md, quickstart.md

**Tests**: REQUIRED (Constitution VI TDD). Tests precede implementation per phase.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [X] T001 Create migration `migrations/000008_early_exit_judgments.up.sql` + `.down.sql` (ROOT only) per data-model.md incl. UNIQUE(position_id, cluster_id) for terminal actions and `status='error'` CHECK constraints
- [X] T002 [P] `.env.example` += guard keys: `EARLY_EXIT_ENABLED=true`, `EARLY_EXIT_MIN_HOLD_MIN=30`, `EARLY_EXIT_MAX_PER_DAY=3`, `EARLY_EXIT_COOLDOWN_MIN=60`, `EARLY_EXIT_CONF_FLOOR=0.75`

## Phase 2: Foundational

- [X] T003 Failing tests `internal/config/early_exit_config_test.go`: invalid guard values (conf_floor out of 0..1, negative durations, budget<1) → startup error; kill switch parses (FR-104, no silent clamp)
- [X] T004 Implement guard config load in `internal/config` (satisfies T003)
- [X] T005 [P] Add 5 guard keys to PUT allowlist `internal/server/system_handlers.go` (contracts §5)

## Phase 3: User Story 1 — News-triggered early close (P1)

**Goal**: open positions × deduplicated cluster → one batched core judgment → DO_NOT_HOLD closes with NEWS_EARLY_EXIT + Telegram.
**Independent test**: injected cluster + forced flip → close + exactly-one telegram mock + judgment row (quickstart V3).

- [X] T006 [US1] Failing tests `internal/trader/early_exit_test.go`: question builder `close_now` Noul shape per contracts §1 (noul=true means close); batch = one question per open position in ONE request
- [X] T007 [US1] Failing tests: verdict mapping — noul ≥ floor → close path invoked (`ForceClosePosition(..., "NEWS_EARLY_EXIT")`), telegram called EXACTLY once after close; noul < floor → HOLD record; core error → error row, position untouched (FR-101/102/103/105, quickstart V2/V3/V4)
- [X] T008 [US1] Implement `internal/trader/early_exit.go`: dedup (position,cluster) check, batched judgment, verdict→guards→close→telegram-once orchestration (satisfies T006/T007)
- [X] T009 [US1] Telegram formatter: `internal/telegram/formatter.go` early-exit message per contracts §4 (symbol, direction, cluster, confidence, route, PnL); mockable sender interface for tests (FR-103)
- [X] T010 [US1] DB store: `internal/db` insert/join for `early_exit_judgments` incl. outcome backfill helper (FR-106)

## Phase 4: User Story 2 — Guardrails (P2)

**Goal**: min-hold, per-day budget, cooldown, confidence floor, kill switch — first-fail-wins, recorded.
**Independent test**: rapid clusters → budget respected, guarded_skip rows carry reason (quickstart V1/V5, SC-103).

- [X] T011 [US2] Failing tests: each guard blocks with exact reason string (kill_switch/min_hold/budget/cooldown/conf_floor); order = contracts §2; kill switch still records (FR-104/109)
- [X] T012 [US2] Implement guards in `internal/trader/early_exit.go` (satisfies T011)

## Phase 5: User Story 3 — Evidence & report (P3)

**Goal**: judgments joinable to outcomes; report counts.
**Independent test**: generated judgments → queryable + report includes early-exit stats (quickstart evidence gate).

- [X] T013 [US3] Failing tests for `internal/db` report query: counts closed/guarded_skip/error + outcome join (FR-106)
- [X] T014 [US3] Implement query + surface in `GET /api/admin/shadow/report?type=news_exit` variant (satisfies T013)

## Phase 6: Polish & Cross-Cutting

- [X] T015 [P] Failure-injection suite: Telegram down → close happens, telegram_error recorded, NO second close (SC-104); core timeout → explicit error (quickstart V4)
- [X] T016 Wire worker: start early-exit loop in `cmd/trader/main.go` (interval = news pipeline cadence); SSE broadcast on close via existing channels
- [X] T017 [P] Live smoke `-tags=liveapi`: real Jev close_now verdict shape validated
- [X] T018 Regression: `go test ./...`, `go vet ./...`, `docker compose build backend` green; kill switch toggling verified live (quickstart V7)

## Dependencies
Phase 1 → 2 → US1 → US2 (guards used by close path — T011/T012 before T008 final wiring) → US3.

**Parallel**: T001/T002; T005; T015/T017.

**MVP**: Phase 1+2+US1 with guards minimal (T001-T010).
**Strategy**: tests first; close-under-engine-lock preserved (no race); telegram strictly after successful close.
