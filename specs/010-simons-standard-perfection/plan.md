# Implementation Plan: Production Perfection & Jim Simons Quantitative Standard

**Feature ID**: `010-simons-standard-perfection`  
**Phase**: Implementation & Convergence  
**Status**: IN EXECUTION  

---

## 1. Technical Architecture & Component Design

### 1.1 Live Market Feed & SSE Stream Hydration
- **Initial Hydration on SSE Connect**:
  - In `internal/server/sse.go`, enhance `ServeHTTP`: on client connection, immediately stream the initial snapshot of all 25 asset quotes cached in `s.marketData` and `s.lastQuotes`.
  - In `internal/cache/keys.go`, ensure `TickerQuote` supports dual serialization (`change24h` and `change_24h`).
  - In `web/src/hooks/useSSE.ts`, read both `tick.change24h` and `tick.change_24h`. Restore default `targetCorePct: 0.60` and `targetAlphaPct: 0.40` in `INITIAL_SUMMARY` to align with `App.test.ts`.

### 1.2 Full 25-Asset Universe & Dropdown Alignment
- **Purge XAUT Duplication**:
  - Remove all remaining mentions of `XAUT/USDT` from `internal/market/assets.go`, `internal/market/multi_source.go`, `internal/server/signal_handlers.go`, and `web/src/components/MLTrainingView.tsx`.
- **Dynamic Frontend Dropdowns**:
  - In `FuturesSignalsView.tsx`, replace the 12-item hardcoded select with dynamic options generated from the `assets` list (all 25 assets).
  - In `MLTrainingView.tsx`, update select options to include all 25 assets.
  - In `internal/market/screener.go`, update `DefaultScreenerConfig` candidate pairs to all 25 assets, with appropriate commodity handling so commodities are not rejected as zero-volume crypto.

### 1.3 Authentic Live Quantitative Indicator Pipeline
- **End-to-End Indicator Builder**:
  - In `internal/indicators/`, implement `BuildSnapshot(symbol string, candles []db.Candle, weights map[string]float64) Snapshot`.
  - Calculate RSI (14), MACD (12, 26, 9), SuperTrend (10, 3.0), Bollinger Bands (20, 2.0), ATR (14), Garman-Klass (14), Parkinson (14), Kaufman ER (10), Chaikin Money Flow (20), Regime Classifier, and Confluence Score.
- **Server Cache Ingestion**:
  - In `internal/server/server.go`, implement `CalculateAndCacheIndicatorSnapshot(ctx context.Context, symbol string) (*cache.IndicatorSnapshot, error)`.
  - When evaluating an asset signal in `EvaluateSymbolSignal`, fetch authentic candles via `s.candleDownloader.FetchHistoricalKlines`, calculate the genuine snapshot, cache to Redis via `SetIndicatorSnapshot`, and pass to strategy evaluators.
  - Zero hardcoded mock values!

### 1.4 Authentic Live Economic Calendar Feed
- In `internal/market/calendar.go`:
  - Implement `FetchLiveMacroEvents(ctx context.Context) ([]MacroEvent, error)` pointing to `https://nfs.faireconomy.media/ff_calendar_thisweek.json`.
  - Parse events, map high-impact USD and EUR events to `MacroEvent`, and update calendar events.
  - Remove all mock event generation in `calendar.go` and `server.go`.

### 1.5 Dynamic Configuration via `.env`
- In `internal/config/config.go`:
  - Add `.env` fields:
    - `KELLY_FRACTION` (default `0.50`)
    - `MIN_RISK_PER_TRADE_PCT` (default `0.005`)
    - `MAX_RISK_PER_TRADE_PCT` (default `0.020`)
    - `MAX_CONCURRENT_SIGNALS` (default `5`)
    - `IMPACT_FACTOR` (default `0.05`)
    - `CALENDAR_HALT_MINUTES` (default `15`)
    - `ECONOMIC_CALENDAR_URL`
- In `internal/ai/client.go`:
  - Update `fallbackHeuristic` to dynamically use `c.cfg` values for leverage, stop-loss, take-profit, and risk parameters.

### 1.6 GPU ML Maximizer Training
- In `ml/train_max_acc.py`:
  - Train Deep Residual Alpha Network with Purged Walk-Forward Cross Validation on local NVIDIA RTX 2060 GPU with CUDA acceleration.
  - Generate updated high-accuracy model weights and metrics JSON.

### 1.7 Testing, Docker, Git & VPS Deployment
- Pass `bun test` in `web/`.
- Pass `go test -race ./...` in backend.
- Build frontend via `bun run build`.
- Build container via Docker Compose.
- Commit and push to GitHub repository.
- Deploy to VPS `<VPS_HOST>` via SSH key `<SSH_KEY_PATH>`.
- Verify `/health` on live VPS.
