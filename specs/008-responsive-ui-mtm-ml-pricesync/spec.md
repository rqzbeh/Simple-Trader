# Feature Specification: Responsive UI, Dynamic Mark-to-Market, ML Integration, and Price Synchronization

**Feature Branch / Spec ID**: `008-responsive-ui-mtm-ml-pricesync`  
**Status**: APPROVED & READY FOR IMPLEMENTATION  
**Target Date**: 2026-09-21  

---

## 1. Executive Summary & Problem Statement

Simple-Trader's current frontend and backend exhibit several critical UI, valuation, and synchronization issues:
1. **Responsive Viewport & Mobile UI Inaccessibility**:
   - The desktop layout doesn't fit standard viewport heights cleanly and suffers from clipping/overflow.
   - On mobile viewports (< 640px), the top navigation tabs are hidden (`hidden sm:flex`) with no mobile drawer or alternative navigation menu, making sections (Investor Ledger, Liquid Screener, News Trading, Weights/ML) completely inaccessible.
2. **Static Mock Data & Static Return Badges**:
   - The portfolio summary header hardcodes `+2.45% All-Time` as a static string literal, remaining fixed even as live prices move.
   - Initial portfolio calculations anchor to synthetic baselines rather than dynamically reflecting cash + mark-to-market positions from live feeds.
3. **Machine Learning Pipeline Disconnection**:
   - `MLTrainingView.tsx` was implemented for GPU/statistical training with `/api/v1/ml/train`, `/api/v1/ml/status`, and `/api/v1/ml/runs`, but is not exposed or accessible in `App.tsx`.
   - The autonomous `trader.TradingDaemon` is instantiated in tests but omitted from `cmd/trader/main.go`, meaning autonomous background tick evaluation doesn't trigger in live daemon runs.
4. **Asset Price Desynchronization**:
   - BTC/USD is quoted at current market rates in ticker feeds (or around $60k-$90k depending on simulated/live stream), but signal handlers in `internal/server/signal_handlers.go` and `internal/server/server.go` had hardcoded fallback entry prices of $65,000 when Redis ticker quotes are absent.
   - Generated AI signals and ticker prices must synchronize from the active live price feed.

---

## 2. User Stories & Acceptance Criteria

### User Story 1: Fully Accessible Responsive & Mobile Navigation
- **As a** mobile or desktop trader,
- **I want** the UI to fit within my browser viewport cleanly and provide full mobile navigation,
- **So that** I can switch between Terminal, Investor Ledger, Liquid Screener, News Trading, and ML / Weights on any device.
- **Acceptance Criteria**:
  - Add a mobile navigation header with a hamburger toggle and dropdown/drawer menu in `web/src/App.tsx`.
  - Ensure all navigation items (`terminal`, `investors`, `screener`, `news`, `ai_weights`, `ml`) are accessible on screens < 640px.
  - Fix container max-widths and responsive padding so desktop and mobile views display cleanly without broken overflow.

### User Story 2: True Dynamic Mark-to-Market Portfolio Valuation
- **As an** investor/trader,
- **I want** the portfolio equity, total profit, and all-time return percentage to update dynamically in real time from live ticks,
- **So that** returns accurately reflect price movements without static hardcoded text.
- **Acceptance Criteria**:
  - In `web/src/App.tsx`, calculate total return dynamically:
    `const returnPct = initialCapital > 0 ? ((summary.totalEquity - initialCapital) / initialCapital) * 100 : 0;`
  - Display the calculated return percentage with sign formatting (`+` or `-`) and appropriate color (`text-emerald-500` or `text-rose-500`).
  - In `web/src/hooks/useSSE.ts`, ensure mark-to-market valuations calculate based on live asset ticks and open positions rather than static mock baselines.

### User Story 3: Integrated ML Engine & Autonomous Daemon
- **As an** operator,
- **I want** the Deep Learning / ML training interface accessible in the UI, and the autonomous trading daemon actively evaluating market ticks,
- **So that** models can be trained and signals generated continuously from live data.
- **Acceptance Criteria**:
  - Expose `MLTrainingView` within the UI (either as a dedicated tab or within an integrated AI/ML workspace) with full access to `/api/v1/ml/status`, `/api/v1/ml/train`, and `/api/v1/ml/runs`.
  - Wire `trader.NewTradingDaemon` in `cmd/trader/main.go` and connect it to process incoming price ticks.

### User Story 4: Synchronized Price Ticker and AI Trade Signals
- **As an** algorithmic trader,
- **I want** generated AI trade signals to use the current live market price from the market feed / quote cache,
- **So that** entry prices in trade signals match the current live ticker without contradictions.
- **Acceptance Criteria**:
  - In `internal/server/signal_handlers.go`, retrieve the latest price from the active `marketFeed` / quote cache instead of falling back to static 65000.0.
  - In `internal/server/server.go`, ensure backtest and signal entry prices match actual market feeds.

### User Story 5: Pure Crypto Universe (USDT/USDC Pairs) & Leverage Support
- **As a** crypto-first trader,
- **I want** all tradeable assets to be based purely on crypto markets (USDT/USDC pairs), with equivalent crypto tokens for commodities (e.g., PAXG/USDT for Gold, XAG/USDT for Silver, OIL/USDT for Oil, and EURC/USDC for currencies), and trade suggestions and open positions clearly indicating recommended and active leverage,
- **So that** the entire ecosystem operates natively on crypto liquidity and derivatives with transparent risk gearing.
- **Acceptance Criteria**:
  - Remove legacy Forex pairs (EUR/USD, etc.).
  - Represent commodities as crypto-native equivalents: PAXG/USDT (Gold), XAG/USDT (Silver), OIL/USDT (Crude Oil), with stablecoin currency EURC/USDC.
  - Base all pairs on USDT or USDC.
  - Add leverage support to `TradePosition` and display leverage badges (e.g. `10x Cross`, `5x Isolated`) on open positions and AI trade suggestions.

### User Story 6: Streamlined & Clean UI Experience
- **As an** operator,
- **I want** a clean, uncluttered, focused trading interface that hides internal backend telemetry,
- **So that** only relevant market information, position status, and actionable intelligence are presented.
- **Acceptance Criteria**:
  - Declutter metric cards and header badges.
  - Remove backend-only internal noise (e.g., raw debugging logs, unneeded backend telemetry cards) from the default terminal view.
  - Enhance readability and ergonomics across all device sizes.

