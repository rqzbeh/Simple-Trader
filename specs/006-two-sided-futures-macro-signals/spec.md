# Feature Specification: Two-Sided Futures Trading, Dynamic Macro Allocation, News-Catalyst Alpha, Auth Security, and Telegram Signals

**Feature Branch**: `006-two-sided-futures-macro-signals`

**Created**: 2026-09-20

**Status**: Draft

**Input**: User description: "Two-Sided Futures Trading, Real-World Dynamic Macro Allocation, News-Catalyst Short-Term Alpha, PWA Install & iOS Guidance, Auth Security, Telegram Signals Bot, and Real-Data ML Training"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Secure Access Control & Data Protection (Priority: P1)

As a fund manager or authorized trader, I need the terminal protected by a secure authentication gate and hardened storage so that unauthorized external actors cannot view proprietary portfolio allocations, alter capital ledger units, or trigger trades.

**Why this priority**: Without authentication, exposing the terminal on a public VPS host allows anyone to access trade controls, view investor capital records, or execute unwanted operations.

**Independent Test**: Can be tested by visiting the dashboard while unauthenticated, verifying access is blocked, authenticating with valid credentials to unlock full trading controls, and verifying that brute-force attempts are throttled.

**Acceptance Scenarios**:
1. **Given** an unauthenticated session, **When** the user accesses the web application or API endpoints, **Then** a clean login interface is presented and all sensitive endpoints reject unauthenticated requests with HTTP 401 Unauthorized.
2. **Given** valid manager credentials, **When** submitted to the authentication interface, **Then** an authenticated session token is issued, unlocking the trading dashboard, ledger, and signals.
3. **Given** 5 consecutive failed login attempts, **When** another attempt is made, **Then** the system triggers automated rate limiting, delaying subsequent attempts.

---

### User Story 2 - Two-Sided Futures Trading & Capital Risk Management (Priority: P1)

As a quantitative trader, I need the system to recommend and execute both LONG (buy) and SHORT (sell) futures positions with precise per-trade capital allocation, leverage calculation, and minimum 1:2 Risk-to-Reward (R:R) ratios so that my short-term fund is never wiped out by a single adverse price movement.

**Why this priority**: Markets move in both directions; restricting trades to buy-only cuts potential market opportunities in half and prevents hedging against downtrends. Strict per-trade capital caps prevent catastrophic ruin.

**Independent Test**: Can be tested by evaluating market scenarios (e.g. overbought bearish divergence vs oversold bullish breakout) and verifying the engine outputs both LONG and SHORT signals specifying exact capital allocation (% and USD), entry, stop loss, take profit, leverage, and R:R ratio $\ge 2.0$.

**Acceptance Scenarios**:
1. **Given** a bearish macro or breaking news event on a screened asset, **When** execution indicators confirm downward momentum, **Then** the engine generates a SHORT signal with entry price, stop-loss above recent resistance, take-profit targets, leverage level, and capital sizing strictly capped at the defined risk limit (e.g., 2% of portfolio equity).
2. **Given** an active signal generation cycle, **When** the calculated potential reward divided by risk is less than 2.0 (R:R < 1:2), **Then** the trade is rejected or skipped as mathematically unfavorable.
3. **Given** an available short-term tactical fund balance, **When** sizing a position, **Then** the engine outputs the exact recommended position allocation in dollars and percentage of the tactical pool.

---

### User Story 3 - News-Catalyst Alpha with Technical Execution Timing (Priority: P2)

As a trader, I want real-time macroeconomic and geopolitical news headlines to serve as the primary catalyst for opening short-term trades, while technical indicators (RSI, SuperTrend, MACD, OBI, CVD) are utilized specifically for timing entry points, stop-loss, take-profit, and leverage.

**Why this priority**: Technical indicators alone frequently produce false breakouts in choppy markets; high-impact news catalysts provide genuine directional conviction, while technicals ensure disciplined risk-managed execution.

