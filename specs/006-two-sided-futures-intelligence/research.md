# Research: Two-Sided Futures Trading, Dynamic Macro Allocation, News-Catalyst Alpha, Auth Security, and Telegram Signals

**Feature**: `006-two-sided-futures-intelligence`  
**Date**: 2026-09-20  
**Status**: Completed  

---

## 1. Two-Sided Futures Contract Execution & Mathematical Sizing

### Decision
Model symmetrical two-sided futures positions (`LONG` and `SHORT`) with leverage, explicit liquidation price calculation, and strict risk-to-reward gating.

### Mathematical Formulation

#### 1. Symmetrical PnL & ROI Calculations
For position size $Q$ (in base asset units), entry price $P_{\text{entry}}$, exit price $P_{\text{exit}}$, leverage $L$, and allocated initial margin $M = \frac{Q \times P_{\text{entry}}}{L}$:

$$\text{PnL}_{\text{Long}} = Q \times (P_{\text{exit}} - P_{\text{entry}})$$

$$\text{PnL}_{\text{Short}} = Q \times (P_{\text{entry}} - P_{\text{exit}})$$

$$\text{ROI} (\%) = \frac{\text{PnL}}{M} \times 100 = \pm \frac{|P_{\text{exit}} - P_{\text{entry}}|}{P_{\text{entry}}} \times L \times 100$$

#### 2. Liquidation Boundaries (Isolated Margin)
With maintenance margin rate $MMR$ (typically $0.5\%$ for crypto futures):

$$P_{\text{liq, Long}} = P_{\text{entry}} \times \left(1 - \frac{1}{L} + MMR\right)$$

$$P_{\text{liq, Short}} = P_{\text{entry}} \times \left(1 + \frac{1}{L} - MMR\right)$$

#### 3. Strict Capital Allocation & Half-Kelly Sizing
To prevent single-trade ruin, every trade allocates at most $2.0\%$ of total portfolio equity to risk:

$$\text{Risk Amount} = \text{Portfolio Equity} \times 0.02$$

$$\text{Stop Loss Distance} = \frac{|P_{\text{entry}} - P_{\text{SL}}|}{P_{\text{entry}}}$$

$$\text{Position Value} = \frac{\text{Risk Amount}}{\text{Stop Loss Distance}}$$

$$\text{Margin Required} = \frac{\text{Position Value}}{L} \le \text{Available Tier 2 Tactical Alpha}$$

#### 4. Risk-to-Reward Ratio ($R:R$)
A trade signal is valid only if:

$$R:R = \frac{|P_{\text{TP}} - P_{\text{entry}}|}{|P_{\text{entry}} - P_{\text{SL}}|} \ge 1.5 \quad (\text{Institutional Target} \ge 2.0)$$

### Alternatives Considered
- *Unleveraged Spot Only*: Rejected because spot cannot profit during market downturns, cutting opportunity by 50% and preventing portfolio hedging.
- *Full Kelly Sizing*: Rejected because full Kelly exhibits excessive volatility and drawdown ($>50\%$), which is unacceptable for institutional risk management.

---

## 2. News-First Catalyst Trigger Architecture

### Decision
Establish news catalysts (geopolitical conflicts, whale accumulation/dumps, interest rate announcements, regulatory actions) as the mandatory primary trigger for short-term trades. Technical indicators (RSI, SuperTrend, MACD, Bollinger Bands, CVD, OBI) are relegated strictly to execution timing (entry limit boundary, leverage calibration, stop-loss, and take-profit targets).

### Trade Decision Matrix

