# Tasks: Two-Sided Futures Trading, Dynamic Macro Allocation, News-Catalyst Alpha, Auth Security, Telegram Signals, and Real-Data ML

**Input**: Design documents from `/specs/006-two-sided-futures-intelligence/`  
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/openapi.yaml`, `quickstart.md`

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: User story mapping (`[US1]`, `[US2]`, `[US3]`, `[US4]`, `[US5]`, `[US6]`)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Database migrations, security keys, and shared configuration setup

- [ ] T001 Create database schema migration in `migrations/000003_futures_auth_telegram.up.sql` for `admin_users`, `encrypted_system_secrets`, `futures_trade_signals`, `macro_regimes`, and `ml_training_runs` tables with exact constraints
- [ ] T002 [P] Create rollback migration in `migrations/000003_futures_auth_telegram.down.sql`
- [ ] T003 [P] Add configuration fields for `ADMIN_PASSWORD`, `APP_SECRET`, `TELEGRAM_BOT_TOKEN`, and `TELEGRAM_CHAT_ID` in `internal/config/config.go`
- [ ] T004 [P] Define Redis cache keys for active sessions, rate limiting, and signal queue in `internal/cache/keys.go`

---

## Phase 2: Foundational (Core Prerequisites)

**Purpose**: Security primitives and base database entities required across features

- [ ] T005 Implement AES-GCM-256 encryption and decryption helpers with 12-byte IV in `internal/auth/crypto.go`
- [ ] T006 [P] Implement database models and SQL queries for `admin_users` and `encrypted_system_secrets` in `internal/db/auth_store.go`
- [ ] T007 [P] Implement database models and SQL queries for `futures_trade_signals` and `macro_regimes` in `internal/db/signal_store.go`
- [ ] T008 [P] Implement database models and queries for `ml_training_runs` in `internal/db/ml_store.go`
- [ ] T009 Create unit tests for AES-GCM encryption and password hashing in `internal/auth/crypto_test.go`

---

## Phase 3: User Story 1 - News-First Catalyst Short-Term Alpha with Two-Sided Futures Execution (Priority: P1) 🎯 MVP

**Goal**: Enable symmetrical `LONG` and `SHORT` futures recommendations and paper execution triggered by breaking news catalysts and sized with risk-to-reward $\ge 1.5$ and max $2\%$ equity risk.

**Independent Test**: Simulate high-impact bearish headline, verify generation of a valid `SHORT` futures signal with stop-loss above resistance, take-profit with $R:R \ge 1.5$, leverage factor, and capital sizing $\le 2\%$.

- [ ] T010 [P] [US1] Implement symmetrical two-sided futures PnL and liquidation price math in `internal/trader/futures_math.go`
- [ ] T011 [P] [US1] Create unit tests for futures math (`CalculateFuturesPnL`, `LiquidationPrice`, `RiskRewardRatio`) in `internal/trader/futures_math_test.go`
- [ ] T012 [US1] Update trading engine in `internal/trader/engine.go` to support two-sided futures positions (`LONG` and `SHORT`), isolated leverage, and stop-loss/take-profit triggers
- [ ] T013 [US1] Refactor AI decision prompts and catalyst logic in `internal/ai/client.go` to require breaking news sentiment as the primary entry catalyst and indicators strictly for entry/SL/TP/leverage
- [ ] T014 [US1] Implement futures signal generation service in `internal/trader/signals.go` validating $R:R \ge 1.5$ and capital allocation $\le 2.0\%$ of portfolio equity
- [ ] T015 [US1] Implement HTTP handler `POST /api/v1/signals/futures/decide` and `GET /api/v1/signals/futures` in `internal/server/signal_handlers.go`
- [ ] T016 [P] [US1] Create React components for two-sided futures signal cards displaying direction badge, leverage, R:R, and catalyst source in `web/src/components/FuturesSignalsView.tsx`

---

## Phase 4: User Story 2 - Real-World Dynamic Macro Allocation (Priority: P2)

**Goal**: Dynamically adjust 3-Tier allocations (Cash, Tactical Alpha, Core Preservation) based on real-world geopolitical tension, inflation rates, and central bank interest rate bias.

**Independent Test**: Query `/api/v1/macro/regime` under simulated geopolitical crisis stress ($S \ge 0.65$), verifying Core expands to $\ge 55\%$, Cash expands to $\ge 20\%$, and Alpha contracts to $\le 25\%$.

- [ ] T017 [P] [US2] Implement dynamic macro stress score formula and target tier calculation in `internal/trader/macro_regime.go`
- [ ] T018 [P] [US2] Create unit tests for macro regime transitions and boundary validations in `internal/trader/macro_regime_test.go`
- [ ] T019 [US2] Integrate macro regime scoring with portfolio allocator in `internal/trader/allocator.go`
- [ ] T020 [US2] Implement HTTP endpoint `GET /api/v1/macro/regime` in `internal/server/macro_handlers.go`
- [ ] T021 [P] [US2] Build React dynamic macro allocation telemetry card with stress gauges and dynamic percentage bars in `web/src/components/MacroRegimeView.tsx`

---

## Phase 5: User Story 3 - Telegram Signals Bot with Complete Trade Lifecycle and ROI Reporting (Priority: P3)

**Goal**: Deliver real-time Telegram trade alerts with full trade parameters and automated closed-trade notifications reporting realized ROI %.

**Independent Test**: Connect Telegram bot, trigger trade signal and closure, verify Telegram channel receives formatted MarkdownV2 signal card and final ROI resolution report.

- [ ] T022 [P] [US3] Implement Telegram Bot API client with exponential backoff retries in `internal/telegram/bot.go`
- [ ] T023 [P] [US3] Implement MarkdownV2 message formatters for trade entry and trade resolution in `internal/telegram/formatter.go`
- [ ] T024 [P] [US3] Create unit tests for Telegram formatting and escaping in `internal/telegram/formatter_test.go`
- [ ] T025 [US3] Wire signal engine events and position closures to asynchronous Telegram dispatcher in `internal/trader/daemon.go`
- [ ] T026 [US3] Implement HTTP endpoints `GET /api/v1/telegram/config`, `POST /api/v1/telegram/config`, and `POST /api/v1/telegram/test` in `internal/server/telegram_handlers.go`
- [ ] T027 [P] [US3] Build Telegram bot configuration and test modal in `web/src/components/TelegramConfigModal.tsx`

---

## Phase 6: User Story 4 - Secure Web Authentication & Encrypted Database Protection (Priority: P4)

**Goal**: Restrict web dashboard and trading APIs with salted bcrypt password authentication, Redis session tokens, brute-force rate limiting, and encrypted credentials.

**Independent Test**: Visit dashboard unauthenticated, verify HTTP 401 redirect to login modal; verify 5 failed attempts trigger rate-limit lockout; verify encrypted storage in DB.

- [ ] T028 [P] [US4] Implement bcrypt password verification and cryptographically random session token generator in `internal/auth/auth.go`
- [ ] T029 [P] [US4] Implement sliding-window rate limiter (5 failed attempts per 15 min) in `internal/auth/rate_limiter.go`
- [ ] T030 [US4] Create HTTP authentication middleware intercepting protected routes with Bearer token / cookie verification in `internal/auth/middleware.go`
- [ ] T031 [US4] Implement HTTP endpoints `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, and `GET /api/v1/auth/session` in `internal/server/auth_handlers.go`
- [ ] T032 [P] [US4] Create unit and integration tests for authentication flow and rate limiting in `internal/auth/auth_test.go`
- [ ] T033 [P] [US4] Build administrative password login modal and authentication context in `web/src/components/LoginModal.tsx` and `web/src/context/AuthContext.tsx`

