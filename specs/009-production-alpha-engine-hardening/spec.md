# Feature Specification: Production Alpha Engine Hardening & Autonomous Deployment

**Feature Branch / Spec ID**: `009-production-alpha-engine-hardening`  
**Status**: APPROVED & IN IMPLEMENTATION  
**Target Date**: 2026-09-22  
**Target Host**: `<VPS_HOST>` (Port: 22, User: root)

---

## 1. Executive Summary & Root Cause Analysis

A rigorous, multi-lens audit of the Simple-Trader codebase (`/home/redsnow/Simple-Trader`) adhering to the highest quantitative finance (Jim Simons standard) and full-stack engineering criteria revealed critical architectural defects:

1. **Exchange Routing & Broken SSE Pipeline ($0 Asset Prices)**:
   - In `internal/market/live_feed.go`, `FetchAllLiveTicks` batched all 25 assets into a single Binance Spot REST query (`/api/v3/ticker/24hr?symbols=[...]`).
   - Because commodities (`OIL/USDT` with `CL=F`, `ALU/USDT` with `ALI=F`) and metals (`XAU/USDT`, `XAG/USDT`, `COPPER/USDT`, `XPT/USDT`, `XPD/USDT`) are not Binance Spot pairs, Binance rejected the entire batch with HTTP 400 (errors `-1100` and `-1121`).
   - Consequently, zero ticker events were emitted over Server-Sent Events (`/api/v1/events`), leaving all assets permanently frozen at $0 on the frontend dashboard.

2. **Gold Asset Duplication & Universe Inconsistency**:
   - `internal/market/assets.go` simultaneously registered `PAXG/USDT`, `XAUT/USDT` (Tether Gold), and `XAU/USDT` (Gold/Tether). `XAUT/USDT` duplicated gold exposure while lacking exchange mapping in `internal/market/yahoo.go`.
   - `internal/server/signal_handlers.go:324-326` in `GenerateAllFuturesSignalsHandler` defaulted empty symbol queries to `screener.GetActiveUniverse()`, which filtered candidates with strict liquidity thresholds ($50M 24h volume, 10 bps spread). This filtered the candidate set down to 14, causing the "Scan All Assets" button to report scanning only 14 assets despite 25 being displayed on the dashboard.

3. **Pervasive Hardcoded Constants & Mock Data**:
   - Risk parameters: `kelly.Fraction = 0.50`, `MinRiskPct = 0.005`, `MaxRiskPct = 0.020`, `DefaultWinRate = 0.52`, `DefaultWinLoss = 1.60` were hardcoded in `internal/trader/kelly.go`.
   - Fee and friction models: Maker fee (2 bps), taker fee (5 bps), impact factor (0.05), maximum slippage cap (5%), simulated depth (100.0) were hardcoded in `internal/trader/friction.go`.
   - Signal constraints: Max concurrent signals (3), liquidity buffer (5%), evaluation timeout (12s), AI fallback leverage (8x), fallback SL (1.0%), fallback TP (3.0%), and baseline weights (`RSI: 1.15`, `MACD: 1.20`, `SUPERTREND: 1.45`, `MICROSTRUCTURE: 1.50`) were hardcoded in source files rather than read from configuration or dynamically computed from market regime and volatility.
   - Frontend initial state in `web/src/hooks/useSSE.ts` hardcoded 25 static asset stubs with $0 prices.

4. **Institutional Machine Learning & Indicator Enhancements**:
   - Modern quantitative indicators (Garman-Klass volatility, Parkinson volatility, Kaufman Efficiency Ratio, Chaikin Money Flow) need to be natively computed and integrated into the Go backend.
   - GPU-trained PyTorch models on the RTX 2060 must be connected to the Bayesian attribution and signal inference pipelines.

---

## 2. User Stories & Acceptance Criteria

### User Story 1: Multiplexed Market Feed & Reliable SSE Streaming
- **As a** platform operator and trader,
- **I want** the market feed to correctly multiplex Binance Spot, Binance Futures, and Yahoo Finance according to asset type,
- **So that** all 25 assets stream live real-time prices over SSE without $0 freezes or HTTP 400 errors.
- **Acceptance Criteria**:
  - `internal/market/live_feed.go` segments assets into:
    1. Binance Spot (`api.binance.com/api/v3/ticker/24hr`): Spot crypto and tokenized commodities (`PAXGUSDT`).
    2. Binance Futures (`fapi.binance.com/fapi/v1/ticker/24hr`): Metals contracts (`XAUUSDT`, `XAGUSDT`, `COPPERUSDT`, `XPTUSDT`, `XPDUSDT`).
    3. Yahoo Finance (`query1.finance.yahoo.com/v8/finance/chart`): Energy and industrial commodities (`CL=F`, `ALI=F`).
  - `/api/v1/events` broadcasts live ticks continuously every 1 second.
  - Initial `GET /api/v1/assets` returns latest cached ticker quotes so the dashboard renders real prices immediately upon page load.

