# Specification 002: Institutional-Grade Quantitative Architecture Upgrade

## 1. Objective & Scope
Elevate Simple-Trader from an educational/retail architecture to an institutional-grade autonomous quantitative trading engine capable of operating in production live markets with:
1. **Realistic Market Friction & Smart Routing**: True exchange fee accounting (Maker 0.02% / Taker 0.05%), dynamic bid-ask spread models, and quadratic liquidity impact slippage.
2. **Order Flow Microstructure & Regime Switching**: Real-time Order Book Imbalance (OBI), Cumulative Volume Delta (CVD), multi-timeframe confluence (15m, 1h, 4h), and ATR-based Volatility Regime classification.
3. **Macro Economic Calendar & News NLP Engine**: Pre-event trading halts around high-impact macro releases (US CPI, FOMC, NFP) and structured LLM news sentiment extraction.
4. **Bayesian Learning & Fractional Kelly Sizing**: Beta-Binomial Thompson Sampling for indicator weights and Half-Kelly Criterion position sizing based on rolling Sharpe ratios.
5. **Vectorized Backtesting & Monte Carlo Engine**: High-throughput historical replay, Sharpe/Sortino computation, and 1,000-run Monte Carlo ruin probability simulation.
6. **Live Production Wiring**: Integration into `cmd/trader/main.go`, SSE stream, and React 18 Bun PWA dashboard.

---

## 2. Mathematical Models & Formulations

### 2.1 Execution Friction & Liquidity Slippage
For an order of size $Q$ executed at tick price $P$ with available top-of-book depth $D$ and bid-ask spread $S$:
$$\text{Execution Price}_{\text{BUY}} = P + \frac{S}{2} + P \cdot \kappa \left(\frac{Q}{D}\right)^2$$
$$\text{Execution Price}_{\text{SELL}} = P - \frac{S}{2} - P \cdot \kappa \left(\frac{Q}{D}\right)^2$$
where $\kappa = 0.05$ (liquidity impact coefficient).
Total transaction cost:
$$\text{Cost} = Q \cdot P_{\text{exec}} \cdot (1 + \text{Fee}_{\text{taker}})$$
where $\text{Fee}_{\text{taker}} = 0.0005$ (0.05%) and $\text{Fee}_{\text{maker}} = 0.0002$ (0.02%).

### 2.2 Order Book Imbalance (OBI) & Cumulative Volume Delta (CVD)
For Level-2 order book depth $N$:
$$\text{OBI}_t = \frac{\sum_{i=1}^N Q_{\text{bid}, i} - \sum_{i=1}^N Q_{\text{ask}, i}}{\sum_{i=1}^N Q_{\text{bid}, i} + \sum_{i=1}^N Q_{\text{ask}, i}} \in [-1.0, 1.0]$$
Cumulative Volume Delta:
$$\Delta V_t = V_{\text{buy}, t} - V_{\text{sell}, t}, \quad \text{CVD}_T = \sum_{t=0}^T \Delta V_t$$

### 2.3 Volatility Regime Switching
Using 14-period normalized ATR:
$$\text{NATR}_t = \frac{\text{ATR}_{14}(t)}{P_t} \times 100$$
- $\text{NATR}_t < 0.8 \times \mu_{\text{NATR}}$: **LOW_VOL_CONSOLIDATION** (favor Mean Reversion & Bollinger bounce).
- $0.8 \times \mu_{\text{NATR}} \le \text{NATR}_t \le 1.8 \times \mu_{\text{NATR}}$: **NORMAL_TRENDING** (favor SuperTrend & MACD breakouts).
- $\text{NATR}_t > 1.8 \times \mu_{\text{NATR}}$: **HIGH_VOL_CHOP / DANGER** (reduce position size by 50% or enter `CASH_HOLD`).

### 2.4 Bayesian Thompson Sampling
Each indicator $i \in \{\text{RSI}, \text{MACD}, \text{SUPERTREND}, \text{OBI}, \text{CVD}\}$ maintains posterior parameters $(\alpha_i, \beta_i)$:
- Prior: $\alpha_i = 2.0, \beta_i = 2.0$ (Beta distribution prior).
- After trade evaluation with return $R$:
  - If $R > 0$ and indicator aligned: $\alpha_i \leftarrow \alpha_i + \min(5.0, R \times 100)$
  - If $R \le 0$ and indicator aligned: $\beta_i \leftarrow \beta_i + \min(5.0, |R| \times 100)$
- Sampling step:
  $$\theta_i \sim \text{Beta}(\alpha_i, \beta_i), \quad w_i = \frac{e^{\theta_i / \tau}}{\sum_j e^{\theta_j / \tau}}$$

### 2.5 Fractional Kelly Criterion
Based on rolling win rate $p$, average win/loss ratio $b = \frac{\bar{W}}{\bar{L}}$:
$$f^* = \frac{p \cdot b - (1 - p)}{b}$$
$$\text{Position Risk} = \text{clamp}\left(0.5 \times f^*, 0.005, 0.02\right)$$
Using Half-Kelly ensures maximum growth while preventing catastrophic drawdowns.

---

## 3. Component Architecture & Data Flow
- `internal/trader/friction.go`: ExecutionModel with fees, spread, and quadratic slippage.
- `internal/indicators/orderflow.go`: Order Book Imbalance (OBI) and Cumulative Volume Delta (CVD).
- `internal/indicators/regime.go`: Volatility Regime Classifier (Consolidation, Trending, High Vol).
- `internal/market/calendar.go`: High-impact macro economic calendar with automated trading halt scheduler.
- `internal/ai/bayesian.go`: Thompson Sampling weight engine replacing heuristic adjustments.
- `internal/trader/kelly.go`: Rolling statistical tracker & Half-Kelly position sizer.
- `internal/backtest/engine.go`: Historical backtester and Monte Carlo ruin simulator.
- `internal/trader/daemon.go`: Production continuous trading daemon loop.
- `web/src/`: Updates to PWA dashboard displaying OBI, CVD, Volatility Regimes, Macro Calendar status, and Backtest views.
