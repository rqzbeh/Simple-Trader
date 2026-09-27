# Simple-Trader • Institutional Quantitative Trading Specification

**Spec ID:** SPEC-002  
**Status:** Approved  
**Target:** Pure Go 1.24+ Backend & React 18 Bun PWA  
**Focus:** Institutional Market Friction, Order Microstructure, Volatility Regimes, Macro Calendar, Bayesian Thompson Sampling, Kelly Position Sizing, and Monte Carlo Validation.

---

## 1. Mathematical & Engineering Foundations

### 1.1 Market Friction Model (Friction & Liquidity Impact)
In real-world institutional trading, naive backtests fail due to unmodeled transaction costs:
$$\mathbb{E}[R] = \sum (P_{\text{win}} W - P_{\text{loss}} L) - \sum \text{Friction}$$

Friction per round-trip trade consists of three deterministic components:
1. **Exchange Fee**:
   - Maker (Passive limit): $F_{\text{maker}} = 0.02\%$ ($0.0002$)
   - Taker (Aggressive market): $F_{\text{taker}} = 0.05\%$ ($0.0005$)
2. **Bid-Ask Spread**:
   $$\text{HalfSpread}(s) = \frac{P_{\text{ask}} - P_{\text{bid}}}{2 \cdot P_{\text{mid}}}$$
3. **Quadratic Liquidity Impact (Slippage)**:
   $$\text{Slippage}(Q, V) = \gamma \cdot P \cdot \left(\frac{Q}{\bar{V}_{\text{minute}}}\right)^2$$
   where $\gamma = 0.05$ (market impact coefficient), $Q$ is order quantity, and $\bar{V}_{\text{minute}}$ is average minute volume.

**Effective Execution Pricing**:
- **BUY (Taker)**: $P_{\text{exec}} = P_{\text{mid}} \cdot (1 + \text{HalfSpread}) \cdot (1 + \text{Slippage}) + \text{Fee}$
- **SELL (Taker)**: $P_{\text{exec}} = P_{\text{mid}} \cdot (1 - \text{HalfSpread}) \cdot (1 - \text{Slippage}) - \text{Fee}$

---

### 1.2 Order Flow Microstructure & Volatility Regimes

#### Order Book Imbalance (OBI)
Measures the supply-demand asymmetry of top-$N$ limit order book levels:
$$\text{OBI}_N = \frac{\sum_{i=1}^N Q_{\text{bid}, i} - \sum_{i=1}^N Q_{\text{ask}, i}}{\sum_{i=1}^N Q_{\text{bid}, i} + \sum_{i=1}^N Q_{\text{ask}, i}} \in [-1.0, 1.0]$$
- $\text{OBI} > +0.30$: Institutional bid wall / accumulation pressure.
- $\text{OBI} < -0.30$: Institutional ask wall / distribution pressure.

#### Cumulative Volume Delta (CVD)
Aggregates aggressive buyer vs seller volume:
$$\Delta V_t = V_{\text{buyer-initiated}} - V_{\text{seller-initiated}}$$
$$\text{CVD}_t = \sum_{\tau=0}^t \Delta V_\tau$$
- **Bullish Absorption**: Price makes a new lower low while CVD forms a higher low (smart money absorbing sell orders).
- **Bearish Exhaustion**: Price makes a higher high while CVD makes a lower high.

#### Volatility Regime Filter
Classifies market states dynamically using relative ATR expansion:
$$\text{VolRatio} = \frac{\text{ATR}_{14}}{\text{SMA}(\text{ATR}_{14}, 50)}$$
1. **Regime 1: High Volatility Trend ($\text{VolRatio} > 1.30$)**
   - Active: SuperTrend + MACD Momentum.
   - Sizing: Scaled down inversely by $\text{VolRatio}$ to maintain constant portfolio volatility.
2. **Regime 2: Normal Volatility ($0.70 \le \text{VolRatio} \le 1.30$)**
   - Active: Multi-indicator confluence (RSI + MACD + OBI + CVD).
3. **Regime 3: Low Volatility Mean-Reversion ($\text{VolRatio} < 0.70$)**
   - Active: Bollinger Band mean-reversion with strict RSI overbought/oversold limits. Avoid breakout signals.

---

### 1.3 Macroeconomic Event Filter & News NLP

#### Economic Calendar Halt Engine
Tracks scheduled high-impact events:
- US Non-Farm Payrolls (NFP)
- US Consumer Price Index (CPI)
- Federal Reserve FOMC Rate Decision & Press Conference
- ECB Rate Decision

