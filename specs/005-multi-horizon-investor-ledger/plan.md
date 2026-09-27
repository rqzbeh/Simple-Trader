# Implementation Plan: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Branch**: `005-multi-horizon-investor-ledger` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-multi-horizon-investor-ledger/spec.md`

## Summary

Implement an institutional-grade multi-horizon liquidity and capital management architecture consisting of:
1. **Investor Capital Ledger**: Administrative non-login tracking of investor deposits, withdrawals, equity attribution, and real-time ROI based on unitized Net Asset Value (NAV) accounting in PostgreSQL 16 and Redis 7.
2. **3-Tier Liquidity Allocation**: Splitting portfolio equity into Tier 1 (15% Cash/Withdrawal reserve buffer with zero market slippage), Tier 2 (45% Core wealth preservation commodities `XAU/USD`, `XAG/USD`), and Tier 3 (40% Tactical Alpha multi-horizon trading).
3. **Multi-Horizon 3-Hour Swing Execution**: Aggregating ticks into 3-hour candles with dedicated ATR/SuperTrend/RSI indicators and automated profit sweeping back into Tier 1 cash.
4. **Autonomous News Ingestion & Sentiment**: Continuous RSS/REST crawling (CryptoPanic, Yahoo Finance, ForexFactory) with SHA-256 deduplication and hyperbolic tangent sentiment scoring incorporated into trade confluence decisions.
5. **Dynamic Liquid Crypto Screener**: Periodic screening of liquid crypto pairs filtering for >$50M 24h turnover and <10 bps spread.
6. **PWA Dashboard**: React 18 + Bun UI tab for fund operators with investor creation, transaction logging, live metrics, and news sentiment telemetry.

## Technical Context

**Language/Version**: Go 1.24 (`CGO_ENABLED=0` pure Go backend), TypeScript 5.5+ on Bun 1.3+ (Frontend)

**Primary Dependencies**:
- Backend: Standard Go libraries, `github.com/lib/pq` (PostgreSQL driver), `github.com/redis/go-redis/v9` (Redis 7 client)
- Frontend: React 18, Lucide React, Tailwind CSS, TradingView Lightweight Charts (via Bun)

**Storage**:
- PostgreSQL 16: Durable relational storage for `investors`, `investor_transactions`, `portfolio_snapshots`, `news_articles`
- Redis 7: High-frequency caching for live NAV, investor summary stats, screened crypto universe, and SSE streaming

**Testing**:
- Backend: `go test -v -race ./...`
- Frontend: `bun test` and `bun run build`

**Target Platform**: Linux (Docker Compose container deployment on Ubuntu 24.04 / WSL2), Modern Web Browsers (PWA)

**Project Type**: Full-stack financial trading daemon + progressive web application

**Performance Goals**:
- Sub-5ms 3-hour candlestick indicator calculation
- Sub-50ms investor NAV and pro-rata balance recalculation
- Sub-60s live headline ingestion, deduplication, and sentiment attribution
- 100% slippage-free withdrawal fulfillment within the 15% Tier 1 buffer

**Constraints**:
- Zero CGO dependencies (`CGO_ENABLED=0`)
- Immutable financial transaction audit log in PostgreSQL
- Strict non-login requirement for investor records (internal admin management)
- Zero reliance on deprecated legacy Iranian bourse modules

**Scale/Scope**:
- Multi-asset trading engine handling 10-50 concurrent assets
- Hundreds of investor profiles and thousands of capital ledger transactions
- Ingestion of hundreds of daily news articles

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. Zero-CGO Go Backend** | PASS | Pure Go 1.24 implementation for all allocator math, indicator calculations, news crawling, and screener modules. |
| **II. Redis Cache & Pub/Sub First** | PASS | Live NAV, investor cache, and news sentiment cached in Redis 7; audit ledger persisted in PostgreSQL 16. |
| **III. Bun Runtime & React 18 PWA** | PASS | All UI updates built with Bun 1.3+ and React 18 TypeScript; zero Node.js runtime dependencies. |
| **IV. Complete Purge of Iranian Market** | PASS | Strictly operates on global Core commodities (`XAU/USD`, `XAG/USD`) and dynamic liquid crypto universe (`BTC`, `ETH`, `SOL`, `AVAX`, `LINK`, etc.). |
| **V. Unified AI Intelligence** | PASS | News sentiment scores dynamically augment or dampen AI confidence scores and trade confluence. |
| **VI. Test-Driven Verification** | PASS | Comprehensive test suites for allocator, investor ledger, news crawler, and screener. |
| **VII. Spec-Driven Deployment** | PASS | Unified Docker Compose deployment and reproducible migrations. |

## Project Structure

### Documentation (this feature)

```text
specs/005-multi-horizon-investor-ledger/
├── plan.md              # This plan
├── research.md          # Phase 0 architectural decisions
├── data-model.md        # Phase 1 data entities and DB schemas
├── quickstart.md        # Phase 1 validation scenarios
├── contracts/           # Phase 1 REST OpenAPI contracts
│   └── openapi.yaml
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 work decomposition (via speckit-tasks)
```

### Source Code Layout

```text
Simple-Trader/
├── internal/
│   ├── db/
│   │   ├── migrations/
│   │   │   ├── 000002_investor_ledger.up.sql
│   │   │   └── 000002_investor_ledger.down.sql
│   │   ├── investor.go       # Investor CRUD & ledger transactions
│   │   └── postgres.go
│   ├── cache/
│   │   ├── redis.go
│   │   └── types.go
│   ├── trader/
│   │   ├── allocator.go      # 3-Tier Liquidity Allocator (Tier 1 Cash, Tier 2 Core, Tier 3 Alpha)
│   │   ├── daemon.go         # Multi-horizon 3-hour swing & macro execution loop
│   │   └── kelly.go
│   ├── market/
│   │   ├── news_sentiment.go # Tanh sentiment scoring & lexicon
│   │   ├── news_crawler.go   # Background RSS/API news collector & SHA-256 dedup
│   │   ├── screener.go       # Dynamic liquid crypto universe filter
│   │   └── indicators.go     # 3-hour bar aggregator & indicator calculations
│   └── server/
│       ├── server.go         # REST endpoints for investors, transactions, news, screener
│       └── sse.go            # Real-time event streaming
├── web/
│   ├── src/
│   │   ├── components/
│   │   │   ├── InvestorLedgerView.tsx  # Investor accounting dashboard & modals
│   │   │   ├── NewsStreamView.tsx      # Real-time news & sentiment telemetry
│   │   │   └── ScreenerView.tsx        # Liquid crypto universe monitor
│   │   ├── App.tsx                     # Main layout & navigation tabs
│   │   └── types.ts                    # TypeScript types for ledger & screener
└── docker-compose.yml
```

**Structure Decision**: Web application layout integrating into existing `internal/` Go modules and `web/` Bun React application, adhering strictly to Constitution principles.

## Complexity Tracking

*No constitution violations. All designs adhere directly to core principles.*
