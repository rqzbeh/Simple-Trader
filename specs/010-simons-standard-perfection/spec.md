# Specification: Production Perfection & Jim Simons Quantitative Standard

**Feature ID**: `010-simons-standard-perfection`  
**Feature Branch**: `010-simons-standard-perfection`  
**Author**: Quantitative Systems Architect & Full-Stack Core Engineer  
**Status**: APPROVED & IN EXECUTION  
**Target Architecture**: Go 1.24, React 19/Vite PWA, PyTorch CUDA 12.4 (RTX 2060), PostgreSQL 16, Redis 7  
**Deployment Target**: VPS `<VPS_HOST>` (Port: 22, User: root, Key: `<SSH_KEY_PATH>`)  
**Date**: 2026-09-23  

---

## 1. Executive Summary & Harsh Review Findings

Following a ruthless, multi-perspective audit across Quantitative Finance, Go Concurrency, Database Engineering, React/PWA Frontend, and DevOps, eight structural defects were isolated:

1. **Asset Catalog Inconsistencies & Duplication**:
   - `XAUT/USDT` remained in comments, TradingView scanner mappings (`internal/market/multi_source.go`), and frontend dropdowns (`MLTrainingView.tsx`).
   - `FuturesSignalsView.tsx` hardcoded only 12 assets in its dropdown; `MLTrainingView.tsx` hardcoded 13 assets with duplicate gold.
   - Screener candidate filtering disqualified commodities due to a crypto-specific $50M 24h volume threshold, truncating candidate sets to 14 assets and causing "Scan All Assets" to evaluate only 14 assets instead of the full 25-asset portfolio.

2. **SSE & Live Pricing Pipeline ($0 Asset Freeze)**:
   - `internal/cache/keys.go` serialized `TickerQuote` with `json:"change_24h"`, whereas `web/src/hooks/useSSE.ts` parsed `tick.change24h`.
   - SSE connection handler (`sse.go`) only pushed an initial `ping` event without immediate state hydration of all currently cached asset quotes.
   - `INITIAL_SUMMARY` had its target equity percentages zeroed out, causing `bun test` failures.

3. **Complete Absence of Live Indicator Computation Pipeline**:
   - `internal/server/signal_handlers.go` and `internal/trader/providers.go` previously defaulted indicator values or relied on empty `cache.IndicatorSnapshot{}` structures because no live calculation pipeline existed to populate Redis via `SetIndicatorSnapshot`.
   - Indicator suite in `internal/indicators/` contained isolated calculation functions but lacked an end-to-end `BuildSnapshot` workflow taking authentic historical candles.

4. **Fake Economic Calendar Data vs. Authentic Institutional Feeds**:
   - `calendar.go` and `server.go` populated mock FOMC/CPI events with relative timestamps (`time.Now().Add(...)`).
   - Real-world high-impact macro events must be continuously scraped from authentic institutional calendar feeds (`nfs.faireconomy.media/ff_calendar_thisweek.json`) with zero mock data.

5. **Pervasive Hardcoded Constants & Risk Bounds**:
   - Kelly fraction, risk floors/ceilings, maximum concurrent signals, fee rates, slippage limits, and fallback heuristics contained hardcoded magic numbers rather than reading strictly from `.env`.

6. **Machine Learning Model Overfitting & Calibration**:
   - Prior models overfit 1h return directions (78% train vs 53% val) due to raw binary classification over noise.
   - Need volatility-adjusted, triple-barrier or trend-efficiency normalized training targets with Deep Residual MLP and Purged Walk-Forward Cross Validation on the local RTX 2060 GPU.

7. **Frontend PWA & Accessibility Hardening**:
   - Asset dropdowns must dynamically mirror the 25 supported assets.
   - Real-time mark-to-market revaluation must handle both camelCase and snake_case quote fields without throwing or dropping ticks.

8. **Deployment & DevOps Verification**:
   - Docker multi-stage builds must pass without permission errors.
   - Automated deployment to VPS `<VPS_HOST>` must follow Spec-Kit guidelines with verified health check.

---

## 2. User Scenarios & Functional Requirements

### User Story 1 - Zero-Latency Live Terminal Pricing (Priority: P1)
As a quantitative trader, I want all 25 Core and Alpha assets to display authentic live market prices immediately upon opening the dashboard and continuously stream fluctuations with zero `$0` freezes.

