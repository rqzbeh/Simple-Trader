# Technical Research: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Feature**: `specs/005-multi-horizon-investor-ledger`  
**Date**: 2026-09-20  
**Status**: Completed

---

## 1. Investor Capital Ledger & Pro-Rata Share Accounting (NAV Pool)

### Problem Statement
Managing third-party investor funds without user logins requires an administrative system where deposits, withdrawals, and trading gains/losses are accurately attributed. If an investor deposits funds after a trading gain has occurred, their capital must not dilute prior gains or unfairly capture past performance.

### Decision: Share-Based Net Asset Value (NAV) Unit Accounting
- We adopt institutional mutual fund / hedge fund unitized NAV accounting:
  - Total Portfolio Net Asset Value ($\text{NAV}_t$) starts at 1.00 USD per unit at inception.
  - When Investor $i$ deposits amount $D$ at time $t$:
    $$\text{Units Issued} = \frac{D}{\text{NAV}_t}$$
    $$\text{Total Units}_t = \text{Total Units}_{t-1} + \text{Units Issued}$$
  - Live Net Asset Value at time $t$ given current portfolio equity $E_t$:
    $$\text{NAV}_t = \frac{E_t}{\text{Total Units}_t}$$
  - Individual Investor Equity at any time $t$:
    $$\text{Equity}_{i,t} = \text{Units}_i \times \text{NAV}_t$$
  - Individual ROI and Profit:
    $$\text{Total Profit}_i = \text{Equity}_{i,t} + \text{Total Withdrawn}_i - \text{Total Deposited}_i$$
    $$\text{ROI}_i = \frac{\text{Total Profit}_i}{\text{Total Deposited}_i} \times 100\%$$
- **Storage**: PostgreSQL tables `investors` (profile, total deposited, total withdrawn, units, notes) and `investor_transactions` (id, investor_id, type [DEPOSIT, WITHDRAWAL, PROFIT_PAYOUT], amount, units_transacted, nav_at_time, created_at).
- **Cache**: Fast real-time summaries stored in Redis (`investor:summary:{id}` and `investor:pool_nav`) for sub-second UI rendering.

### Alternatives Considered
1. *Fixed-percentage profit split calculated at trade closure*: Fails when deposits or withdrawals happen between trades; leads to unfair profit dilution.
2. *Separate virtual accounts per investor*: Extremely complex order execution; requires splitting every single market order into fractional sub-orders per investor, causing excessive exchange fees and minimum order size violations.

---

## 2. Three-Tier Multi-Horizon Capital Allocation & Liquidity Management

### Problem Statement
Traders cannot afford to have 100% of capital exposed to market volatility when investors request on-demand withdrawals. Liquidating long-term positions or open swing trades during market drawdowns incurs severe slippage and realized losses.

### Decision: 3-Tier Partitioning with Dynamic Buffering
1. **Tier 1 - Liquidity & Withdrawal Buffer (Target: 15%)**:
   - Held 100% in unencumbered cash / USD / USDC / USDT.
   - Zero leverage, zero directional market risk.
   - All investor withdrawal requests are debited directly from Tier 1.
   - Realized profits from Tier 3 tactical swing trades are swept into Tier 1 to compound the withdrawal buffer.
2. **Tier 2 - Core Wealth Preservation (Target: 45%)**:
   - Allocated to high-stability, macro-trend assets: Gold (`XAU/USD`) and Silver (`XAG/USD`).
   - Evaluated on 4-hour and Daily horizons with trend-following strategies (SuperTrend, EMA 50/200, MACD).
3. **Tier 3 - Tactical Alpha Pool (Target: 40%)**:
   - Dedicated to short-term intraday momentum and breakout strategies.
   - Primary timeframe: **3-Hour Candlestick Bars**.
   - Position sizing governed by Half-Kelly criterion with dynamic volatility scaling (ATR).

### Withdrawal Execution Logic
- If `WithdrawalAmount <= Tier1CashReserve`:
  - Instant fulfillment. No open positions modified.
