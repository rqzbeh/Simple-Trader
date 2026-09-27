# Implementation Plan: Production Overhaul (Jim Simons Standard)

**Spec ID**: `009-production-overhaul-simons-standard`  
**Phase**: Execution  

---

## Technical Tasks Breakdown

### Task 1: Reconcile Asset Catalog & Multi-Source Live Feed
- [x] Remove duplicate `XAUT/USDT` in `internal/market/assets.go`.
- [x] Add `TON/USDT` to `SupportedAssets` (17 Alpha, 8 Core = 25 assets total).
- [x] Set explicit `FeedSource`: `BINANCE_SPOT`, `BINANCE_FUTURES`, `YAHOO`.
- [x] Re-architect `internal/market/live_feed.go`:
  - Fetch Binance Spot crypto in batch.
  - Fetch Binance Futures metals in batch.
  - Fetch Yahoo Finance commodities concurrently.
- [x] Update `internal/server/signal_handlers.go` and `web/src/components/AISignalFeed.tsx` so "Scan All Assets" evaluates the complete 25-asset universe.
- [x] Update `GET /api/v1/assets` to include latest cached prices so the dashboard never boots with $0.

### Task 2: Implement Institutional Quantitative Indicators
- [x] `internal/indicators/garman_klass.go`: Extreme-value volatility (GK) and Parkinson volatility estimators.
- [x] `internal/indicators/kaufman.go`: Kaufman Efficiency Ratio (KER).
- [x] `internal/indicators/cmf.go`: Chaikin Money Flow (CMF).
- [x] Integrate new indicators into `internal/indicators/confluence.go` and `internal/ai/client.go`.

### Task 3: Dynamic Configuration & Elimination of Hardcoded Values
- [x] Add dynamic risk, fee, and Kelly parameters to `internal/config/config.go` and `.env.example`.
- [x] Parameterize `internal/trader/kelly.go` and `internal/trader/friction.go`.
- [x] Dynamically compute Stop Loss & Take Profit from market volatility (ATR/NATR) and minimum Risk-to-Reward ratio (`MIN_RISK_TO_REWARD_RATIO`).

### Task 4: Frontend State & React Modernization
- [x] Align `web/src/hooks/useSSE.ts` with the 25 assets.
- [x] Ensure seamless fallback from REST initial prices to live SSE ticks.
- [x] Validate responsive mobile layout and MtM portfolio valuation.

### Task 5: Docker Verification, Hardening & VPS Deployment
- [x] Run `go test ./...` across all packages.
- [x] Build and test frontend (`npm run build`).
- [x] Test containerized build with Docker Compose.
- [x] Rewrite `README.md` with complete architecture and setup guide.
- [x] Deploy to `root@<VPS_HOST>` (port 22) and verify `/healthz`.
