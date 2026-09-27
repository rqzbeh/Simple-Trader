# Feature Specification: Institutional Quantitative Trading Architecture Upgrade

**Feature Branch**: `004-institutional-quant`  
**Created**: 2026-09-20  
**Status**: In-Progress / Implementation  
**Methodology**: GitHub Spec-Kit Spec-Driven Development (`specify-cli`)  

## Executive Summary & Objectives
Simple-Trader is upgraded from a retail indicator scoring prototype to a production-grade institutional quantitative trading engine. It directly addresses the mathematical and market realities of live trading:
1. **Realistic Execution Friction & Smart Routing**: Exchange fee modeling (Maker 0.02%, Taker 0.05%), dynamic half-spread cost, quadratic liquidity impact slippage, and an autonomous continuous execution daemon.
2. **Order Flow Microstructure & Volatility Regimes**: Real-time Order Book Imbalance (OBI), Cumulative Volume Delta (CVD) absorption tracking, multi-timeframe confluence, and ATR-based Volatility Regime classification (Low Vol Mean Reversion, Normal Trending, High Vol Chop).
3. **Macro Economic Calendar & News NLP Engine**: Pre-event trading halts (FOMC, CPI, NFP) to prevent tail-risk blowups, and financial news ingestion with LLM sentiment extraction.
4. **Bayesian Learning & Fractional Kelly Sizing**: Beta-Binomial Thompson Sampling indicator weight updates replacing heuristic steps, and Half-Kelly Criterion position sizing pegged to rolling Sharpe ratios.
5. **Vectorized Backtesting & Monte Carlo Engine**: Historical candle simulation engine with Sharpe/Sortino computation and a 1,000-run bootstrap ruin probability simulator.
6. **Unified PWA Terminal**: Real-time visualization of OBI, CVD, Volatility Regimes, Macro Calendar status, and Backtest views in the React 18 Bun PWA.

---

## User Scenarios & Testing

### User Story 1 - Realistic Execution Friction & Order Routing (Priority: P1)
As a quantitative fund manager, I need order execution to account for exchange fees (Maker 2 bps, Taker 5 bps), bid-ask spreads, and liquidity impact slippage, so that backtest results and paper trading reflect live institutional market liquidity.

**Why this priority**: Without transaction friction and slippage, all quantitative strategies produce false-positive alpha that immediately fails in production live trading.

**Independent Test**: Can be verified independently via `internal/trader/trader_test.go` checking fee deduction, spread addition/subtraction, and non-linear slippage scaling with order size.

**Acceptance Scenarios**:
1. **Given** a BUY order of 10 BTC at mid-price $60,000 with available depth of 100 BTC, **When** executed as a taker, **Then** the effective execution price exceeds $60,000 by half-spread plus quadratic slippage, and taker fee (0.05%) is added to total net cost.
2. **Given** an open position, **When** market price moves to Take Profit or Stop Loss, **Then** exit friction is calculated and deducted from realized net PnL.

---

### User Story 2 - Order Flow Microstructure & Volatility Regimes (Priority: P2)
As an algorithmic trader, I need Order Book Imbalance (OBI) and Cumulative Volume Delta (CVD) integrated with multi-timeframe confluence, alongside a dynamic ATR Volatility Regime classifier, so that the engine enters momentum trends in normal volatility and mean-reversion in low volatility while avoiding choppy danger zones.

**Why this priority**: Indicators like RSI and MACD suffer high failure rates in high-volatility chop or consolidation without order book pressure verification.

**Independent Test**: Verified via `internal/indicators/indicators_test.go` calculating OBI $\in [-1, 1]$, CVD cumulative sums, and classifying market regimes into LOW_VOL_CONSOLIDATION, NORMAL_TRENDING, and HIGH_VOL_CHOP.

**Acceptance Scenarios**:
1. **Given** top-N bid volume significantly exceeding ask volume ($OBI > +0.30$), **When** price dips, **Then** confluence score reflects institutional absorption buying pressure.
2. **Given** VolRatio $> 1.80$, **When** regime classifier runs, **Then** HIGH_VOL_CHOP is triggered and position sizing is reduced or positions remain in cash.