- **FR-001**: System MUST fetch authentic quotes for all 25 assets immediately on startup and cache them in memory and Redis.
- **FR-002**: When an SSE client connects to `/api/v1/events`, system MUST immediately stream the full initial snapshot of all 25 asset quotes.
- **FR-003**: System MUST serialize tick quotes with both `change24h` and `change_24h` backward/forward compatibility.

### User Story 2 - Comprehensive 25-Asset Signal Scanner (Priority: P1)
As a portfolio manager, when I trigger "Scan All Assets", the terminal MUST evaluate all 25 assets across Core commodities and Alpha cryptocurrencies without truncating to 14 assets.

- **FR-004**: "Scan All Assets" endpoint (`POST /api/v1/signals/futures/decide-all`) with `bucket: "ALL"` MUST scan exactly the 25 supported assets.
- **FR-005**: All UI dropdowns (`FuturesSignalsView.tsx`, `MLTrainingView.tsx`, `AssetTickerGrid.tsx`) MUST dynamically render all 25 assets without duplicate or removed symbols (`XAUT/USDT` eliminated).

### User Story 3 - Authentic Live Quantitative Indicators (Priority: P1)
As a risk manager, all technical and microstructure indicators (RSI, MACD, SuperTrend, Bollinger Bands, ATR, Garman-Klass, Parkinson, Kaufman ER, CMF, Order Book Imbalance, CVD) MUST be calculated on authentic exchange candlestick data with ZERO mock or hardcoded values.

- **FR-006**: System MUST download authentic Binance/Yahoo historical candles and compute complete `Snapshot` instances dynamically.
- **FR-007**: System MUST store computed snapshots in Redis via `SetIndicatorSnapshot` with dynamic TTL and supply them to AI and strategy evaluators.

### User Story 4 - Live Macro Calendar Feed & Autonomous Halts (Priority: P2)
As a systematic trader, trading halts around high-impact macro releases (FOMC, CPI, NFP) MUST be governed by authentic live calendar feeds rather than hardcoded mock events.

- **FR-008**: System MUST fetch real-time macroeconomic events from the verified live institutional calendar feed.
- **FR-009**: System MUST evaluate symbol halts dynamically based on event impact (`HIGH`) and asset base/quote currencies.

### User Story 5 - Dynamic Environment Configuration (Priority: P2)
As a DevOps engineer, no financial thresholds (Risk-to-Reward ratio, Kelly fractions, fees, slippage, leverage limits) may be hardcoded in Go code; all MUST be loaded from `.env`.

- **FR-010**: All risk, fee, friction, and signal parameters MUST be configurable via `.env` with sensible defaults.

### User Story 6 - GPU-Accelerated ML Maximizer (Priority: P2)
As a quantitative researcher, deep neural models MUST be trained on local RTX 2060 GPU with CUDA acceleration over stationary engineered features to achieve maximum out-of-sample prediction accuracy.

- **FR-011**: Training script MUST utilize PyTorch CUDA, CosineAnnealingWarmRestarts, and Purged Walk-Forward splits.
- **FR-012**: Best checkpoints and holdout metrics MUST be recorded to `models/` and database.

### User Story 7 - VPS Deployment & Verification (Priority: P3)
As a system administrator, the entire containerized stack MUST deploy cleanly to VPS `<VPS_HOST>` with verified health endpoints and updated documentation.

- **FR-013**: Docker Compose build MUST complete without errors.
- **FR-014**: Git repository MUST be pushed to GitHub with comprehensive `README.md`.
- **FR-015**: VPS service MUST pass `/health` check.

---

## 3. Success Criteria

- **SC-001**: 100% of unit and race tests pass (`go test -race ./...` and `bun test`).
- **SC-002**: All 25 assets display non-zero live prices on initial load and SSE stream.
- **SC-003**: "Scan All Assets" evaluates 25 assets in under 5 seconds.
- **SC-004**: Zero mockups or hardcoded indicator values remain in the Go backend.
- **SC-005**: All financial parameters loaded dynamically from `.env`.
- **SC-006**: ML model achieves validated out-of-sample edge on local GPU.
- **SC-007**: Production Docker containers run healthy on VPS `<VPS_HOST>`.
