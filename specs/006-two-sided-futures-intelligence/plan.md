# Implementation Plan: Two-Sided Futures Trading, Dynamic Macro Allocation, News-Catalyst Alpha, Auth Security, Telegram Signals Bot, and Real-Data ML Training

**Branch**: `006-two-sided-futures-intelligence` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-two-sided-futures-intelligence/spec.md`

## Summary

Implement an advanced quantitative, security-hardened futures trading framework across backend Go services and React 18 PWA frontend:
1. **Secure Web Authentication & Data Protection**: Dedicated login modal with bcrypt password verification, signed session tokens, rate limiting against brute force, and AES-GCM-256 encryption at rest for sensitive API keys and bot tokens in PostgreSQL.
2. **Two-Sided Futures Trading**: Generate both `LONG` and `SHORT` trade recommendations and execution commands on futures pairs with leverage (1x-10x), strict stop loss (SL), take profit (TP), minimum 1:1.5 (or 1:2.0) Risk-to-Reward (R:R), and strict $\le 2.0\%$ single-trade equity risk limits.
3. **News-Catalyst Priority Architecture**: Macroeconomic/geopolitical breaking news and whale alerts serve as the primary entry justification for short-term trades, while technical indicators (RSI, SuperTrend, MACD, Bollinger Bands, CVD, OBI) strictly calculate entry prices, leverage, SL, and TP.
4. **Real-World Dynamic Macro Allocation**: Dynamically calculate 3-tier portfolio allocation (Tier 1 Cash Buffer, Tier 2 Tactical Alpha, Tier 3 Core Preservation) based on real geopolitical risk indices, central bank interest rate bias, and inflation metrics rather than static splits.
5. **Telegram Signals Bot**: Real-time broadcast of structured entry signals (`Symbol`, `LONG`/`SHORT`, `Entry`, `SL`, `TP`, `Leverage`, `Allocation`, `R:R`) and automated closure messages reporting final exit price, hold duration, realized PnL, and ROI %.
6. **Real-Data Machine Learning Training**: Train backend scoring and Bayesian weights strictly on authentic historical Binance OHLCV candles and real news archives, prohibiting synthetic mockup data.
7. **PWA Install Prompt & iOS Safari Guide**: Automatic `beforeinstallprompt` banner for Chromium/Android and an interactive step-by-step modal guide for iOS Safari users.

## Technical Context

**Language/Version**: Go 1.24 (`CGO_ENABLED=0` pure Go backend), TypeScript 5.5+ on Bun 1.3+ (React 18 PWA)

**Primary Dependencies**:
- Backend: Standard Go libraries (`crypto/aes`, `crypto/cipher`, `crypto/rand`), `golang.org/x/crypto/bcrypt`, `github.com/lib/pq`, `github.com/redis/go-redis/v9`
- Frontend: React 18, Bun, Lucide React, Tailwind CSS, TradingView Lightweight Charts

**Storage**:
- PostgreSQL 16: System users (`admin_users`), encrypted credentials, dynamic macro snapshots (`macro_regimes`), signals audit log (`trade_signals`), closed trades (`trade_executions`), ML training runs (`ml_training_runs`)
- Redis 7: Rate-limiting buckets, active session tokens, real-time tick caches, and Telegram dispatch queues

**Testing**:
- Backend: `go test -v -race ./...`
- Frontend: `bun test` and `bun run build`
- End-to-end integration: `python3 scripts/verify_e2e_pipeline.py`

**Target Platform**: Linux (Host-managed Nginx reverse proxy + Docker Compose backend on Ubuntu 24.04 / WSL2), Modern Web Browsers & Mobile iOS/Android PWA

**Project Type**: Full-stack autonomous quantitative trading daemon + Progressive Web Application

**Performance Goals**:
- Auth verification latency $< 100\text{ms}$ with bcrypt work factor 12
- Two-sided signal evaluation $< 10\text{ms}$ per candle bar
- Telegram signal delivery $< 2.0\text{s}$ from signal generation
- PWA install prompt / iOS guidance visible within $3\text{s}$ of visit

**Constraints**:
- Zero CGO dependencies (`CGO_ENABLED=0`)
- Maximum single-trade risk capped strictly at $\le 2.0\%$ portfolio equity
- Minimum Risk-to-Reward ratio enforced at $R:R \ge 1.5$ (default target $2.0$)
- Strictly zero synthetic or mock data during ML training routines

**Scale/Scope**:
- Continuous 24/7 autonomous monitoring of screened dynamic crypto universe
- Real-time Telegram alert delivery with queue retry mechanisms
- Mobile-first standalone PWA capability on iOS and Android

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. Zero-CGO Go Backend** | PASS | All authentication, cryptography (AES-GCM-256), indicators, and two-sided futures sizing implemented in standard pure Go 1.24. |
| **II. Redis Cache & Pub/Sub First** | PASS | Auth token blacklisting/caching, signal dispatch queue, and rate limiting run through Redis 7 before PostgreSQL. |
| **III. Bun Runtime & React 18 PWA** | PASS | Frontend managed strictly via Bun 1.3+, implementing native PWA install banners and iOS Safari guides. |
| **IV. Complete Purge of Iranian Market** | PASS | Operates exclusively on global liquid commodities (`XAU/USD`, `XAG/USD`) and dynamic crypto pairs. |
| **V. Unified AI Intelligence** | PASS | OmniRoute Gateway integration with high reasoning; news headlines act as primary entry catalyst while indicators handle execution parameters. |
| **VI. Test-Driven Verification** | PASS | Comprehensive unit and integration tests across auth, two-sided execution, dynamic macro allocator, and Telegram bot. |
| **VII. Spec-Driven Deployment** | PASS | Deterministic deployment with host-managed Nginx and Docker Compose stack. |

## Project Structure

### Documentation (this feature)

```text
specs/006-two-sided-futures-intelligence/
├── plan.md              # This plan
├── research.md          # Phase 0 architectural & algorithmic research
├── data-model.md        # Phase 1 data entities, schemas, and state transitions
├── quickstart.md        # Phase 1 verification and run guide
├── contracts/           # Phase 1 OpenAPI/REST specifications
│   └── openapi.yaml
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 implementation task breakdown
```

### Source Code Layout

```text
Simple-Trader/
├── internal/
│   ├── auth/                 # Admin authentication, bcrypt hashing, AES-GCM encryption, rate limiting
│   │   ├── auth.go
│   │   ├── crypto.go
│   │   ├── middleware.go
│   │   └── auth_test.go
│   ├── trader/
│   │   ├── allocator.go      # Dynamic real-world macro allocator
│   │   ├── engine.go         # Two-sided LONG/SHORT futures execution engine
│   │   ├── risk.go           # Risk-to-Reward & Kelly capital allocation
│   │   └── engine_test.go
│   ├── telegram/             # Telegram Bot API signal broadcaster and ROI reporting
│   │   ├── bot.go
│   │   ├── formatter.go
│   │   └── bot_test.go
│   ├── ai/
│   │   ├── training.go       # Real-data historical ML training pipeline
│   │   ├── client.go         # OmniRoute Gateway client
│   │   └── training_test.go
│   ├── server/
│   │   ├── server.go         # HTTP router with auth middleware
│   │   ├── handlers.go       # Auth, signals, macro, telegram, ML endpoints
│   │   └── server_test.go
│   └── db/
│       ├── migrations/
│       │   ├── 000003_futures_auth_telegram.up.sql
│       │   └── 000003_futures_auth_telegram.down.sql
│       ├── auth_store.go
│       └── signal_store.go
├── web/
│   ├── src/
│   │   ├── components/
│   │   │   ├── LoginModal.tsx        # Password login modal
│   │   │   ├── PWAInstallBanner.tsx  # Chromium install & iOS Safari guide
│   │   │   ├── SignalsFeed.tsx       # Two-sided LONG/SHORT signals card
│   │   │   ├── MacroAllocation.tsx   # Dynamic macro regime gauges
│   │   │   └── TelegramConfig.tsx    # Telegram bot integration settings
│   │   └── App.tsx
│   └── vite.config.ts
└── scripts/
    └── verify_e2e_pipeline.py
```

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | Fully complies with all constitutional principles. |