---

### User Story 3 - Macro Economic Calendar Circuit Breaker (Priority: P3)
As a risk officer, I need the system to track scheduled high-impact macroeconomic releases (CPI, FOMC, NFP) and automatically halt new entries within $\pm 15$ minutes of the event, protecting capital from sudden volatility spikes.

**Why this priority**: Macro announcements cause extreme spread widening and slippage spikes that violate normal technical models.

**Independent Test**: Verified via `internal/market/calendar_test.go` ensuring `IsHalted(symbol, time)` returns true during release windows and prevents new order placement.

---

### User Story 4 - Bayesian Thompson Sampling & Fractional Kelly Sizing (Priority: P4)
As a portfolio manager, I need indicator weight calibration to follow Beta-Binomial Bayesian inference (Thompson Sampling) and position sizes to follow the Half-Kelly criterion, so that indicator allocations converge reliably without noise overreaction and position sizing mathematically maximizes geometric growth while guaranteeing ruin prevention.

**Why this priority**: Fixed 2% sizing ignores empirical strategy edge, whereas heuristic weight scaling is susceptible to recency bias.

**Independent Test**: Verified via `internal/ai/bayesian_test.go` and `internal/trader/kelly_test.go` checking Beta distribution sampling bounds $[0.20, 3.00\text{x}]$ and Half-Kelly position sizing formula clamped to $[0.5\%, 2.0\%]$.

---

### User Story 5 - Vectorized Backtesting & Monte Carlo Ruin Simulator (Priority: P5)
As a quantitative researcher, I need a high-throughput historical backtest engine with a 1,000-path Monte Carlo bootstrap simulation, computing Sharpe, Sortino, Max Drawdown, and Probability of Ruin.

**Why this priority**: Validates strategy robustness against sequence risk before allocating capital.

**Independent Test**: Verified via `internal/backtest/backtest_test.go` executing historical simulations and bootstrap runs.

---

## Functional Requirements
- **FR-001**: Engine MUST compute execution quotes incorporating maker/taker fees, spread, and quadratic slippage:
  $$\text{Slippage} = P_{\text{mid}} \times 0.05 \times \left(\frac{Q}{D}\right)^2$$
- **FR-002**: Engine MUST run a continuous 24/7 autonomous trading daemon monitoring active quotes, checking exits, and executing algorithmic orders.
- **FR-003**: System MUST calculate Level-2 Order Book Imbalance (OBI) and Cumulative Volume Delta (CVD).
- **FR-004**: System MUST classify volatility regimes using normalized ATR ($\text{NATR}$) relative to historical moving average.
- **FR-005**: Engine MUST halt opening new trades inside $[T_{\text{event}} - 15\text{min}, T_{\text{event}} + 15\text{min}]$ for high-impact macro releases.
- **FR-006**: Indicator weights MUST be sampled using Beta-Binomial Thompson Sampling: $\theta_i \sim \text{Beta}(\alpha_i, \beta_i)$ clamped in $[0.20, 3.00\text{x}]$.
- **FR-007**: Position risk MUST be dynamically sized using Half-Kelly criterion:
  $$f^* = \frac{p \cdot b - (1-p)}{b}, \quad \text{Risk} = \text{clamp}(0.5 \times f^*, 0.005, 0.02)$$
- **FR-008**: Backtest suite MUST execute Monte Carlo 1,000-run bootstrap analysis reporting 95th/99th percentile Drawdown and Probability of Ruin.
- **FR-009**: PWA dashboard MUST render real-time microstructure signals, volatility regime tags, macro calendar countdown, and backtest results.

---

## Measurable Success Criteria
- **SC-001**: 100% of unit and integration test suites pass in Go (`go test -v ./internal/...`) and Bun (`bun test`).
- **SC-002**: Pure Go execution engine with zero CGO compiles cleanly (`CGO_ENABLED=0`).
- **SC-003**: Slippage and transaction costs accurately reflect real-world exchange liquidity models.
- **SC-004**: Backtester can evaluate 10,000 historical bars in under 100 milliseconds.
