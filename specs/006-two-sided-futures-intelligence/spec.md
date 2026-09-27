# Feature Specification: Two-Sided Futures Trading, Dynamic Macro Allocation, News-Catalyst Short-Term Alpha, PWA Install & Auth Security, Telegram Signals Bot, and Real-Data ML Training

**Feature Branch**: `006-two-sided-futures-intelligence`

**Created**: 2026-09-20

**Status**: Draft

**Input**: User description: "Two-Sided Futures Trading, Real-World Dynamic Macro Allocation, News-Catalyst Short-Term Alpha, PWA Install & iOS Guidance, Auth Security, Telegram Signals Bot, and Real-Data ML Training"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - News-First Catalyst Short-Term Alpha with Two-Sided Futures Execution (Priority: P1)

As a quantitative trader, I want news catalysts (breaking economic news, whale alerts, and geopolitical/war events) to be the primary trigger for entering short-term trades, while technical indicators serve exclusively to define entry price, leverage, stop-loss, and take-profit targets. Furthermore, I want the engine to evaluate both long (buy) and short (sell) futures opportunities so the system can profit in declining as well as rallying markets.

**Why this priority**: Indicators lag price action; news catalysts drive instantaneous market repricing. Limiting trades to buy-only cuts trading opportunity in half and prevents hedging during market downturns or geopolitical crises.

**Independent Test**: Simulate or ingest a major bearish news headline (e.g., regulatory ban or exchange insolvencies). Verify that the engine initiates a high-conviction **SHORT** signal, calculates an optimal stop loss, take profit, and risk-to-reward ratio (e.g., minimum 1:2.0), allocates a specific fraction of the available short-term capital, and records the trade.

**Acceptance Scenarios**:
1. **Given** a high-impact news catalyst (bullish or bearish), **When** the short-term alpha engine evaluates the asset, **Then** it generates a trade decision with direction (`LONG` or `SHORT`), specific capital allocation (e.g. 2% of Tier 2 alpha funds), entry level, stop loss, take profit, and a verified risk-to-reward ratio of at least 1:1.5.
2. **Given** no news catalyst or ambiguous/neutral news, **When** indicators signal overbought or oversold conditions, **Then** the engine holds and does not enter a trade purely on indicator readings.
3. **Given** a bearish news catalyst and confirming technical indicators, **When** evaluated, **Then** the engine emits a `SELL` / `SHORT` futures position with corresponding risk controls.

---

### User Story 2 - Real-World Dynamic Macro Allocation (Priority: P2)

As a fund risk manager, I want capital allocation across Cash, Short-Term Alpha, and Core Preservation to dynamically adjust based on empirical macroeconomic realities—such as ongoing wars, inflation trajectories, and central bank interest rate decisions—rather than relying on hardcoded static percentages.

**Why this priority**: Static capital weights fail during systemic macro shocks (e.g. escalating conflicts or rapid rate hikes). Dynamic allocation rooted in real economic data protects principal and concentrates capital into safe-haven assets (gold, cash) during conflict or into tactical alpha during calm expansion.

**Independent Test**: Query the allocation status while injecting real geopolitical stress and rate environment metrics. Verify that the Core Preservation (gold/silver) and Cash buffers expand dynamically while Tactical Alpha reduces its exposure to high-beta crypto.

**Acceptance Scenarios**:
1. **Given** rising geopolitical conflict / war headlines or high macroeconomic volatility, **When** the allocation engine recalculates targets, **Then** Core Capital (Gold/Treasury proxies) and Tier 1 Cash targets automatically expand, and Tactical Alpha risk targets automatically contract.
2. **Given** a tranquil macroeconomic regime with favorable liquidity, **When** recalculated, **Then** Tactical Alpha expands to capture short-term momentum while maintaining minimum cash safety constraints.

---

### User Story 3 - Telegram Signals Bot with Complete Trade Lifecycle and ROI Reporting (Priority: P3)

As a remote trader, I want to connect a Telegram bot that broadcasts immediate, actionable signals (asset, LONG/SHORT, entry price, stop-loss, take-profit, leverage, capital allocation) and follows up with an automated closed-trade summary detailing the final Return on Investment (ROI) and profit/loss.

**Why this priority**: Traders cannot watch the dashboard 24/7. Immediate push notifications on mobile via Telegram allow human oversight and transparency into how every algorithmic signal performed.

**Independent Test**: Configure a Telegram Bot Token and Chat ID. Trigger a trade signal from the engine and verify Telegram receives the formatted signal message. Close the trade and verify Telegram receives the completed trade report with exact ROI percentage and dollar PnL.