**Independent Test**: Can be tested by feeding incoming news events (e.g. whale alerts, inflation releases, political trade disclosures) and observing that trading rationale cites the news catalyst as the primary motive, while technical indicators calibrate entry slippage, stops, and targets.

**Acceptance Scenarios**:
1. **Given** neutral market newsflow with isolated indicator swings, **When** evaluating short-term alpha entry, **Then** the system refrains from entering high-conviction trades unless a validated news or order flow catalyst is present.
2. **Given** a high-impact news catalyst (e.g., major whale exchange accumulation or regulatory clearance), **When** evaluated, **Then** the engine identifies the directional bias from the news and queries technical indicators to identify optimal entry pullback, stop-loss boundary, and take-profit levels.

---

### User Story 4 - Real-World Dynamic Macro Allocation (Priority: P2)

As a portfolio manager, I need the 3-tier capital allocation (Cash Buffer, Tactical Alpha, Core Preservation) to adapt dynamically based on real-world macroeconomic and geopolitical conditions (e.g., wars, inflation spikes, central bank interest rate shifts) backed by financial science rather than static hardcoded percentages.

**Why this priority**: Static capital splits fail to protect capital during geopolitical crises or capture upside during broad risk-on liquidity cycles. Real-world macro awareness aligns portfolio exposure with systemic market risk.

**Independent Test**: Can be tested by evaluating allocation percentages under simulated macro shock regimes (e.g., geopolitical escalation vs dovish liquidity expansion) and confirming the engine shifts capital toward Core Preservation (Gold/Cash) during high crisis risk and toward Tactical Alpha during stabilized liquidity conditions.

**Acceptance Scenarios**:
1. **Given** escalating geopolitical conflict or surging inflation data in news feeds, **When** dynamic allocation runs, **Then** Tier 3 (Core Preservation: Gold, Silver) and Tier 1 (Cash Buffer) targets increase (e.g. up to 60-70% combined), while Tier 2 Tactical Alpha risk is curtailed.
2. **Given** low macro volatility, dovish monetary policy, and stable geopolitical conditions, **When** dynamic allocation runs, **Then** Tier 2 Tactical Alpha allocation expands to capture active trading opportunities.

---

### User Story 5 - Real-Time Telegram Signals Bot (Priority: P3)

As a trader on the go, I want a Telegram bot connected to the system that broadcasts live trade signals with complete trade specifications (action, entry, SL, TP, leverage, fund allocation) and posts an automated performance summary with ROI percentage when the trade concludes.

**Why this priority**: Traders cannot monitor a web dashboard 24/7. Instant mobile alerts on Telegram allow immediate notification of high-conviction opportunities and transparent outcome tracking.

**Independent Test**: Can be tested by configuring a bot token and chat ID, triggering a trade decision event, and confirming that a formatted trade alert arrives in Telegram, followed by a closure notification calculating realized ROI upon position exit.

**Acceptance Scenarios**:
1. **Given** a new AI trade signal is confirmed, **When** dispatched, **Then** the Telegram bot sends a message containing: Symbol, Direction (LONG/SHORT), Entry Price, Take Profit, Stop Loss, Risk/Reward ratio, Recommended Leverage, and Capital Allocation (% and $).
2. **Given** an active trade that reaches its Take Profit or Stop Loss, **When** position closure occurs, **Then** the Telegram bot sends an automated resolution message reporting final exit price, hold duration, realized PnL, and ROI percentage.

---

### User Story 6 - Real-Data Machine Learning Model Training (Priority: P3)

As a quantitative researcher, I want the backend ML models and scoring parameters trained strictly on real historical market data (real Binance candles and actual historical macroeconomic news) rather than simulated mockups or synthetic data.

**Why this priority**: Models trained on synthetic or mock data produce unrealistic win rates and fail catastrophically in live market conditions due to volatility clustering and fat-tailed distribution.

**Independent Test**: Can be tested by triggering the historical training pipeline, verifying it downloads real historical kline datasets from Binance and pairs them with real news/calendar events, producing backtested parameter weights with verifiable historical metrics.

