# Feature Specification: Responsive UI, Mark-to-Market Valuation, ML Integration, and Price Sync

**Feature Branch / Spec ID**: `008-responsive-ui-mtm-ml-sync`  
**Status**: APPROVED & IN PROGRESS  
**Target Date**: 2026-09-21  

---

## 1. Executive Summary & Problem Statement

Comprehensive audit of Simple-Trader identified critical bugs spanning usability, mathematical accuracy, pipeline connectivity, and data integrity:

1. **Mobile & Viewport UI Breakage**:
   - Navigation links in `App.tsx` had `hidden sm:flex`, rendering the navigation completely inaccessible on mobile devices (`< 640px`) without any hamburger menu or drawer.
   - Desktop and tablet viewports experienced clipping or non-fluid horizontal overflow.
2. **Static Mock Data & Flawed Mark-to-Market (MtM) Calculations**:
   - The all-time performance badge displayed a hardcoded `+2.45% All-Time` string regardless of asset price movements or actual PnL.
   - Initial portfolio calculations relied on artificial static baselines (`baseCore = 61470.0`, `baseAlpha = 40980.0`) in `useSSE.ts` rather than dynamic summation of actual position valuations and cash balances.
3. **Disconnected Machine Learning & Autonomous Trading Subsystems**:
   - `trader.NewTradingDaemon` and autonomous tick-by-tick evaluation were never wired or booted in `cmd/trader/main.go`.
   - The GPU / Bayesian ML training pipeline (`/api/v1/ml/train`, `/api/v1/ml/status`, `/api/v1/ml/runs`) had an unlinked React component (`MLTrainingView.tsx`) absent from the primary navigation.
4. **BTC Price & Signal Contradiction**:
   - Signal generation (`internal/server/signal_handlers.go`) hardcoded fallback prices (e.g. `BTC/USD` at `$65,000.00`), while frontend asset feeds and ticker updates displayed different values (e.g. `$92,450.00` or `$40,000.00`), resulting in contradictory trade entry recommendations.

---

## 2. User Stories & Acceptance Criteria

### User Story 1: Fluid Responsive UI with Accessible Mobile Navigation
- **As a** mobile or desktop trader,
- **I want** full access to all application views (Terminal, Investor Ledger, Liquid Screener, News Trading, AI Weights, ML Deep Learning) regardless of screen width,
- **So that** I can operate the platform on phone, tablet, and desktop viewports seamlessly.
- **Acceptance Criteria**:
  - Top navigation includes an interactive mobile menu toggle (hamburger drawer / dropdown / bottom bar) on screens `< 640px`.
  - All tabs are switchable on mobile devices with touch targets >= 44px.
  - Page container utilizes fluid max-width constraints without horizontal clipping.

### User Story 2: True Mark-to-Market (MtM) Valuation & Dynamic Returns
- **As an** investor or risk manager,
- **I want** the portfolio equity, Core/Alpha allocations, and All-Time Return percentage to calculate dynamically in real time from live position prices and cash,
- **So that** portfolio metrics reflect actual real-time market value without hardcoded static badges.
- **Acceptance Criteria**:
  - Remove hardcoded `+2.45% All-Time` string in `App.tsx`.
  - Calculate dynamic total return percentage: `((totalEquity - initialCapital) / initialCapital) * 100` and display with appropriate color formatting (`+` in emerald, `-` in rose).
  - Dynamic equity calculates from live positions `entryPrice`, `currentPrice`, and `size`.

### User Story 3: Integrated Machine Learning & Autonomous Daemon
- **As a** quant trader,
- **I want** the ML Training & Telemetry engine accessible in the UI, and the autonomous trading loop actively running in the backend,
- **So that** deep learning model runs and automated risk evaluations are executed and visible.
- **Acceptance Criteria**:
  - Add "ML Engine" tab to navigation in `App.tsx` rendering `MLTrainingView`.
  - Ensure `cmd/trader/main.go` instantiates and boots the autonomous trading daemon/tick processor.

### User Story 4: Synchronized Market Price & Signal Generation
- **As an** operator,
- **I want** trade signals generated with entry prices matching the live market price of the asset,
- **So that** signal entry prices and live ticker quotes are fully aligned without hardcoded discrepancies.
- **Acceptance Criteria**:
  - `internal/server/signal_handlers.go` and `internal/server/server.go` query current asset prices from the active feed/redis/in-memory provider rather than static fallback literals.
  - Signal entry prices accurately reflect current market quote at generation time.