**Acceptance Scenarios**:
1. **Given** an active Telegram configuration, **When** a trade signal is generated, **Then** the bot posts a structured message specifying Asset, Action (`LONG`/`SHORT`), Entry Price, Leverage, Stop Loss, Take Profit, Capital Allocation ($ and %), and Risk/Reward Ratio.
2. **Given** an open position triggered by a signal, **When** the trade hits take-profit, stop-loss, or manual exit, **Then** the bot transmits a resolution message showing final exit price, hold duration, and net ROI %.

---

### User Story 4 - Secure Web Authentication & Encrypted Database Protection (Priority: P4)

As an administrator, I want the web interface and API protected by a secure authentication screen with password hashing and database encryption so unauthorized internet actors cannot view fund balances, trigger trades, or tamper with investor records.

**Why this priority**: The platform manages real investor capital and trade execution. Leaving endpoints public or storing credentials in plaintext creates critical security vulnerabilities.

**Independent Test**: Access the web terminal in an unauthenticated browser state. Verify that a clean login screen appears and all API requests return HTTP 401 Unauthorized until valid administrative credentials are provided. Verify that stored passwords utilize bcrypt/argon2 hashing and sensitive credentials are encrypted.

**Acceptance Scenarios**:
1. **Given** an unauthenticated visitor, **When** navigating to the dashboard or calling private API endpoints, **Then** access is denied with a login challenge.
2. **Given** valid credentials, **When** logging in, **Then** a cryptographically signed session token is issued with secure cookie/header attributes.
3. **Given** invalid credentials or brute-force attempts, **When** attempted, **Then** the system rate-limits and rejects the requests without leaking system details.

---

### User Story 5 - PWA Install Prompt & iOS "Add to Home Screen" Guide (Priority: P5)

As a mobile trader, I want the browser to proactively prompt me to install Simple-Trader as a standalone app, and on iOS Safari, display a clean modal guiding me to tap "Share" and "Add to Home Screen".

**Why this priority**: Simple-Trader is a Progressive Web App (PWA) designed to provide native-app performance without app store restrictions. Users on desktop and mobile need intuitive installation prompts.

**Independent Test**: Open the web application on Chromium and verify the native install banner/modal triggers. Open on iOS Safari and verify the step-by-step visual tooltip modal displays the share icon and "Add to Home Screen" instructions.

**Acceptance Scenarios**:
1. **Given** a Chromium-based browser supporting `beforeinstallprompt`, **When** the user loads the app, **Then** a dismissible install banner or prompt is offered.
2. **Given** an iOS Safari user agent, **When** the app is opened outside standalone mode, **Then** a guidance banner displays the Safari Share button icon and "Add to Home Screen" instruction.
3. **Given** the app is already installed and launched in standalone mode, **Then** neither install prompt nor iOS guidance modal is displayed.

---

### User Story 6 - Real-Data Machine Learning Training on Historical & Live Market Series (Priority: P6)

As a quantitative researcher, I want the backend machine learning model trained exclusively on verified historical market data and real news-reaction series (no mockups, synthetic or fake numbers) so that probability estimations and weight adjustments reflect genuine market dynamics.

**Why this priority**: Models trained on mock data develop hallucinations and unrealistic win-rate expectations, resulting in catastrophic drawdown during live market execution.

**Independent Test**: Execute the training pipeline against downloaded Binance 1h/3h historical candle bars and ingested real news archives. Verify that indicator weights, regret minimization parameters, and win probabilities are fitted against actual historical returns.

**Acceptance Scenarios**:
1. **Given** authentic historical OHLCV data from Binance/Yahoo Finance and actual news events, **When** the model training module runs, **Then** it computes loss, updates model weights, and saves the calibrated model artifact.
2. **Given** the training completes, **When** evaluated against a holdout test dataset, **Then** performance metrics (MAE, Log-Loss, Directional Accuracy) are reported accurately without mock data injection.

---

### Edge Cases

