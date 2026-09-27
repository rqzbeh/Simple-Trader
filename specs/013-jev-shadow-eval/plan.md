# Implementation Plan: Jev Decision Model Shadow Evaluation

**Branch**: `013-jev-shadow-eval` | **Date**: 2026-09-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/013-jev-shadow-eval/spec.md`

## Summary

Add a shadow evaluation layer that runs three judgment types — entry (`LONG`/`SHORT`/`NO_TRADE`), exit (exit-now probability), news impact (`BULLISH`/`BEARISH`/`NEUTRAL`/`MIXED`) — against the Jev decision model (TypeSafe System One) while the live decision path stays byte-identical. Per user decision (Option A): the keyword lexicon news classifier is **replaced** by the OpenAI-compatible 9Router LLM classifier as the live news judgment; keyword word-lists are deleted, not kept as fallback. Jev shadows the LLM news classifier and both entry/exit paths. All failures surface as explicit errors (spec FR-007/007a/013). Paired records feed a comparison report (agreement, latency, cost, outcome-conditioned calibration) that gates any future promotion (FR-012).

## Technical Context

**Language/Version**: Go 1.24 (CGO_ENABLED=0, Constitution I)

**Primary Dependencies**:
- TypeSafe Jev: `POST https://api.typesafe.ai/v1/systemone`, Bearer key from env `TYPESAFE_API_KEY` (already in `/home/redsnow/.env`), `model: jev-latest` (verified live: HTTP 200, 0.55s)
- 9Router LLM classifier: existing OpenAI-compatible client (`internal/ai/client.go`) with JSON-constrained structured output
- Existing: Redis (cache), PostgreSQL 16 (persistence), SSE (streaming)

**Storage**: PostgreSQL 16 — new `shadow_judgments` table (audit-trail store per Constitution II); Redis for shadow config flags (dynamic config pattern)

**Testing**: Go `testing` unit + integration tests (Constitution VI); live-API tests behind build tag/env guard so CI stays deterministic

**Target Platform**: Linux VPS Docker Compose (Constitution VII)

**Project Type**: Web service (Go backend + React PWA)

**Performance Goals**: shadow entry/exit judgment p95 within decision cycle budget (Jev measured ~0.55s; LLM 1–3s — shadow runs async, never blocking live path)

**Constraints**:
- Shadow layer MUST NOT alter live order behavior (spec FR-001, US1.3)
- Position-intent vocabulary only for Jev/LLM questions; order-side conversion stays in execution layer (FR-005)
- No silent fallbacks anywhere in this feature; explicit errors with component+reason+cycle id (FR-007/007a)
- API credentials server-side only, never logged/committed/exposed (FR-006)

**Scale/Scope**: 113 pairs, ~8 RSS sources, evaluation window ≥14 days or ≥500 paired records per judgment type; admin-only comparison view

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Gate | Status | Notes |
|---|------|--------|-------|
| I | Pure Go, zero CGO, native indicators | ✅ PASS | New code is HTTP client + SQL; no CGO |
| II | Redis-first reads, PG persistence, SSE | ✅ PASS | Shadow config via Redis flags; records in PG; report via admin endpoint |
| III | Bun/React 18 PWA frontend | ✅ PASS | No frontend change in scope (admin report may reuse existing UI patterns; comparison UI minimal) |
| IV | Purge legacy Iranian market | ✅ PASS | Out of scope, untouched |
| VI | TDD & regression shielding | ✅ PASS | Task phase requires unit+integration tests first; shadow mode is inherently non-breaking (live path regression test required) |
| VII | Spec-Kit pipeline for trading-logic changes | ✅ PASS | This spec IS the pipeline artifact; news classifier replacement specified in FR-013/Option A |
| VIII | Evidence-based risk parameters | ✅ PASS with note | No risk-parameter changes in this feature; FR-012 gates promotion on recorded evidence; SC-005/SC-006 provide the evidence protocol |

**Violations requiring justification**: None.

## Design Decision: News Arms (user-confirmed Option A)

| Component | Before | After this feature |
|---|---|---|
| News live classifier | keyword lexicon (`AnalyzeNewsSentiment`) | **9Router LLM classifier** (semantic, structured output) |
| Keyword word-lists | live path | **deleted** (`bullishTerms`/`bearishTerms` removed; explicit config error if unreachable) |
| Jev | absent | shadow judge for entry + exit + news |
| Fallbacks | implicit masking | **none** — failures explicit (FR-007) |

Keyword results are NOT retained as baseline. Historical comparison baseline = realized outcomes (SC-006).

## Project Structure

### Documentation (this feature)

```text
specs/013-jev-shadow-eval/
├── plan.md              # this file
├── research.md          # Phase 0: Jev question-design + LLM prompt rules (primary sources)
├── data-model.md        # Phase 1: shadow_judgments, config, report queries
├── contracts/           # Phase 1: Jev client contract, LLM classifier contract, report API
├── quickstart.md        # Phase 1: validation scenarios
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks, not created here)
```

### Source Code (repository root)

```text
internal/
├── ai/
│   ├── jev.go           # NEW: Jev HTTP client (Choice/Noul/Score), typed errors
│   ├── jev_test.go      # NEW
│   ├── newsclassify.go  # NEW: LLM news classifier (replaces keyword path), structured output
│   ├── newsclassify_test.go # NEW
│   └── client.go        # EXISTING: extended only where structured-output call is shared
├── market/
│   ├── news_sentiment.go # MODIFIED: AnalyzeNewsSentiment + word-lists REMOVED (FR-013)
│   └── news_cluster.go   # UNCHANGED (clustering/weights stay)
├── trader/
│   ├── shadow.go         # NEW: shadow orchestrator (entry/exit hook points, async, non-blocking)
│   ├── shadow_test.go    # NEW
│   └── signals.go        # MINIMAL: call sites for shadow hooks; live logic untouched
├── db/
│   ├── shadow_store.go   # NEW: shadow_judgments persistence
│   └── shadow_store_test.go # NEW
└── server/
    └── shadow_handlers.go # NEW: admin comparison report endpoint

migrations/
└── XXX_shadow_judgments.sql # NEW (root migrations/ — see memory: dual-directory rule)

cmd/ ... unchanged
web/ ... unchanged (admin report served as JSON; UI optional)
```

**Structure Decision**: Single Go project, follow-the-caller layout — new files sit beside their domains (`ai` = external judgment clients, `trader` = orchestration hooks, `db` = persistence). No new packages/modules.

## Complexity Tracking

No constitution violations. Nothing to justify.
