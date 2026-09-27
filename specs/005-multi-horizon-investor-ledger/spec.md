# Feature Specification: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Feature Branch**: `005-multi-horizon-investor-ledger`

**Created**: 2026-09-20

**Status**: Draft

**Input**: User description: "Multi-Horizon 3-Tier Liquidity Allocator, Live News Trading Ingestion, Dynamic Liquid Crypto Screener, and Investor Capital Ledger System"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Investor Capital Ledger & Non-Login Account Tracking (Priority: P1)

As a fund manager or quantitative operator, I need an administrative interface to record investor profiles without requiring user logins. I must record their initial deposits, track incremental deposits and withdrawals, and automatically compute each investor's pro-rata share of trading profits, realized ROI, and net asset value (NAV) based on the master portfolio's performance.

**Why this priority**: Managing third-party investor capital requires an immutable, transparent accounting ledger of deposits, withdrawals, and pro-rata returns before executing live trades on their behalf.

**Independent Test**: Can be tested by creating an investor (e.g. "Alice", $10,000 initial capital), simulating a +5% portfolio gain, executing a partial withdrawal ($2,000), and verifying that Alice's calculated ROI, remaining equity ($8,500), and cash ledger match mathematical pro-rata attribution exactly.

**Acceptance Scenarios**:
1. **Given** the operator enters an investor's full name, email/contact tag, and initial deposit amount in the UI, **When** the operator submits the form, **Then** the investor is securely persisted in PostgreSQL, their allocated equity is credited, and an initial deposit transaction record is logged.
2. **Given** an existing registered investor with deposited capital, **When** the operator posts a deposit or withdrawal transaction, **Then** the investor's balance updates immediately, the system checks liquidity buffer availability for withdrawals, and the historical transaction ledger is appended.
3. **Given** open and closed trading results across the platform, **When** the operator views the Investor Dashboard, **Then** each investor's individual dollar profit, percentage ROI, and fractional pool ownership share are dynamically rendered with zero manual calculation errors.

---

### User Story 2 - Three-Tier Liquidity Allocation with Withdrawal Reserve (Priority: P1)

As a risk manager, I need the automated capital allocator to divide total portfolio equity across three disciplined tiers:
- **Tier 1 (15% Target)**: Unencumbered liquid cash/stablecoins held strictly in reserve for instant user withdrawal fulfillment without forced liquidations.
- **Tier 2 (45% Target)**: Core wealth preservation assets (Gold `XAU/USD`, Silver `XAG/USD`) held for long-term hedging and low turnover.
- **Tier 3 (40% Target)**: Tactical Alpha trading pool executing multi-horizon strategies.

**Why this priority**: Solves the critical financial risk where sudden investor withdrawal requests force market liquidations during adverse volatility, incurring slippage and locking in losses.

**Independent Test**: Can be tested by allocating $100,000 total capital, placing tactical trades, and requesting a $10,000 withdrawal. The withdrawal must be debited directly from Tier 1 liquid cash without disturbing Tier 2 or Tier 3 active positions.

**Acceptance Scenarios**:
1. **Given** total portfolio capital $C$, **When** the capital allocator initializes or rebalances, **Then** it reserves at least 15% in liquid cash, allocates up to 45% to Core commodities, and directs the remaining 40% to Tactical Alpha trading.
2. **Given** an investor requests a withdrawal within the Tier 1 cash buffer, **When** the withdrawal is approved, **Then** it is deducted from cash immediately without closing open positions or generating market slippage.
3. **Given** a withdrawal request that exceeds Tier 1 cash, **When** processed, **Then** the engine safely flags the liquidity deficit and rebalances tactical capital gracefully rather than dumping market orders indiscriminately.

---

### User Story 3 - Multi-Horizon Trading with 3-Hour Swing Timeframe (Priority: P2)

As an algorithmic trader, I need the trading engine to operate across multiple discrete time horizons:
- **Short-Term Intraday / 3-Hour Swing**: Computes indicators on 3-hour candle bars to capture intraday momentum, volume breakouts, and order book pressure while filtering out 1-minute microstructure noise.
- **Long-Term Macro Trend**: Daily/4-hour trend evaluation for Core assets.
Realized short-term profits are periodically swept into Tier 1 (Cash) to lock in gains and grow the withdrawal buffer.