### User Story 2: Elimination of Duplicates & Asset Universe Alignment
- **As a** portfolio manager,
- **I want** a consistent 25-asset catalog without redundant gold tokens, and "Scan All" to evaluate all assets in the selected bucket,
- **So that** the universe is unified between the dashboard, screener, and AI signal engine.
- **Acceptance Criteria**:
  - Remove duplicate `XAUT/USDT` from `internal/market/assets.go`. Replace with high-volume liquid crypto `TON/USDT` to maintain a robust 25-asset universe (8 Core commodities, 17 Alpha cryptocurrencies).
  - Update `internal/server/signal_handlers.go`: when `req.Symbols` is empty, scan all assets belonging to `req.Bucket` (or all 25 if `req.Bucket == "ALL"`), ensuring "Scan All" scans the full catalog instead of arbitrarily truncating to 14.
  - Update frontend `web/src/hooks/useSSE.ts` to reflect the reconciled 25-asset catalog.

### User Story 3: Dynamic Volatility-Based Parameters & .env Integration
- **As a** quantitative risk manager,
- **I want** risk thresholds (min Risk-to-Reward ratio, Kelly fractions, stop loss, take profit, fees, slippage caps) loaded dynamically from `.env` or computed from market volatility (NATR / Garman-Klass) and regime,
- **So that** no trading constraints or risk limits are hardcoded in Go or TypeScript.
- **Acceptance Criteria**:
  - Expand `internal/config/config.go` to parse:
    - `MIN_RISK_TO_REWARD_RATIO` (default: `1.5`)
    - `KELLY_FRACTION` (default: `0.50`)
    - `MIN_RISK_PER_TRADE_PCT` (default: `0.005`)
    - `MAX_RISK_PER_TRADE_PCT` (default: `0.020`)
    - `MAX_CONCURRENT_SIGNALS` (default: `5`)
    - `MAKER_FEE_RATE` (default: `0.0002`)
    - `TAKER_FEE_RATE` (default: `0.0005`)
    - `MAX_SLIPPAGE_PCT` (default: `0.02`)
  - Calculate dynamic Stop Loss and Take Profit levels using ATR/NATR multipliers rather than static percentages.

### User Story 4: Modern Quantitative Indicators
- **As a** quantitative analyst,
- **I want** institutional volatility and microstructure indicators (Garman-Klass, Parkinson, Kaufman Efficiency Ratio, Chaikin Money Flow),
- **So that** signal generation leverages non-linear variance and noise estimation.
- **Acceptance Criteria**:
  - Implement `CalculateGarmanKlassVol`, `CalculateParkinsonVol`, `CalculateKaufmanER`, and `CalculateCMF` in `internal/indicators/`.
  - Include these metrics in the technical analysis snapshot and AI prompts.

### User Story 5: Production Verification, Test Hardening & VPS Deployment
- **As a** DevOps engineer,
- **I want** all backend and frontend unit/integration tests running clean inside Docker,
- **So that** the production container is validated, pushed to GitHub, and deployed to VPS `<VPS_HOST>`.
- **Acceptance Criteria**:
  - `go test ./...` passes with 0 failures.
  - Frontend `npm run test` and `npm run build` pass without TypeScript errors.
  - Docker Compose build succeeds.
  - Deploy to `<VPS_HOST>` over SSH, restart services, and verify health check at `/healthz`.

---

## 3. Architecture & Data Flow

```
[Binance Spot API]    --> Spot Crypto & PAXG    \
[Binance Futures API] --> Metals (XAU, XAG, etc)  --> [LiveMarketFeed] --> [SSEBroadcaster] --> EventSource (/api/v1/events) --> React UI
[Yahoo Finance API]   --> Macro Energy & Metals /         |
                                                    [Redis Cache]
                                                          |
                                               [Autonomous Evaluator]
                                                    + [AI Client]
                                                    + [Indicators (GK, KER, CMF)]
                                                          |
                                                [PostgreSQL DB Store]
```

---

## 4. Verification Plan

1. **Unit Tests**:
   - `market_test.go`: Test multiplexed ticker fetching, symbol routing, and asset finding.
   - `indicators_test.go`: Test Garman-Klass, Parkinson, Kaufman ER, and CMF calculations against known numerical vectors.
   - `config_test.go`: Test dynamic environment variable parsing and fallback defaults.
2. **Integration Tests**:
   - `signal_handlers_test.go`: Verify batch evaluation over full 25-asset universe.
   - SSE connection test: Ensure live ticks stream continuously and prices update.
3. **Container & Deployment**:
   - Docker build and container run test.
   - Remote SSH deployment to `<VPS_HOST>` with health validation.