- **Extreme Flash Crash or Black Swan News**: If a catastrophic geopolitical event occurs, news sentiment drops to extreme negative values; the system must execute circuit breakers or hedge with short positions, never risking more than the user-configured max risk per trade (e.g. 2%).
- **Telegram API Outage or Rate Limit**: If Telegram's servers return HTTP 429 or timeout, signal dispatch must be queued and retried asynchronously without blocking the core Go trading execution engine.
- **Concurrent Long and Short Signals**: If different indicators or news timeframes present conflicting directional bias, the engine must prioritize the higher-horizon macro regime or remain neutral (HOLD).
- **Session Expiry Mid-Trade**: If an admin's web authentication token expires while viewing active trades, ongoing background autonomous trading must remain unaffected on the server.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support two-sided futures trading, generating both `LONG` (buy) and `SHORT` (sell) recommendations and orders.
- **FR-002**: System MUST use news catalysts (macro news, whale alerts, political disclosures) as the primary entry rationale for short-term trades, utilizing technical indicators (RSI, SuperTrend, MACD, Bollinger Bands, CVD) primarily for entry pricing, leverage factor, stop-loss, and take-profit calculation.
- **FR-003**: Every trade recommendation MUST explicitly specify:
  - Asset pair (e.g., `BTC/USD`, `SOL/USD`, `XAU/USD`)
  - Order Direction (`LONG` or `SHORT`)
  - Entry Price
  - Suggested Leverage (e.g., 1x to 10x based on volatility)
  - Stop-Loss Price and Take-Profit Price
  - Calculated Risk-to-Reward Ratio ($R:R \ge 1:1.5$)
  - Capital Allocation ($ amount and % of available tactical alpha fund)
- **FR-004**: System MUST dynamically calibrate the 3-Tier Liquidity Allocation based on real-world economic indicators, inflation news, interest rate policies, and geopolitical tension indices, adjusting Tier 1 (Cash), Tier 2 (Alpha), and Tier 3 (Core) weights programmatically.
- **FR-005**: System MUST include a secure authentication gateway requiring administrator login with salted bcrypt password verification and cryptographically signed session tokens.
- **FR-006**: System MUST encrypt sensitive database fields (API keys, Telegram bot tokens, private investor notes) at rest using AES-GCM-256.
- **FR-007**: System MUST provide a customizable Telegram Bot integration that:
  - Broadcasts formatted entry signals with full trade parameters.
  - Broadcasts exit notifications when a trade is closed, stating final exit price, holding period, and net Return on Investment (ROI %).
  - Provides basic Telegram command interactions (e.g. `/status`, `/allocations`, `/signals`).
- **FR-008**: System MUST display an intuitive PWA install prompt on supported desktop/Android browsers, and a specialized step-by-step "Add to Home Screen" tutorial modal for iOS Safari users.
- **FR-009**: System MUST train the ML scoring and fine-tuning engine strictly on authentic historical market datasets and verified news-impact pairs, prohibiting any synthetic or mockup data generation in training.

---

### Key Entities

- **TradeSignal**: Represents a two-sided trade opportunity containing symbol, direction (`LONG`/`SHORT`), catalyst source, entry price, stop-loss, take-profit, leverage, allocated capital percentage, risk-to-reward ratio, confidence score, and timestamp.
- **MacroEconomicRegime**: Captures dynamic world situation metrics, including Geopolitical Stress Index, Interest Rate Bias, Inflation Trajectory, and calculated target tier allocations.
- **TelegramSubscription**: Stores configured Telegram bot token, authorized chat IDs, enabled notification tiers, and delivery status logs.
- **AdminUser**: Stores administrator credentials with bcrypt password hash, role-based permissions, and last login audit timestamps.
- **MLTrainingDataset**: Contains authentic historical OHLCV records and timestamped real news headlines with observed market return outcomes used for model calibration.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Two-sided trading allows system to capture profitable short trades during market downswings with a minimum risk-to-reward ratio of 1:1.5 on all suggested positions.
- **SC-002**: Capital allocation per single trade is strictly bounded to $\le 2.0\%$ portfolio equity risk, preventing single-trade catastrophic drawdown.
- **SC-003**: Telegram signals are delivered to authorized channels within 2.0 seconds of engine signal generation.
- **SC-004**: Unauthenticated access to private trading dashboards or management APIs is blocked with 100% enforcement.
- **SC-005**: iOS Safari and Android/Desktop users receive context-aware app installation guidance within 3 seconds of initial page visit.
- **SC-006**: ML models are trained on $\ge 1,000$ authentic historical market bars with documented loss and directional validation metrics.

---

## Assumptions

- Users wishing to receive Telegram notifications have registered a Telegram bot via `@BotFather` and have a target Chat ID or Channel ID.
- Real-time and historical market data are sourced from public exchange APIs (e.g. Binance US/Global public endpoints and Yahoo Finance) without requiring paid API subscriptions.
- Initial admin credentials can be configured via environment variables with immediate prompt to change default credentials.
- Browser service worker registration operates over HTTPS or `localhost` as required by modern Web standards.