**Why this priority**: 3-hour bars represent an institutional sweet spot for swing trading, capturing sustained directional moves with superior signal-to-noise ratios compared to noisy 1-minute ticks.

**Independent Test**: Can be tested by feeding historical price series aggregated into 3-hour candles, evaluating confluence signals, and executing entries with automated Stop-Loss (SL) and Take-Profit (TP) tailored to 3-hour Average True Range (ATR).

**Acceptance Scenarios**:
1. **Given** incoming market ticks, **When** aggregated into completed 3-hour bars, **Then** the technical indicator engine calculates RSI, MACD, SuperTrend, and Bollinger Bands specifically calibrated for the 3-hour horizon.
2. **Given** a tactical 3-hour trade that hits its profit target, **When** closed, **Then** the realized net profit is apportioned to the liquid cash buffer to compound portfolio safety.

---

### User Story 4 - Live News Trading Ingestion & Sentiment Circuit (Priority: P2)

As a quantitative analyst, I need an automated news aggregator that continuously fetches real-time market news headlines from public RSS/API feeds (CryptoPanic, Yahoo Finance, ForexFactory) and evaluates them via financial lexicon analysis and LLM intelligence to boost or halt trading signals before market impact.

**Why this priority**: Major breaking headlines create rapid liquidity vacuums and directional surges; trading blind to live breaking news risks severe adverse selection.

**Independent Test**: Can be tested by ingesting synthetic and live RSS headline feeds, verifying headline deduplication, and validating that high-sentiment headlines dynamically modulate confluence scores or trigger entry freezes.

**Acceptance Scenarios**:
1. **Given** continuous background operation, **When** new RSS articles or headlines are published, **Then** the news daemon ingests them, deduplicates via SHA-256 fingerprinting, and stores them in the database.
2. **Given** an ingested headline containing extreme negative keywords (e.g. "insolvency", "sec lawsuit", "hack"), **When** evaluated, **Then** the sentiment polarity registers as `BEARISH` with negative score, dampening tactical buy signals for affected assets.

---

### User Story 5 - Dynamic Liquid Crypto Screener (Top Volume & Low Spread) (Priority: P3)

As a portfolio manager, I want the system to dynamically screen and select tradeable cryptocurrencies beyond just BTC, ETH, and SOL, evaluating the top liquid assets (e.g. Top 20 by 24h volume on major exchanges) with strict volume thresholds (> $50M/day) and low bid-ask spreads (< 0.10%), filtering out illiquid low-cap tokens.

**Why this priority**: Safely expands alpha opportunities across high-momentum altcoins while mathematically protecting against quadratic slippage and manipulation.

**Independent Test**: Can be tested by running the screener against market tickers, verifying that high-volume liquid coins (e.g. SOL, AVAX, LINK, SUI) pass admission, while low-volume tokens are rejected.

**Acceptance Scenarios**:
1. **Given** a universe of candidate crypto pairs, **When** the screener runs periodic checks, **Then** it admits only assets meeting minimum 24h volume and maximum spread criteria.
2. **Given** an admitted asset experiencing a sudden volume drop or spread blowout, **When** the screener detects the deterioration, **Then** the asset is placed in freeze mode (no new entry quotes allowed).

---

### Edge Cases

