# Phase 0 Research: Trading System Optimization

Date: 2026-09-26 · Source material: `docs/RESEARCH-trading-system-optimization.md`

## R1. Entry gate inputs (G1)

**Decision**: Gate = (a) trend/session alignment using existing VWAP + SuperTrend(10,3) flip state,
(b) catalyst-candle volume ≥ 2.5× the 20-period 5m volume SMA, (c) anti-chase: block if last
price > 2σ above 15m VWAP (Bollinger upper band on VWAP residuals — implemented as price vs
Bollinger(20,2) of 15m closes), (d) optional OI-delta check (price↑+OI↓ ⇒ trap ⇒ block).

**Rationale**: All inputs except OI already exist in `internal/indicators`. OI comes from the
exchange futures `openInterestHist` endpoint (5m step, last 3 intervals) with a 500ms bounded
timeout; gate degrades to "no OI data ⇒ neutral" so entry path never blocks.

**Alternatives**: (1) LLM-only gate — rejected, non-deterministic and untestable; (2) full order-book
microstructure gate — rejected, OBI/CVD exist but add latency for marginal edge; (3) no OI at all —
rejected, OI delta is the cheapest trap detector (arXiv:2606.12210).

## R2. Stop / target / time-decay defaults (G2, G3)

**Decision** (all config keys under `risk.profile.<asset_class>`):
- `sl_atr_mult = 2.0` (1h ATR), `sl_swing_offset = 0.25` ATR behind 15m swing low/high
- `sl_min_pct = 0.6`, `sl_max_pct = 2.5` (keep current clamps as guardrails)
- `tp1_atr_mult = 1.0`, `tp1_close_frac = 0.6`, `tp2_atr_mult = 1.8`
- trigger: 5m candle **close** beyond level (not touch), mark-price preferred when available
- `decay_breakeven_at_min = 30` (move SL to entry+fees if PnL < +0.5R), `decay_flat_at_min = 40`
  (close if PnL ≤ 0), both < `horizon_min = 60`
- `rr_floor`: removed as hard constraint; RR reported as diagnostic only

**Rationale**: Empirical 1h MFE p90 = 0.63% (BTC) / 1.08% (SOL) makes the current forced
+2.5% TP a 4-sigma target (0% hits in 33 trades). 2×ATR ≈ 0.9–1.5% keeps stops outside p90
hourly range noise while risk budget caps damage via sizing.

**Alternatives**: structure-only stops (rejected: no volatility adaptation); fixed 1.1% (rejected —
status quo); k-percentile stops (rejected: needs long lookback, marginal gain over ATR at 1h).

## R3. Leverage & liquidation buffer (G4)

**Decision**: `lev = clamp(1, 8, target_hourly_vol / atr_pct)` with `target_hourly_vol = 3.5%`;
add invariant `liq_distance_pct / sl_pct >= 4` where `liq_distance ≈ 1/lev − maintenance_margin`
(margin from exchange per-symbol table, default 0.5%). Violation ⇒ step leverage down until pass.

**Rationale**: Equalizes ROE risk per stop across BTC (ATR 0.5%) and DOGE-class alts (ATR 1.2%);
current fixed 8x makes alt stops −9% ROE in one wick.

**Alternatives**: vol-target on position notional (rejected: conflates with sizing); per-exchange
bracket orders (rejected: system simulates PnL, no live exchange account coupling).

## R4. Position sizing (G7 partially)

**Decision**: keep `risk = equity × 1.5%` fixed-fractional; `notional = risk / (sl_pct + 0.1%)
/100`; margin = notional/lev; clamps: ≤20% equity margin, ≤ remaining tier budget, plus **slot
budget removal** — commodities/crypto tier budgets become caps, not fixed $8k slots.

**Rationale**: Observed $8,000 fixed margins show budget-as-slot behavior; sizing must track stop
distance (FR-007). ¼-Kelly cap computed as advisory telemetry only (allocator's Kelly function
already exists).

## R5. News clustering / decay / veto (G5, G8)