| News Catalyst Sentiment ($\tanh$) | Technical Alignment | Trade Decision | Rationale |
|---|---|---|---|
| Strong Bullish ($\ge +0.30$) | Oversold Pullback / Bullish SuperTrend | `LONG` | Catalyst confirms directional breakout; indicators identify optimal low-slippage entry. |
| Strong Bearish ($\le -0.30$) | Overbought Rally / Bearish SuperTrend | `SHORT` | Catalyst confirms systemic downside; indicators set tight stop-loss above recent resistance. |
| Neutral / Noise ($-0.15$ to $+0.15$) | Any Indicator Signal | `HOLD` | Refrain from trade entry; indicators without catalyst produce chop and false breakouts. |
| Bullish ($\ge +0.25$) | Divergent Extreme Overbought | `HOLD` / Wait | Wait for technical pullback before executing `LONG`. |
| Bearish ($\le -0.25$) | Divergent Extreme Oversold | `HOLD` / Wait | Wait for technical bounce before executing `SHORT`. |

### Alternatives Considered
- *Indicator-Only Autonomous Trades*: Rejected based on user feedback and quantitative testing showing indicators lag and produce false breakouts during news-driven re-pricing.

---

## 3. Real-World Dynamic Macro Allocation

### Decision
Replace static 3-Tier capital allocations ($15\%$ Cash, $40\%$ Alpha, $45\%$ Core) with a dynamic macro regime engine that evaluates real geopolitical stress, interest rate policy bias, and inflation metrics.

### Quantitative Regime Model

$$\text{Macro Stress Score} (S) = w_{\text{geo}} \cdot I_{\text{geo}} + w_{\text{inf}} \cdot I_{\text{inf}} + w_{\text{rate}} \cdot I_{\text{rate}} \in [0.0, 1.0]$$

1. **High Crisis Regime ($S \ge 0.65$)**: Escalating war/geopolitical conflict or stagflation shock.
   - **Tier 3 (Core Preservation - Gold/Silver)**: Expands to **$55\%$ - $60\%$**
   - **Tier 1 (Cash Buffer - USD/USDC)**: Expands to **$20\%$ - $25\%$**
   - **Tier 2 (Tactical Alpha)**: Contracts to **$15\%$ - $25\%$**
2. **Normal Trending Regime ($0.35 \le S < 0.65$)**: Balanced macro environment.
   - **Tier 3 (Core)**: **$45\%$**
   - **Tier 2 (Alpha)**: **$40\%$**
   - **Tier 1 (Cash)**: **$15\%$**
3. **Dovish Liquidity Expansion ($S < 0.35$)**: Tranquil macro, rate cuts, expansionary liquidity.
   - **Tier 2 (Tactical Alpha)**: Expands to **$45\%$ - $50\%$**
   - **Tier 3 (Core)**: Contracts to **$35\%$ - $40\%$**
   - **Tier 1 (Cash)**: Maintains safety baseline of **$15\%$**

### Alternatives Considered
- *Fixed Percentage Allocations*: Rejected because static allocations suffer severe drawdowns during geopolitical black swans and fail to scale exposure in benign liquidity cycles.

---

## 4. Secure Web Authentication & AES-GCM-256 Field Encryption

### Decision
- **Authentication**: Salted `bcrypt` password hashing (cost factor 12) with cryptographically random session tokens (32-byte hex) cached in Redis/memory with 7-day expiration.
- **Brute Force Defense**: Sliding-window rate limiter per client IP and username (maximum 5 failed login attempts per 15 minutes, returning HTTP 429).
- **Session Protection**: HttpOnly, SameSite=Strict cookies and `Authorization: Bearer <token>` support for API requests.
- **Database Field Encryption**: AES-256-GCM encryption-at-rest for sensitive columns (exchange API keys, Telegram bot tokens, investor private contact info) using a 256-bit master key derived via SHA-256 from `APP_SECRET` or `ENCRYPTION_KEY`.

### Security Flow
```
Client Browser ---> HTTP POST /api/v1/auth/login ---> Rate Limit Check (Redis)
                                                          │
                                                    Valid Rate?
                                                    ├── No  ---> HTTP 429 Too Many Requests
                                                    └── Yes ---> Query Admin Password Hash
                                                                    │
                                                              bcrypt.CompareHashAndPassword
                                                              ├── Fail ---> Increment Fail Count, HTTP 401
                                                              └── Pass ---> Issue Secure Token & Cookie
```