- **Mass Withdrawal Request**: If simultaneous withdrawal requests exceed the current 15% Tier 1 cash buffer, the engine halts new tactical entries and orderly unwinds shortest-horizon positions without exceeding daily slippage tolerance.
- **Extreme High-Impact News Flash**: If multiple breaking news items register extreme negative sentiment within 5 minutes, tactical order entry quotes for the affected asset class are immediately frozen for 30 minutes.
- **Asset Delisting or Sudden Volume Collapse**: If an altcoin in the dynamic crypto screener loses liquidity or is halted on primary exchanges, open positions are liquidated via limit orders where possible, and the symbol is removed from the active universe.
- **Investor Deposit During Active Market Volatility**: New deposits are initially credited to Tier 1 Cash and gradually phased into tactical positions during the next rebalancing cycle to prevent buying market tops.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide an administrative interface and API to create, read, update, and list investor profiles (storing name, contact identifier, status, and registration timestamp) without requiring user authentication or logins.
- **FR-002**: The system MUST maintain an immutable ledger of all investor capital transactions (deposits, withdrawals, fees, and profit distributions) stored in PostgreSQL.
- **FR-003**: The system MUST continuously calculate and display each investor's net equity, total deposited capital, total withdrawn capital, dollar profit/loss, percentage ROI, and pro-rata ownership fraction of the total pool.
- **FR-004**: The capital allocator MUST divide total available capital into three distinct tiers: Tier 1 Liquidity & Withdrawal Buffer (target 15%), Tier 2 Core Wealth Preservation (target 45%), and Tier 3 Tactical Alpha Trading (target 40%).
- **FR-005**: All investor withdrawal requests MUST be settled directly from Tier 1 liquid cash. If a withdrawal would breach minimum cash reserve thresholds, the system MUST flag a liquidity alert and reject or queue the withdrawal.
- **FR-006**: The trading engine MUST support a discrete 3-hour candlestick timeframe for tactical intraday alpha assets, aggregating ticks into 3-hour OHLCV bars and calculating technical indicators thereon.
- **FR-007**: The system MUST run an autonomous background news crawler that ingests market headlines from public financial and crypto RSS feeds at configurable polling intervals.
- **FR-008**: Ingested headlines MUST be deduplicated using SHA-256 content hashes and evaluated using quantitative lexicon scoring and LLM zero-shot sentiment classification.
- **FR-009**: Sentiment polarity scores MUST be dynamically incorporated into trade confluence scoring, dampening or augmenting signal confidence.
- **FR-010**: The system MUST implement an automated crypto universe screener that dynamically filters candidates based on 24-hour turnover (> $50M equivalent) and maximum allowable bid-ask spread (< 10 bps).

---

### Key Entities *(include if feature involves data)*

- **Investor**: Represents an individual capital contributor. Attributes: unique ID, full name, contact/email tag, notes, created timestamp, active status.
- **CapitalTransaction**: Represents a discrete financial flow. Attributes: transaction ID, investor ID, type (`DEPOSIT`, `WITHDRAWAL`, `PROFIT_DISTRIBUTION`), amount, currency, timestamp, notes, reference balance.
- **PortfolioSnapshot**: Daily/hourly portfolio NAV checkpoint tracking pro-rata pool valuation for accurate historical ROI calculation.
- **NewsArticle**: Normalized financial headline. Attributes: content hash, title, source, URL, published time, sentiment score, sentiment polarity, key phrases.
- **ScreenerCriteria**: Parameters defining asset qualification (minimum 24h volume, maximum spread, volatility bounds, ranking tier).

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can create a new investor record and log an initial deposit in under 15 seconds via the UI terminal.
- **SC-002**: Investor pro-rata equity and ROI calculations reflect live portfolio NAV changes with sub-second recalculation latency.
- **SC-003**: Tier 1 cash buffer guarantees that 100% of standard withdrawal requests within the 15% threshold are processed with zero market slippage and zero impact on open trades.
- **SC-004**: News aggregator collects, deduplicates, and evaluates market headlines within 60 seconds of publication on monitored feeds.
- **SC-005**: 3-hour technical indicator calculation executes in under 5 milliseconds per bar on the Go execution engine.
- **SC-006**: Dynamic screener refreshes eligible altcoin universe every 60 minutes, ensuring zero capital is allocated to pairs with spreads exceeding 10 basis points.

---

## Assumptions

- Investor profiles are strictly administrative records for internal fund accounting; external investors do not need direct web authentication credentials in v1.
- Initial capital is denominated in USD or USD-pegged stablecoins (USDT/USDC) for consistent portfolio accounting.
- RSS and public REST APIs (e.g. Yahoo Finance, Binance Public Market Data, CryptoPanic) are accessible without requiring enterprise-tier paid subscriptions.
- PostgreSQL 16 is the primary durable store for investor ledger tables, and Redis 7 caches live NAV and investor balance summaries for fast UI rendering.