- If `Tier1CashReserve < WithdrawalAmount <= TotalFreeEquity`:
  - Fulfill available Tier 1 cash immediately.
  - Queue remainder; pause new Tier 3 tactical entries; gracefully close shortest-horizon tactical positions on natural take-profit or tight stop-loss.
- If `WithdrawalAmount > TotalFreeEquity`:
  - Reject transaction with error: `INSUFFICIENT_LIQUIDITY_BUFFER`.

---

## 3. 3-Hour Candlestick Aggregation & Multi-Horizon Engine

### Problem Statement
Financial markets contain substantial noise on 1-minute to 15-minute timeframes. 3-hour candles offer a distinct advantage: they align cleanly with global 24-hour trading sessions (8 candles per 24-hour cycle: 00:00, 03:00, 06:00, 09:00, 12:00, 15:00, 18:00, 21:00 UTC) while filtering microstructural slippage.

### Decision: In-Memory Bar Synthesizer in Go
- Implement `CandleAggregator` in `internal/market/aggregator.go`:
  - Ingests 1-minute OHLCV or raw ticker quotes.
  - Synthesizes 3-hour bars keyed by timestamp bucket: `t_bucket = (unix_timestamp / 10800) * 10800`.
  - On 3-hour bar closure, computes standard indicators in pure Go:
    - RSI(14)
    - MACD(12, 26, 9)
    - Bollinger Bands(20, 2.0)
    - ATR(14)
    - SuperTrend(10, 3.0)
- Triggers Tactical Alpha signal evaluation concurrently with microsecond execution latency.

---

## 4. Autonomous News Ingestion & Sentiment Circuit

### Problem Statement
Live news trading was previously passive because it lacked an automated, continuous crawler and deduplication mechanism.

### Decision: Autonomous News Crawler Daemon with Multi-Source Ingestion
- `internal/market/news_crawler.go`:
  - Configurable background worker polling every 60 seconds.
  - Ingests RSS and REST headline streams:
    - CryptoPanic public RSS / developer API (`https://cryptopanic.com/news/rss/`)
    - Yahoo Finance RSS for Gold, Silver, Currencies (`https://finance.yahoo.com/news/rssindex`)
    - Forexfactory economic events
  - Deduplication: SHA-256 hashing of normalized titles: `SHA256(strings.ToLower(strings.TrimSpace(title)))`. Stored in Redis set `news:seen_hashes` with 72-hour TTL.
  - Lexicon NLP Scoring: Uses hyperbolic tangent compression ($\tanh$) from `internal/market/news_sentiment.go`.
  - Sentiment circuit:
    - If `SentimentScore <= -0.60` (extreme panic / hack / lawsuit), a 30-minute freeze on new Long entries is activated for the affected asset category.
    - If `SentimentScore >= +0.40`, tactical entry confluence scores are boosted by +15%.

---

## 5. Dynamic Liquid Crypto Universe Screener

### Problem Statement
Trading only hardcoded BTC, ETH, and SOL leaves significant alpha on the table during altcoin breakouts, but trading arbitrary coins risks extreme quadratic slippage ($\text{Slippage} \propto (Q/D)^2$) and rug pulls.

### Decision: Automated Liquidity Screener
- `internal/market/screener.go`:
  - Evaluates candidate universe (Top 50 market cap) on major public exchange tickers (Binance, Bybit public endpoints).
  - **Admission Criteria**:
    1. 24-hour Dollar Volume $\ge \$50,000,000$.
    2. Bid-Ask Spread $\le 0.10\%$ (10 basis points).
    3. Minimum historical price data $\ge 90$ days.
    4. Stablecoin / Fiat quote pairs (`USDT`, `USDC`, `USD`).
  - Qualified pairs (e.g. SOL, AVAX, LINK, SUI, NEAR, XRP) are published to Redis set `screener:active_universe` and refreshed hourly.
  - Pairs dropping below $25M volume or blowing out spread > 25 bps are immediately marked `FREEZE` for risk management.