**Acceptance Scenarios**:
1. **Given** the training pipeline execution, **When** training is initialized, **Then** it pulls authentic historical price bars and real news headlines, rejecting any synthetic or mock datasets.
2. **Given** completed training on real market data, **When** model weights and Bayesian priors are updated, **Then** the system logs the training dataset metadata, sample size, date ranges, and verified out-of-sample performance metrics.

---

### User Story 7 - PWA Install Prompt & iOS Home Screen Guidance (Priority: P4)

As a mobile trader, I want an install prompt to appear on supported desktop/Android browsers, and a guided step-by-step walkthrough on iOS devices explaining how to tap "Share" and "Add to Home Screen" so that the app installs as a native full-screen PWA.

**Why this priority**: iOS Safari does not support automated `beforeinstallprompt` banners. Without visual guidance, iOS users do not know how to install the app or achieve full-screen standalone trading.

**Independent Test**: Can be tested on a desktop/Android browser to verify the install prompt banner, and simulated on iOS user-agent to verify the interactive iOS visual walkthrough popup appears.

**Acceptance Scenarios**:
1. **Given** a user visiting on an Android or Chromium browser where the PWA is not yet installed, **When** the page loads, **Then** an elegant install banner appears offering a 1-tap "Install App" button.
2. **Given** a user visiting on an Apple iOS Safari device, **When** the page loads, **Then** a modal appears with visual step-by-step guidance showing the iOS Share icon and the "Add to Home Screen" action.
3. **Given** the app is running in standalone PWA mode (`display-mode: standalone`), **When** loaded, **Then** install banners and guidance prompts are automatically hidden.

---

### Edge Cases

- What happens if Telegram API is unreachable or rate-limited?
  - Signals must still execute locally in the engine; Telegram failures are logged as non-blocking warnings with exponential backoff retries.
- What happens if news feeds are temporarily unavailable?
  - The system defaults to neutral macro sentiment, prevents opening new high-leverage positions, and alerts the operator.
- What happens during an extreme flash crash or geopolitical black swan?
  - The 10% maximum portfolio drawdown breaker and the Tier 1 cash buffer prevent margin liquidations across remaining capital.
- How does authentication behave on mobile PWA after app restart?
  - Sessions persist securely in encrypted local storage / secure cookies with configurable session expiry (e.g. 7 days).

## Requirements *(mandatory)*

### Functional Requirements

#### Authentication & System Security
- **FR-001**: System MUST provide a dedicated login screen requiring password authentication before granting access to web dashboards or operational APIs.
- **FR-002**: System MUST hash all stored passwords using industry-standard cryptographic functions (Argon2id or bcrypt with high work factor).
- **FR-003**: System MUST protect against brute-force attacks via IP-based and username-based rate limiting.
- **FR-004**: Database connections and storage MUST use parameterized queries exclusively to prevent SQL injection, and database credentials MUST be loaded strictly via environment variables.

#### Two-Sided Futures Trading
- **FR-005**: Trading engine MUST support both `LONG` and `SHORT` trade recommendations and execution commands.
- **FR-006**: Every trade signal MUST calculate and specify:
  1. Direction (`LONG` or `SHORT`)
  2. Exact entry price / limit order boundary
  3. Strict Stop-Loss (SL) price
  4. At least one Take-Profit (TP) price target
  5. Mathematical Risk-to-Reward (R:R) ratio ($\ge 1:2$)
  6. Recommended leverage multiplier (1x to 10x max based on volatility)
  7. Capital allocation amount in USD and percentage of available Tier 2 Tactical Alpha funds.
- **FR-007**: System MUST enforce a maximum risk per trade constraint (default $\le 2.0\%$ of portfolio equity) to prevent excessive capital loss on any single trade.

#### News-Catalyzed Alpha & Indicator Roles
- **FR-008**: System MUST utilize verified news catalysts and macroeconomic developments as the primary motive for initiating short-term alpha trades.
- **FR-009**: Technical indicators (RSI, SuperTrend, MACD, Order Book Imbalance, Cumulative Volume Delta) MUST be utilized to validate trend alignment, calculate optimal entry pullbacks, set stop-loss levels, and compute take-profit targets rather than serving as independent buying justification.