**Decision**: Redis-backed rolling clusterer: normalize title (lowercase, strip punctuation/outlets)
→ token 3-gram Jaccard ≥ 0.82 within 45-min window → cluster with `story_count`, source tier
weights (1.0 primary wires / 0.6 crypto press / 0.2 scrapers), freshness
`w = exp(−ln2·Δt/15min)` for crypto profile and `/60min` for commodity profile, discard past
`2× half-life`. Polarization veto: `P > 0.40` ⇒ no trade. Cluster id links to all signals.

**Rationale**: Kills the 6×-duplicate-long failure (same headline syndicated over 8 RSS feeds);
title-level similarity is enough because feeds retitle minimally; no embeddings vendor (constraint:
no new third-party services).

**Alternatives**: LLM-based merge call per batch (rejected: cost/latency/nondeterminism);
embedding similarity via existing AI gateway (rejected for v1 — kept as noted follow-up if
Jaccard proves weak on heavily-reworded stories).

## R6. Confidence vs news sentiment (G6)

**Decision**: keep `catalyst_sentiment` for the fused news score (computed by R5 aggregation);
add `model_confidence` column for the AI's signed confidence. Migration backfills
`catalyst_sentiment` with `model_confidence` values where they differ is NOT performed — old rows
are marked `legacy_sentiment` (assumption: historical semantics accepted as-is).

**Rationale**: FR-012; avoids rewriting history while making new data honest.

## R7. Core-commodities zero-signal diagnosis (G7 / FR-013)

**Decision**: hypotheses to test during implementation with targeted instrumentation + a replay
command (not resolvable from static reading alone):
- H1 scan order/slot starvation: crypto ALPHA candidates evaluated before CORE; budget slots
  consumed first (`scanUniverse()` ordering).
- H2 macro/regime gate vetoing commodities silently.
- H3 catalyst scarcity: RSS set is crypto-centric; commodity keywords rare + freshness window.
- H4 NEWS-CATALYST-FIRST prompt rule ⇒ AI outputs HOLD when few/no commodity headlines pass filter.

**Resolution path**: run evaluation loop in dry-run with per-stage logging, feed synthetic gold/Hawk
headlines, observe where the branch dies; fix the actual stage (scope of FR-013).

**Alternatives**: none — diagnosis is mandatory before building the commodity profile on top.

## R8. Commodity profile semantics (G7)

**Decision**: Commodity profile defaults: `horizon_min = 240` (4h) with `horizon_max = 720`,
`sl_atr_mult = 2.5`, half-life 60min, plus **event blackout calendar** (NFP/CPI/FOMC ±30min,
EIA ±15min — static UTC calendar table, config file) and **reference-market gap guard**: close
signals 15min before CME weekend close (Fri 16:45 ET) since tokenized instruments track
reference-market gaps at reopen. Weekend flat is a profile flag, default on.

**Rationale**: Our commodities are tokenized (Binance 24/7) — literal CME session enforcement is
impossible, but the *risk* the sessions model (report shocks, weekend gaps) still transfers.
Longer horizon matches commodity momentum decay (research §2.7).

**Alternatives**: full session simulation (rejected: instrument never closes); no gap guard
(rejected: operator explicitly wants war/emergency-long-term semantics safe).

## R9. True PnL persistence & migration (G-history / SC-001)

**Decision**: one-time idempotent SQL migration/backfill computing
`roi = ±(exit/entry−1)×lev×100`, `pnl = capital×roi/100` for `realized_roi_pct = 0 AND
exit_price > 0 AND exit_price != entry_price`, set `recomputed = true` (new bool column).
All future closes use `CalculateFuturesPnL` (already fixed for TIME_EXIT in `ac29b2c`).

**Rationale**: Already validated against live data (23 rows backfilled 2026-09-25, totals moved
−$6,612 → −$2,613).

## R10. Replay-simulation validation harness (SC-004)

**Decision**: Go test harness `internal/trader/replay_test.go` that feeds historical 1h+5m klines
(trailing 30 days, fetched via existing candle downloader, cached) through the new evaluation
path, asserting: expectancy > 0, TP1 hit-rate ∈ [40%,80%], stop-rate < 20%, and parameter configs
load with defaults. Runs in CI `test` job.

**Alternatives**: full backtest framework (rejected — YAGNI for this scope); manual analysis only
(rejected — constitution VIII requires validation evidence).

**Rationale**: Constitution VIII gate: no risk-parameter change ships without simulation evidence.