---

## Phase 7: User Story 5 - PWA Install Prompt & iOS "Add to Home Screen" Guide (Priority: P5)

**Goal**: Proactively guide Chromium/Android users to install Simple-Trader and display a step-by-step visual tutorial modal for iOS Safari users.

**Independent Test**: Verify Chromium surfaces install banner; verify iOS user-agent displays step-by-step modal; verify prompts hide in standalone mode.

- [ ] T034 [P] [US5] Implement device detection utility for Chromium `beforeinstallprompt`, iOS Safari, and standalone display mode in `web/src/utils/pwa.ts`
- [ ] T035 [P] [US5] Build PWA install banner component with dismiss and 1-tap install trigger in `web/src/components/PWAInstallBanner.tsx`
- [ ] T036 [P] [US5] Build iOS Safari step-by-step modal with Share icon and "Add to Home Screen" visual walkthrough in `web/src/components/IOSInstallGuideModal.tsx`
- [ ] T037 [US5] Integrate PWA install components and standalone detection into main layout in `web/src/App.tsx`

---

## Phase 8: User Story 6 - Real-Data Machine Learning Training on Historical & Live Market Series (Priority: P6)

**Goal**: Train Bayesian weights and scoring parameters strictly on authentic Binance historical candlesticks and real news archives, completely eliminating synthetic data.