#### Real-World Dynamic Macro Allocation
- **FR-010**: Capital allocator MUST dynamically adjust the 3-Tier targets (Cash Buffer, Tactical Alpha, Core Preservation) based on real-world macroeconomic indicators, central bank rate trajectory, inflation rates, and geopolitical conflict sentiment.
- **FR-011**: Under elevated geopolitical tension or macroeconomic crisis, Core Preservation (Gold/Silver) and Cash Buffer targets MUST automatically increase, while Tactical Alpha exposure is conservatively throttled.

#### Real-Data ML Training Pipeline
- **FR-012**: Backend ML and Bayesian weight optimization MUST ingest and train on authentic historical candlestick datasets and real news events from live exchange APIs (Binance), strictly disallowing synthetic mock data in production training.
- **FR-013**: Training routines MUST produce verifiable model artifacts, logging data date ranges, sample counts, loss metrics, and out-of-sample backtest performance.

#### Telegram Signals Bot Integration
- **FR-014**: System MUST integrate with Telegram Bot API via configurable bot token and chat ID.
- **FR-015**: System MUST broadcast a formatted signal card to Telegram whenever a validated trade signal is generated.
- **FR-016**: System MUST broadcast an automated resolution update to Telegram upon position closure, detailing final exit price, hold duration, realized profit/loss, and ROI percentage.

#### PWA Installation & Mobile Experience
- **FR-017**: Web frontend MUST detect Chromium/Android environments and surface an intuitive "Install Simple-Trader" prompt banner when uninstalled.
- **FR-018**: Web frontend MUST detect iOS Safari environments and display an interactive visual walkthrough instructing users to tap "Share" and "Add to Home Screen".
- **FR-019**: Install banners MUST automatically dismiss when the app detects it is running in standalone PWA mode.

### Key Entities

- **UserSession**: Represents authenticated session credentials, token expiration, and client IP.
- **FuturesTradeSignal**: Represents a two-sided trade opportunity with symbol, direction (`LONG`/`SHORT`), entry, SL, TP, R:R ratio, leverage, allocated capital ($ and %), catalyst news headline, and timestamp.
- **MacroRegimeSnapshot**: Represents the current global economic health score, geopolitical risk index, active inflation tier, and the dynamically derived 3-tier target allocation percentages.
- **TelegramNotification**: Represents queued or transmitted outbound messages to the Telegram channel with message ID, delivery status, and retry counts.
- **ModelTrainingRun**: Records an ML training session with dataset date range, asset symbols, sample size, prior weights, posterior weights, and validation Sharpe ratio.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of unauthenticated attempts to access trading endpoints or dashboards are intercepted and redirected to the login interface.
- **SC-002**: Every generated trade signal has a mathematically verified Risk-to-Reward ratio of $\ge 2.0$ and capital risk $\le 2.0\%$ of portfolio equity.
- **SC-003**: In simulated and historical downtrends, the engine successfully generates and executes SHORT signals, achieving positive returns in bear market regimes.
- **SC-004**: During simulated geopolitical/inflation shock events, dynamic allocation increases defensive asset allocation within 60 seconds of news headline ingestion.
- **SC-005**: Telegram trade signals are dispatched and delivered within 3 seconds of signal generation.
- **SC-006**: Model training consumes strictly real market data from Binance APIs with zero reliance on synthetic random candle generators.
- **SC-007**: 100% of iOS Safari visits present clear home screen installation instructions unless already in standalone mode.

## Assumptions

- The operator will supply a valid Telegram Bot Token and Chat ID via environment variables if Telegram alerts are enabled.
- The administrator will configure an initial admin password in environment variables (`ADMIN_PASSWORD`).
- Market data fetchers have outbound HTTPS access to Binance API endpoints for historical candlestick downloads.
- Modern iOS devices support WebKit PWA capabilities via Add to Home Screen.
