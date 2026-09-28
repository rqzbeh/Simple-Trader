# Implementation Plan: Jev News-Driven Early Trade Exit

**Branch**: `014-jev-news-early-exit` | **Date**: 2026-09-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/014-jev-news-early-exit/spec.md`

## Summary

Add a news-driven early-exit loop: for each open position, deduplicated news clusters feed one batched core judgment (`HOLD`/`DO_NOT_HOLD`, Jev-first per spec-013 routing). A `DO_NOT_HOLD` verdict closes the position with exit reason `NEWS_EARLY_EXIT`, sends one Telegram notification, and persists a judgment record. Hard guards (min-hold, per-day budget, cooldown, confidence floor, kill switch) gate every close; guard skips and failures are explicit records, never silent.

## Technical Context

**Language/Version**: Go 1.24 (CGO_ENABLED=0)

**Primary Dependencies**: spec-013 decision core (`internal/trader/decision_router.go`, `internal/ai/jev.go`), news clusterer (`internal/market/news_cluster.go`), execution engine close path (`internal/trader/engine.go` closePositionLocked), Telegram bot (`internal/telegram`), settings persistence (spec-013 follow-on: PUT /api/v1/system/config → .env)

**Storage**: PostgreSQL 16 — new table `early_exit_judgments` (root `migrations/000008_*`); GuardConfig keys in `.env` (single source of truth, UI-editable)

**Testing**: Go `testing` unit + integration; Telegram mocked; live-API behind `-tags=liveapi`

**Target Platform**: Linux VPS Docker Compose

**Performance Goals**: judgment completes within news pipeline cycle (Jev ~0.5s); batch = 1 core call per cycle for all open positions

**Constraints**: core-only decision making (spec-013 FR-001); explicit errors only (FR-105); position-intent vocabulary `HOLD`/`DO_NOT_HOLD` (FR-107); notification failure never blocks/duplicates close (FR-103)

**Scale/Scope**: ≤5 concurrent positions (MAX_CONCURRENT_SIGNALS); news clusters already deduplicated by clusterer; one new table, one new loop, one Telegram message type

## Constitution Check

| # | Gate | Status | Notes |
|---|------|--------|-------|
| I | Pure Go | ✅ | new code = Go only |
| II | Redis-first / PG audit / SSE | ✅ | judgment rows in PG; SSE broadcast on early close via existing channels |
| III | Bun/React PWA | ✅ | no frontend required (settings exposed via existing config UI) |
| IV | No Iranian market | ✅ | untouched |
| VI | TDD | ✅ | guards/close/notification each test-first |
| VII | Spec-Kit pipeline | ✅ | this artifact |
| VIII | Evidence-based params | ✅ | guard values configurable + SC-105 evidence window before live defaults |

**Violations**: none.

## Project Structure

### Documentation (this feature)

```text
specs/014-jev-news-early-exit/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/early-exit.md
└── tasks.md            # /speckit-tasks output
```

### Source Code (repository root)

```text
internal/trader/
├── early_exit.go         # NEW: loop, guards, judgment orchestration
├── early_exit_test.go    # NEW (TDD)
└── engine.go             # MINOR: expose close with exit-reason override (NEWS_EARLY_EXIT)
internal/telegram/
├── formatter.go          # ADD: early-exit message type
internal/server/
└── system_handlers.go    # MINOR: guard-config settings keys in allowlist (settings persistence)
migrations/
└── 000008_early_exit_judgments.up.sql / .down.sql   # ROOT only (dual-directory rule)
cmd/trader/main.go        # MINOR: start early-exit loop worker
```

**Structure Decision**: follow-the-caller; loop lives in `trader` beside signals/engine; no new packages.

## Complexity Tracking

No violations.