### Alternatives Considered
- *Plain JWT without Invalidation*: Rejected because standard JWTs cannot be revoked instantly if compromised. Stateful Redis-backed tokens allow instant revocation upon logout or security breach.

---

## 5. Telegram Signals Bot Architecture

### Decision
Deploy an autonomous Go Telegram broadcaster connecting to the Telegram Bot API (`https://api.telegram.org/bot<TOKEN>/sendMessage`) with MarkdownV2 formatting.

### Message Structures

#### 1. Formatted Entry Signal Card
```text
🚨 *SIMPLE-TRADER V2.0 FUTURES SIGNAL* 🚨
━━━━━━━━━━━━━━━━━━━━
🎯 *Asset*: BTC/USDT (Futures)
⚡ *Direction*: 🔴 SHORT
💰 *Entry Price*: $63,450.00
📊 *Leverage*: 5x (Isolated)
🛡 *Stop Loss*: $64,250.00 (-1.26%)
🎯 *Take Profit 1*: $61,850.00 (+2.52%)
🎯 *Take Profit 2*: $60,200.00 (+5.12%)
⚖ *Risk/Reward*: 1:2.0
💼 *Allocated Capital*: $1,250.00 (3.1% of Tactical Alpha)
📰 *Catalyst*: "Massive 12,000 BTC exchange deposit detected from institutional whale wallet"
🕒 *Time*: 2026-09-20 14:32 UTC
━━━━━━━━━━━━━━━━━━━━
```

#### 2. Formatted Closed Trade Resolution
```text
🏁 *TRADE CLOSED & RESOLVED* 🏁
━━━━━━━━━━━━━━━━━━━━
🎯 *Asset*: BTC/USDT (Futures)
⚡ *Direction*: 🔴 SHORT
🚪 *Exit Price*: $61,850.00 (Take Profit 1 Hit)
⏱ *Hold Duration*: 3h 42m
💵 *Realized PnL*: +$157.60
📈 *Net ROI*: +12.60% (Leveraged)
━━━━━━━━━━━━━━━━━━━━
```

### Asynchronous Queue & Retries
- Telegram notifications execute asynchronously via Go channels so HTTP delays to Telegram never block the trading engine.
- HTTP 429 (Rate Limit) triggers exponential backoff with jitter up to 3 retries.

---

## 6. PWA Installation & iOS Safari Walkthrough

### Decision
- **Chromium / Android**: Capture `beforeinstallprompt` event, prevent default mini-infobar, store the deferred prompt, and present a sleek, dismissible installation card with an "Install Simple-Trader" CTA.
- **Apple iOS Safari**: Detect via `navigator.userAgent` matching iPhone/iPad combined with `('standalone' in navigator && !navigator.standalone)`. Display an interactive modal walkthrough visually guiding the user:
  1. Tap the Safari **Share** icon (`Square with upward arrow`).
  2. Scroll down and tap **Add to Home Screen** (`Plus icon in square`).
  3. Tap **Add** in the top right.
- **Standalone Mode**: Check `window.matchMedia('(display-mode: standalone)').matches` or `navigator.standalone === true`. When active, suppress all prompts and guidance banners.

---

## 7. Real-Data Machine Learning Training Pipeline

### Decision
Eliminate all synthetic or mockup data generation. Train Bayesian indicator weights and trade prediction models strictly on authentic market history:
1. Ingest $\ge 1,000$ authentic 1-hour and 3-hour OHLCV candles directly from Binance public API (`/api/v3/klines`).
2. Pair candlestick sequences with timestamped historical news and macroeconomic events.
3. Compute forward return distributions and calibrate Bayesian prior-to-posterior updates using observed Sharpe ratio and win-rate metrics.
4. Output persistent training log artifacts including dataset date range, sample count, log-loss, and directional accuracy.
