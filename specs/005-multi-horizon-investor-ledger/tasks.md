# Tasks: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Input**: Design documents from `/specs/005-multi-horizon-investor-ledger/`  
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/openapi.yaml`, `quickstart.md`

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (independent files)
- **[Story]**: Story mapping (`[US1]`, `[US2]`, `[US3]`, `[US4]`, `[US5]`)

---

## Phase 1: Setup & Migrations

**Purpose**: Database migrations and shared foundation for relational persistence and caching

- [x] T001 Create PostgreSQL database migration in `internal/db/migrations/000002_investor_ledger.up.sql` with tables: `investors`, `investor_transactions`, `portfolio_nav_history`, `news_articles`, `crypto_screener_snapshots`
- [x] T002 Create rollback migration in `internal/db/migrations/000002_investor_ledger.down.sql`
- [x] T003 Update DB schema migration runner in `internal/db/postgres.go` to execute `000002_investor_ledger.up.sql` idempotently

---

## Phase 2: Foundational (Data Access & Core Domain Types)

**Purpose**: Data access layer and shared structs that all user stories depend on

- [x] T004 [P] Define core investor, transaction, and 3-tier allocation structs in `internal/db/investor_types.go`
- [x] T005 Implement PostgreSQL CRUD and transaction methods for investors and ledger transactions in `internal/db/investor.go`
- [x] T006 [P] Add Redis cache serialization and pub/sub helpers for live pool NAV and investor metrics in `internal/cache/investor_cache.go`

---

## Phase 3: User Story 1 - Investor Capital Ledger & Non-Login Tracking (Priority: P1) 🎯 MVP

**Goal**: Fund operators can manage investor records, deposit/withdrawal flows, and view pro-rata ROI/equity dynamically calculated via unitized NAV accounting without requiring user logins.

**Independent Test**: Register an investor with $10,000, verify initial balance and 10,000 pool units at NAV 1.0, record a $2,000 withdrawal, and confirm units decrease proportionally with exact mathematical ledger reconciliation.

### Tests for User Story 1
- [x] T007 [P] [US1] Unit tests for investor NAV calculation, share issuance, and ROI attribution in `internal/db/investor_test.go`
- [x] T008 [P] [US1] API endpoint integration tests for investor registration, deposits, and withdrawals in `internal/server/investor_test.go`

### Implementation for User Story 1
- [x] T009 [US1] Implement unitized Net Asset Value (NAV) share calculation logic in `internal/db/investor.go`
- [x] T010 [US1] Add REST API handlers `GET /api/v1/investors`, `POST /api/v1/investors`, `GET /api/v1/investors/{id}`, and `POST /api/v1/investors/{id}/deposit` in `internal/server/server.go`
- [x] T011 [US1] Create frontend investor management dashboard with deposit modal in `web/src/components/InvestorLedgerView.tsx`
- [x] T012 [US1] Integrate Investor Ledger tab into main navigation in `web/src/App.tsx`

---

## Phase 4: User Story 2 - Three-Tier Liquidity Allocation & Withdrawal Reserve (Priority: P1)

**Goal**: Divide portfolio equity across Tier 1 (15% Cash Buffer for slippage-free withdrawals), Tier 2 (45% Core Gold/Silver), and Tier 3 (40% Tactical Alpha), with instant withdrawal settlement from Tier 1.

**Independent Test**: Request a withdrawal within the Tier 1 cash buffer; verify cash buffer is debited instantly with zero disruption to open positions. Request a withdrawal exceeding available buffer; verify graceful error handling.

### Tests for User Story 2
- [x] T013 [P] [US2] Unit tests for 3-tier target allocation ratios and liquidity buffer checks in `internal/trader/allocator_test.go`

### Implementation for User Story 2
- [x] T014 [US2] Refactor `internal/trader/allocator.go` to support 3 tiers: `Tier 1 Cash` (target 15%), `Tier 2 Core` (target 45%), `Tier 3 Tactical Alpha` (target 40%)
- [x] T015 [US2] Implement withdrawal handler `POST /api/v1/investors/{id}/withdraw` in `internal/server/server.go` enforcing Tier 1 liquidity reserve limits
- [x] T016 [US2] Add `GET /api/v1/allocator/tiers` endpoint in `internal/server/server.go`
- [x] T017 [US2] Add 3-tier liquidity breakdown gauges and withdrawal modal in `web/src/components/InvestorLedgerView.tsx`

---

## Phase 5: User Story 3 - Multi-Horizon Trading with 3-Hour Swing Timeframe (Priority: P2)

**Goal**: Aggregate market ticks into 3-hour candlestick bars, compute technical indicators on the 3-hour horizon, and sweep tactical profits into Tier 1 cash reserve.

**Independent Test**: Ingest synthetic ticks spanning a 3-hour window; verify that completed 3-hour bars are generated and tactical swing indicators (RSI, ATR, SuperTrend) execute in under 5ms.

### Tests for User Story 3
- [x] T018 [P] [US3] Unit tests for 3-hour candlestick bar aggregation and indicator calculations in `internal/market/aggregator_test.go`

### Implementation for User Story 3
- [x] T019 [US3] Implement `CandleAggregator` in `internal/market/aggregator.go` synthesizing 3-hour bars from real-time quotes
- [x] T020 [US3] Update trading daemon in `internal/trader/daemon.go` to run tactical 3-hour swing strategies concurrently with macro trend strategies
- [x] T021 [US3] Implement automated profit-sweep mechanism in `internal/trader/allocator.go` transferring realized tactical profits into Tier 1 Cash

---

## Phase 6: User Story 4 - Live News Trading Ingestion & Sentiment Circuit (Priority: P2)

**Goal**: Continuously crawl market news feeds (CryptoPanic, Yahoo Finance, ForexFactory), deduplicate with SHA-256 hashes, score headlines with hyperbolic tangent lexicon NLP, and incorporate sentiment into trade confluence scoring.

**Independent Test**: Ingest sample RSS headlines with negative keywords; verify deduplication and ensure sentiment polarity registers as `BEARISH` with tactical buy signals dampened.

### Tests for User Story 4
- [x] T022 [P] [US4] Unit tests for news crawler, SHA-256 deduplication, and sentiment scoring in `internal/market/news_crawler_test.go`

### Implementation for User Story 4
- [x] T023 [US4] Implement background news crawler in `internal/market/news_crawler.go` ingesting public RSS feeds with SHA-256 title deduplication
- [x] T024 [US4] Integrate news sentiment polarity into trade confluence scoring in `internal/market/news_sentiment.go` and `internal/trader/daemon.go`
- [x] T025 [US4] Expose `GET /api/v1/news/stream` in `internal/server/server.go`
- [x] T026 [US4] Create live news stream widget with sentiment polarity badges in `web/src/components/NewsStreamView.tsx`

---

## Phase 7: User Story 5 - Dynamic Liquid Crypto Screener (Priority: P3)

**Goal**: Dynamically screen candidate crypto pairs to admit only assets with >$50M 24h turnover and <10 bps spread, freezing assets that deteriorate.

**Independent Test**: Evaluate a candidate universe; confirm that high-volume liquid pairs pass admission while low-volume or wide-spread tokens are rejected.

### Tests for User Story 5
- [x] T027 [P] [US5] Unit tests for crypto screener volume and spread filtering in `internal/market/screener_test.go`

### Implementation for User Story 5
- [x] T028 [US5] Implement `CryptoScreener` in `internal/market/screener.go` filtering candidates by volume and spread
- [x] T029 [US5] Expose `GET /api/v1/market/screener` in `internal/server/server.go`
- [x] T030 [US5] Implement dynamic crypto universe dashboard widget in `web/src/components/ScreenerView.tsx`

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Integration verification, test suite execution, container build, and deployment

- [x] T031 Run complete Go test suite with race detector: `CGO_ENABLED=0 go test -v -race ./...`
- [x] T032 Run frontend test and production bundle build: `cd web && bun test && bun run build`
- [x] T033 Execute end-to-end validation scenarios from `quickstart.md`
- [x] T034 Verify Docker Compose container build and run: `docker compose build`