**Execution Rules**:
- Inside $[T_{\text{event}} - 15\text{min}, T_{\text{event}} + 15\text{min}]$:
  - System state switches to `MACRO_VOLATILITY_HOLD`.
  - Prohibits opening any new positions.
  - Automatically tightens existing stops or locks in partial profits.

#### News Sentiment Pipeline
Ingests financial RSS feeds (Yahoo Finance, Finviz, CryptoPanic), deduplicates headlines via SHA-256 canonical hashing, and parses sentiment polarity via OpenAI-compatible endpoints with a deterministic schema:
```json
{
  "symbol": "XAU/USD",
  "polarity": 0.75,
  "urgency": 0.80,
  "actionable": true,
  "reasoning": "Central banks increase gold reserves amid dollar weakness"
}
```

---

### 1.4 Bayesian Thompson Sampling & Fractional Kelly Position Sizing

#### Thompson Sampling for Indicator Weights
Replaces deterministic step weights with Bayesian inference. For each indicator $k \in \{\text{RSI}, \text{MACD}, \text{SuperTrend}, \text{OBI}, \text{CVD}, \text{Sentiment}\}$:
- Prior: $\alpha_k \sim \text{Beta}(2.0, 2.0)$
- Update after trade outcome:
  $$\alpha_k \leftarrow \alpha_k + \text{Attribution}_{\text{profit}}$$
  $$\beta_k \leftarrow \beta_k + \text{Attribution}_{\text{loss}}$$
- Weight sample:
  $$\theta_k \sim \text{Beta}(\alpha_k, \beta_k)$$
  $$w_k = 0.20 + 2.80 \cdot \frac{\theta_k}{\sum \theta_j}$$
  Ensuring weights strictly obey $w_k \in [0.20, 3.00\text{x}]$.

#### Half-Kelly Position Sizing
Calculates mathematically optimal capital fraction based on observed performance:
$$f^* = \frac{p \cdot b - (1 - p)}{b}$$
where $p = \text{Win Rate}$ and $b = \frac{\text{Average Win}}{\text{Average Loss}}$.
$$f_{\text{Half-Kelly}} = \max\left(0, \frac{1}{2} \cdot f^* \cdot \min(1.0, \frac{\text{Sharpe}}{2.0})\right)$$
Risk per trade is strictly constrained by:
$$\text{PositionRisk} = \min(f_{\text{Half-Kelly}}, \text{MaxRiskPerTradePct})$$

---

### 1.5 Vectorized Backtesting & Monte Carlo Simulation
- **Backtesting Metrics**:
  - Sharpe Ratio: $\frac{\mathbb{E}[R_p - R_f]}{\sigma_p} \cdot \sqrt{252}$
  - Sortino Ratio: Downside deviation penalization.
  - Maximum Drawdown (MDD) and Calmar Ratio.
  - Profit Factor: $\frac{\sum \text{Gains}}{\sum \text{Losses}}$.
- **Monte Carlo Simulator**:
  - $N=1,000$ bootstrap paths sampling trade log distributions.
  - Computes 95th/99th percentile Drawdown and Probability of Ruin ($P(\text{Equity} \le 0.50 \cdot E_0)$).

---

## 2. Component Implementation Architecture

```
internal/
├── trader/
│   ├── engine.go           # Paper & live execution with realistic friction
│   ├── friction.go         # Taker/maker fees, spread, liquidity slippage
│   ├── daemon.go           # 24/7 continuous autonomous trading goroutine
│   ├── kelly.go            # Fractional Kelly criterion sizing
│   └── allocator.go        # 60% Core / 40% Alpha capital allocation
├── indicators/
│   ├── microstructure.go   # Order Book Imbalance (OBI) & CVD
│   ├── regime.go           # ATR Volatility Regime Classifier
│   ├── confluence.go       # Multi-timeframe weighted confluence
│   └── ...                 # RSI, MACD, SuperTrend, Bollinger, VWAP
├── ai/
│   ├── bayesian.go         # Beta-Binomial Thompson Sampling Engine
│   ├── news_nlp.go         # LLM Macro News Sentiment Parser
│   └── ...                 # OpenAI client & Fine-tuning exporter
├── market/
│   ├── macro_calendar.go   # High-impact economic calendar circuit breaker
│   ├── live_feed.go        # Live WebSocket / Polling Multi-Source Aggregator
│   └── ...                 # Yahoo, Binance, Deduplication
└── backtest/
    ├── backtester.go       # Historical candle simulation engine
    └── monte_carlo.go      # 1,000-run bootstrap ruin & drawdown simulator
```
