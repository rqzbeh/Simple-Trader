# Tasks Breakdown: Production Perfection & Jim Simons Quantitative Standard

**Feature ID**: `010-simons-standard-perfection`  
**Phase**: Complete  
**Status**: VERIFIED & DEPLOYED  

---

## Dependency Order & Execution Checklist

- [x] **Phase 1: Asset Catalog & Inconsistency Elimination**
  - [x] Task 1.1: Purge all remaining `XAUT/USDT` references from comments and mappings in `internal/market/assets.go`, `internal/market/multi_source.go`, `internal/server/signal_handlers.go`.
  - [x] Task 1.2: Update `web/src/components/MLTrainingView.tsx` to dynamically render all 25 supported assets without XAUT.
  - [x] Task 1.3: Update `web/src/components/FuturesSignalsView.tsx` to dynamically render all 25 supported assets in the dropdown selector.
  - [x] Task 1.4: Update `internal/market/screener.go` candidate pairs and criteria to properly handle the 25 assets.

- [x] **Phase 2: Live Market Feed & SSE $0 Pricing Fix**
  - [x] Task 2.1: In `internal/cache/keys.go`, dual-tag `TickerQuote.Change24h` with both `change24h` and `change_24h` for seamless serialization.
  - [x] Task 2.2: In `internal/server/sse.go`, update `ServeHTTP` to immediately broadcast all current cached quotes upon client connection.
  - [x] Task 2.3: In `web/src/hooks/useSSE.ts`, read both `tick.change24h` and `tick.change_24h`, and restore default `targetCorePct: 0.60`, `targetAlphaPct: 0.40` in `INITIAL_SUMMARY`.
  - [x] Task 2.4: Verify `bun test` in `web/` passes cleanly.

- [x] **Phase 3: Authentic Live Quantitative Indicators Engine**
  - [x] Task 3.1: In `internal/indicators/`, implement `BuildSnapshot` to compute RSI, MACD, SuperTrend, Bollinger Bands, ATR, Garman-Klass, Parkinson, Kaufman ER, CMF, Market Regime, and Confluence Score from authentic candles.
  - [x] Task 3.2: In `internal/server/server.go` and `signal_handlers.go`, implement `CalculateAndCacheIndicatorSnapshot` and call it on authentic klines before AI decision evaluation.
  - [x] Task 3.3: Remove all fake mock indicator snapshots in `internal/server/signal_handlers.go` and `internal/trader/providers.go`.

- [x] **Phase 4: Authentic Live Economic Calendar Feed**
  - [x] Task 4.1: In `internal/market/calendar.go`, implement `FetchLiveMacroEvents` using `https://nfs.faireconomy.media/ff_calendar_thisweek.json`.
  - [x] Task 4.2: Replace all mock events in `internal/server/server.go` and `calendar.go` with live calendar data.
  - [x] Task 4.3: Add unit tests in `internal/market/calendar_test.go` verifying live macro parsing.

- [x] **Phase 5: Dynamic Configuration & Hardcoded Constants Purge**
  - [x] Task 5.1: In `internal/config/config.go`, add `.env` parameters for Kelly fraction, risk floors/ceilings, max concurrent signals, impact factor, and calendar halt window.
  - [x] Task 5.2: In `internal/ai/client.go`, update `fallbackHeuristic` to read bounds from configuration rather than hardcoded magic numbers.
  - [x] Task 5.3: Update `docker-compose.yml`, `docker-compose.prebuilt.yml`, and `.env.example` with all dynamic parameters.

- [x] **Phase 6: GPU Machine Learning Training**
  - [x] Task 6.1: Run `ml/train_max_acc.py` using local NVIDIA RTX 2060 GPU to maximize real out-of-sample prediction accuracy.
  - [x] Task 6.2: Ensure checkpoints and metrics in `models/` are updated.

- [x] **Phase 7: Full Stack Verification, Hardening & VPS Deployment**
  - [x] Task 7.1: Run `go test -race ./...` across all packages.
  - [x] Task 7.2: Run `bun run build` in `web/` to produce production assets.
  - [x] Task 7.3: Rewrite `README.md` with complete architectural documentation.
  - [x] Task 7.4: Commit changes to Git and push to GitHub repository.
  - [x] Task 7.5: Deploy to VPS `<VPS_HOST>` via SSH key `<SSH_KEY_PATH>` and verify `/health` and live terminal.
