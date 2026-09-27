# Implementation Plan: Trading System Optimization

**Branch**: `012-trading-system-optimization` | **Date**: 2026-09-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/012-trading-system-optimization/spec.md`

## Summary

Close 8 evidence-based gaps (G1–G8) in the futures signal pipeline: gate entries on technical
confirmation, rebuild stop/target/leverage/size from measured volatility distributions, cluster
and decay news catalysts (with polarization veto), separate model confidence from news sentiment,
diagnose and fix the zero-signal Core commodities path with its own longer-horizon profile and
UI section, and persist/report true PnL everywhere. Grounded in `docs/RESEARCH-trading-system-optimization.md`.

## Technical Context

**Language/Version**: Go 1.24 (backend, `CGO_ENABLED=0`), TypeScript/React 18 (web, Bun 1.3+)

**Primary Dependencies**: chi router, go-redis v8, pgx/SQL, lightweight-charts v4.2, OpenAI-compatible `/v1/chat/completions` client

**Storage**: PostgreSQL 16 (`futures_trade_signals`, `news_articles`, new columns/tables per data-model), Redis 7 (cache, Pub/Sub, rolling cluster state)

**Testing**: `go test ./internal/...` (unit + handler tests), `bun test` (web), plus new replay-simulation harness cases

**Target Platform**: Linux ARM64 VPS via Docker Compose (`ghcr.io/rqzbeh/simple-trader-backend`), Nginx PWA

**Project Type**: web-service (Go API + SSE + React PWA)

**Performance Goals**: signal evaluation cycle unchanged (<1s reconcile tick); news clustering <50ms per batch; no added blocking network calls in the entry path except optional OI fetch (bounded 500ms timeout)

**Constraints**: config-driven parameters (FR-019), backward-compatible API responses (existing consumers), no new third-party services (no embeddings vendor), no forex

**Scale/Scope**: 123 registered assets (10 Core commodities), ~15-headline evaluation window growing to full rolling buffer, ~50 closed signals/day expected; ~10 Go files touched, ~6 web components touched

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Gate (constitution v1.1.0) | Status | Evidence |
|---|---|---|---|
| 1 | I. Production-ready zero-CGO Go, native indicator math | PASS | All new math (ATR stops, MFE bands, decay, sizing) implemented natively in `internal/trader` + `internal/indicators`; no CGO deps added |
| 2 | II. Redis-first before PostgreSQL | PASS | Rolling headline-cluster state and parameter snapshots cached in Redis; Postgres persists durable signal/outcome records only |
| 3 | III. Bun + React 18 TS PWA | PASS | Commodities section and summary panel built in `web/` under Bun; PWA/SSE untouched |
| 4 | IV. No Iranian-market dependencies | PASS | N/A — global markets only |
| 5 | V. OpenAI-compatible AI + adaptive weights | PASS | AI direction call retained; gates/sizing wrap it; prompt gains clustered-catalyst payload; Thompson weights untouched |
| 6 | VI. Test-first verification shield | PASS | Plan mandates unit + integration + replay-simulation tests before each gate merges (tasks derived from FRs) |
| 7 | VII. Spec-driven single-command deployment | PASS | This pipeline; deploy via existing CI/Compose path used in 011-vps-deployment |
| 8 | VIII. Evidence-based risk parameters (NON-NEGOTIABLE) | PASS | Core of the feature: every param from measured distributions, validated by 30-day replay simulation with reported expectancy before enablement; true PnL persisted (FR-016/017) |

**Post-design re-check**: PASS — no violations, no Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/012-trading-system-optimization/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── trader/              # signals.go (entry gates, SL/TP/leverage/size), futures_math.go, allocator.go
├── indicators/          # ATR/NATR, VWAP, volume, regime — extended for gate inputs
├── market/              # news_crawler.go, news_sentiment.go (clustering, decay, veto), assets.go, screener.go
├── ai/                  # client.go prompt payload (clustered catalyst), types.go
├── server/              # signal_handlers.go (evaluation flow, commodity profile, filter log)
├── db/                  # migrations: new columns/tables; signal_store.go writes
└── telegram/            # unchanged (silent sends shipped in 75c623e)

web/src/
├── components/          # entry-filter log panel, commodities section, performance summary
├── pages/               # Commodities view routing
└── utils/time.ts        # reuse display-timezone formatters
```

**Structure Decision**: Option 2 (backend + frontend) matching the existing repository layout — no new top-level projects; commodities UI lives alongside current views; backend changes stay inside existing `internal/` packages.

## Complexity Tracking

No constitution violations — table intentionally empty.
