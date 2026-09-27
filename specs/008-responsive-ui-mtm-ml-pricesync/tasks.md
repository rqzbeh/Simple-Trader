# Tasks: Responsive UI, Dynamic Mark-to-Market, ML Integration, and Price Synchronization

**Feature Branch / Spec ID**: `008-responsive-ui-mtm-ml-pricesync`

---

## Phase 1: Frontend Responsive UI & Mobile Navigation (`web/src/App.tsx`)
- [ ] **Task 1.1**: Add mobile navigation state (`isMobileMenuOpen`) and responsive mobile menu button (hamburger / X toggle) in the top header.
- [ ] **Task 1.2**: Implement mobile navigation drawer/dropdown displaying all tabs (`Terminal`, `Investor Ledger`, `Liquid Screener`, `News Trading`, `AI Weights`, `ML Engine`) with icons and touch targets.
- [ ] **Task 1.3**: Add `MLTrainingView` import and render it when `activeTab === 'ml'`, giving full UI access to the GPU/Bayesian ML training pipeline.
- [ ] **Task 1.4**: Ensure viewport scaling fits cleanly across mobile, tablet, and desktop screens without broken overflow.

## Phase 2: Dynamic Mark-to-Market & Profit Badges (`web/src/App.tsx` & `web/src/hooks/useSSE.ts`)
- [ ] **Task 2.1**: In `App.tsx`, calculate dynamic total return percentage based on current `summary.totalEquity` vs initial capital ($100,000 baseline or dynamic baseline).
- [ ] **Task 2.2**: Replace hardcoded `+2.45% All-Time` with dynamic `${returnPct >= 0 ? '+' : ''}${returnPct.toFixed(2)}% All-Time` badge with dynamic emerald/rose styling.
- [ ] **Task 2.3**: In `web/src/hooks/useSSE.ts`, verify that `tick` events recalculate position PnL and total equity dynamically from live market prices.

## Phase 3: Machine Learning & Autonomous Trading Daemon Wiring (`cmd/trader/main.go`)
- [ ] **Task 3.1**: In `cmd/trader/main.go`, instantiate `trader.NewTradingDaemon` with `allocator`, `execEngine`, `aiClient`, and `broadcaster`.
- [ ] **Task 3.2**: Wire market feed ticks to call `daemon.ProcessTick(ctx, tick)` in a background goroutine so autonomous AI evaluation runs continuously.

## Phase 4: Market Price & AI Signal Synchronization (`internal/server/signal_handlers.go` & `server.go`)
- [ ] **Task 4.1**: Update `internal/server/signal_handlers.go` to resolve current prices from the active market feed / quote store, ensuring AI trade signals reflect live ticker prices.
- [ ] **Task 4.2**: Ensure `internal/server/server.go` backtest/signal handlers align with live quotes.

## Phase 5: Pure Crypto Universe (USDT/USDC) & Leverage Suggestions
- [ ] **Task 5.1**: Convert all assets in `internal/market/assets.go`, `feed.go`, and frontend `useSSE.ts` to pure crypto USDT/USDC pairs (BTC/USDT, ETH/USDT, SOL/USDT, PAXG/USDT, XAG/USDT, OIL/USDT, EURC/USDC). Remove forex.
- [ ] **Task 5.2**: Add `leverage` to `TradePosition` in `types/index.ts` and `PositionsTable.tsx`. Ensure trade signals suggest realistic leverage (e.g., 5x, 10x, 20x) and display leverage badges.

## Phase 6: UI Simplification & Decluttering
- [ ] **Task 6.1**: Streamline `App.tsx` and terminal view by removing excessive backend-only internal noise.
- [ ] **Task 6.2**: Simplify portfolio metric banners and layout for a clean, professional, high-performance crypto trading terminal.

## Phase 7: Verification & Quality Assurance
- [ ] **Task 7.1**: Build frontend (`bun run build` in `web/`) and verify no TypeScript or bundling errors.
- [ ] **Task 7.2**: Build and run Go test suites (`go test ./...` in root) to ensure all tests pass.
- [ ] **Task 7.3**: Verify end-to-end functionality of responsive navigation, dynamic MtM, ML tab, pure crypto pairs, leverage display, and price synchronization.

