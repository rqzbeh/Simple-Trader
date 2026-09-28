# Research: spec-016 (Phase 0) — source /tmp/timeframe-research.md

- **Decision**: timeframe = second Choice in the SAME batched Jev request (research-verified seam: `internal/trader/signals.go:719-734` builds entry questions; `jev.go` batches N questions/call).
  - Rationale: zero added latency (SC-205), calibrated distribution recorded for free.
  - Alternative: separate call — rejected (doubles latency/cost).
- **Decision**: option sets ALPHA `15m|1h|4h`, CORE `1h|4h|12h` (evidence: crypto catalyst decay 1–6h max; commodities session-spanning 4–12h+; 4–5× multi-timeframe ratio rule supports 15m execution under 1h/4h structure).
  - Alternative: free interval from Jev — rejected (unbounded, no candle-guarantee).
- **Decision**: criteria text per option embeds horizon minutes + decay guidance (FR-208): ALPHA 4h option = "only if macro catalyst, news typically dies ≤6h".
- **Decision**: horizon keyed by timeframe, not bucket — new TimeframeProfile map; static `risk_profiles.go:61-112` (60m/240m) becomes legacy default ONLY for rows without stored timeframe (FR-206 records migration).
  - Alternative: keep bucket horizon, timeframe cosmetic — rejected (feature pointless).
- **Decision**: answer validation = must be in bucket's configured set; invalid/missing → typed error → entry fails loud (FR-202). No default.
- **Decision**: config keys `TIMEFRAME_SET_ALPHA`, `TIMEFRAME_SET_CORE` (comma lists) join settings PUT allowlist; boot: parse + non-empty + all options in known-interval set {15m,30m,1h,2h,4h,6h,12h,1d} else startup error.
- **Decision**: candle fetch for signal's timeframe: reuse existing per-interval kline provider (research: `binance_historical.go:63` already interval-parameterized); server's hardcoded "1h" (server.go:221-230) moves behind per-signal interval at decay/eval points — global screener keeps 1h (out of scope: screener unchanged).
- **Unknown resolved**: reconciler — `applyDecayState` (`signal_handlers.go:1093`) must read stored timeframe; legacy fallback = one-time map (CRYPTO→1h, COMMODITY→4h) written to row at first reconciliation touch, logged.
