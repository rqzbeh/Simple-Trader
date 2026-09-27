# Specification: Production Overhaul (Jim Simons Standard) & Autonomous VPS Deployment

**Spec ID**: `009-production-overhaul-simons-standard`  
**Author**: Quantitative & Full-Stack System Architect  
**Review Status**: HARSH AUDIT COMPLETED -> SPEC APPROVED  
**Target Environment**: Local WSL2 + Production Docker on VPS `<VPS_HOST>` (Port: 22, User: root)  
**Date**: 2026-09-22

---

## 1. Problem Statement & Audit Findings

Following an exhaustive audit using the `codebase-memory` MCP knowledge graph across the Go backend, React/TypeScript PWA, ML subsystem, and quantitative trading engine, the following structural flaws were confirmed:

### 1.1 Live Market Feed & SSE Pipeline Breakdown ($0 Asset Freeze)
- **Root Cause**: In `internal/market/live_feed.go`, `FetchAllLiveTicks` aggregated all 25 assets into a single Binance Spot batch request (`https://api.binance.com/api/v3/ticker/24hr?symbols=[...]`).
- **Failure Mechanism**: Binance Spot returned HTTP 400 (`-1100` illegal characters for `CL=F` and `ALI=F`, and `-1121` invalid symbol for non-spot metal contracts `XAUUSDT`, `COPPERUSDT`, `XPTUSDT`, `XPDUSDT`, `XAGUSDT`). Because the batch call failed atomically, `quotes` was empty, zero `tick` events were pushed to the SSE broadcaster, and all asset prices on the React dashboard remained permanently frozen at `$0`.

### 1.2 Asset Catalog Inconsistencies & Duplications
- **Duplicate Gold Exposure**: `internal/market/assets.go` simultaneously defined `PAXG/USDT` (tokenized gold on Binance Spot), `XAUT/USDT` (Tether Gold), and `XAU/USDT` (Gold futures). `XAUT/USDT` was an unmapped duplicate on spot that lacked conversion in `mapToYahooSymbol`.
- **25 vs. 14 Asset Discrepancy**: The frontend displays 25 assets. When the user clicked "Scan All Assets", `AISignalFeed.tsx` sent `{ bucket: 'ALPHA' }` to `POST /api/v1/signals/futures/decide-all`. `GenerateAllFuturesSignalsHandler` defaulted empty symbols to `screener.GetActiveUniverse()`, which filtered candidates with strict liquidity thresholds ($50M 24h volume, 10 bps spread), truncating the candidate set down to 14 without transparency.

### 1.3 Pervasive Hardcoded Constants & Mock Values
- `internal/trader/kelly.go`: `Fraction = 0.50`, `MinRiskPct = 0.005`, `MaxRiskPct = 0.020`, `DefaultWinRate = 0.52`, `DefaultWinLoss = 1.60`.
- `internal/trader/friction.go`: `MakerFeeRate = 0.0002`, `TakerFeeRate = 0.0005`, `ImpactFactor = 0.05`, `MaxSlippage = 0.05`.
- `internal/ai/client.go`: `fallbackHeuristic`: `lev := 8`, `SuggestedStopLossPct: 1.0`, `SuggestedTakeProfitPct: 3.0`, hardcoded sentiment thresholds `[-0.15, +0.15]`.
- `internal/server/signal_handlers.go`: `maxActiveSignals = 3`, `minBuffer = 0.05`, `timeout = 12 * time.Second`.
- `internal/config/config.go`: Missing `.env` configuration for minimum Risk-to-Reward ratio, slippage caps, fee structures, and Kelly parameters.

### 1.4 Quantitative Indicators
- The current indicator set lacked extreme-value and non-linear volatility estimators (Garman-Klass, Parkinson), trend efficiency estimators (Kaufman Efficiency Ratio), and volume-weighted accumulation metrics (Chaikin Money Flow).

---

## 2. Technical Architecture & Implementation Plan