**Independent Test**: Run `/api/v1/ml/train` with 1,000 Binance historical bars, verify completion with authentic timestamps, training loss, and directional accuracy logged to database.

- [ ] T038 [P] [US6] Implement authentic Binance historical kline downloader (`/api/v3/klines`) supporting 1h/3h candles in `internal/market/binance_historical.go`
- [ ] T039 [US6] Implement real-data machine learning calibration pipeline computing predictive loss and updating weights in `internal/ai/training.go`
- [ ] T040 [P] [US6] Create unit tests validating real-data ingestion and rejecting synthetic random generators in `internal/ai/training_test.go`
- [ ] T041 [US6] Implement HTTP endpoint `POST /api/v1/ml/train` in `internal/server/ml_handlers.go`
- [ ] T042 [P] [US6] Build ML training telemetry and model calibration card in `web/src/components/MLTrainingView.tsx`

---

## Phase 9: Polish & Cross-Cutting Integration

**Purpose**: End-to-end verification, documentation updates, and deployment readiness

- [ ] T043 [P] Update `scripts/verify_e2e_pipeline.py` to test auth gate, two-sided futures signals, dynamic macro allocation, and real-data ML
- [ ] T044 Build and verify frontend production bundle with `bun run build` in `web/`
- [ ] T045 Execute full Go backend test suite with `go test -v -race ./...`
- [ ] T046 Run end-to-end integration test suite against running Docker stack
- [ ] T047 Update project documentation in `README.md` reflecting new futures, auth, telegram, and PWA capabilities

---

## Dependencies & Execution Order

### Phase Dependencies
- **Phase 1 (Setup)**: Can start immediately.
- **Phase 2 (Foundational)**: Depends on Phase 1 completion. Blocks all user stories.
- **Phase 3 (User Story 1 - Futures)**: Depends on Phase 2. MVP core.
- **Phase 4 (User Story 2 - Macro)**: Depends on Phase 2.
- **Phase 5 (User Story 3 - Telegram)**: Depends on Phase 2 and Phase 3 signal structures.
- **Phase 6 (User Story 4 - Auth)**: Depends on Phase 2.
- **Phase 7 (User Story 5 - PWA)**: Independent frontend work after Phase 2.
- **Phase 8 (User Story 6 - ML Training)**: Depends on Phase 2 and Binance data fetchers.
- **Phase 9 (Polish)**: Depends on all user stories being implemented.

### Parallel Opportunities
- Foundational tasks `T006`, `T007`, `T008` can run in parallel.
- All frontend UI tasks (`T016`, `T021`, `T027`, `T033`, `T035`, `T036`, `T042`) can run in parallel once contract types are defined.
- Telegram bot (`US3`), Macro regime (`US2`), and Auth (`US4`) can be developed concurrently across isolated modules.