### Phase 1: Reconcile Asset Catalog & Rebuild Multi-Source Feed (Task #10)
1. **Catalog Modernization** (`internal/market/assets.go`):
   - Remove duplicate `XAUT/USDT`.
   - Add high-volume liquid crypto `TON/USDT` to maintain a balanced 25-asset catalog:
     - 8 CORE Commodities (PAXG/USDT, XAU/USDT, XAG/USDT, COPPER/USDT, XPT/USDT, XPD/USDT, OIL/USDT, ALU/USDT)
     - 17 ALPHA Cryptocurrencies (BTC, ETH, SOL, BNB, XRP, DOGE, ADA, AVAX, SUI, LINK, DOT, NEAR, LTC, BCH, UNI, APT, TON)
   - Update `FeedSource`:
     - Spot Crypto & PAXG -> `"BINANCE_SPOT"`
     - Metals -> `"BINANCE_FUTURES"`
     - Oil & Aluminum -> `"YAHOO"`
2. **Multi-Source Ingestion Engine** (`internal/market/live_feed.go`):
   - Decouple `FetchAllLiveTicks`:
     - Fetch Spot Crypto in batch from `api.binance.com/api/v3/ticker/24hr`.
     - Fetch Metals in batch from `fapi.binance.com/fapi/v1/ticker/24hr`.
     - Fetch Commodities concurrently from `query1.finance.yahoo.com/v8/finance/chart/{symbol}?interval=1m&range=1d`.
   - Aggregate all quotes and stream over SSE channel.
3. **Scan All Alignment** (`internal/server/signal_handlers.go`):
   - If `len(req.Symbols) == 0`:
     - If `req.Bucket == "ALL"`, evaluate all 25 supported assets.
     - If `req.Bucket == "CORE"`, evaluate all Core commodities.
     - Otherwise (default `ALPHA`), evaluate all 17 Alpha cryptocurrencies (or all 25 if user clicked "Scan All Assets" from the UI).
   - In `AISignalFeed.tsx`, update payload to pass `bucket: 'ALL'` for "Scan All Assets", scanning all 25 assets.

### Phase 2: Implement Institutional Quantitative Indicators (Task #14)
1. In `internal/indicators/`:
   - `garman_klass.go`: Implement Garman-Klass and Parkinson historical volatility estimators.
   - `kaufman.go`: Implement Kaufman Efficiency Ratio (KER).
   - `cmf.go`: Implement Chaikin Money Flow (CMF 20).
2. Wire new indicators into `Snapshot` and `confluence.go`.

### Phase 3: Dynamic Configuration & Elimination of Hardcoded Values (Task #22)
1. In `internal/config/config.go`:
   - Add `.env` fields:
     - `MIN_RISK_TO_REWARD_RATIO` (default `1.5`)
     - `KELLY_FRACTION` (default `0.50`)
     - `MIN_RISK_PER_TRADE_PCT` (default `0.005`)
     - `MAX_RISK_PER_TRADE_PCT` (default `0.020`)
     - `MAX_CONCURRENT_SIGNALS` (default `5`)
     - `MAKER_FEE_RATE` (default `0.0002`)
     - `TAKER_FEE_RATE` (default `0.0005`)
     - `MAX_SLIPPAGE_PCT` (default `0.02`)
2. In `internal/trader/kelly.go` and `internal/trader/friction.go`:
   - Pass configuration dynamically instead of using package constants.
3. In `internal/ai/client.go`:
   - Calculate Stop Loss and Take Profit dynamically using Normalized ATR (NATR) and the configured minimum Risk-to-Reward ratio.

### Phase 4: Modernize React/TypeScript Frontend (Task #16)
1. Synchronize `web/src/hooks/useSSE.ts` with the 25 assets.
2. Initialize asset list with real prices by calling `GET /api/v1/assets` which provides latest cached ticker prices.
3. Update `web/src/components/AISignalFeed.tsx` to handle scanning all 25 assets.

### Phase 5: Test Hardening, Docker Testing & VPS Deployment (Tasks #24, #26)
1. Run backend unit tests: `go test -v ./...`.
2. Run frontend tests: `cd web && npm test`.
3. Verify Docker build: `docker compose build`.
4. Deploy to `<VPS_HOST>` via SSH with automated health verification.
