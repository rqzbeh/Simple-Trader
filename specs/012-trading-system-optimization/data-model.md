# Phase 1 Data Model: Trading System Optimization

## Entity Relationship Overview

```text
catalyst_events 1───n signals 1───1 trade_outcomes (embedded in signals row)
risk_profiles 1───n signals (profile = 'CRYPTO' | 'COMMODITY')
signals 1───n entry_filter_log (rejections reference a would-be evaluation, signal_id nullable)
performance_summary = derived view over signals + outcomes (no storage)
```

## 1. `catalyst_events` (new table)

| Field | Type | Rules / Notes |
|---|---|---|
| id | bigserial PK | |
| fingerprint | text | normalized-title shingle hash, unique per cluster |
| headline | text | representative (first) headline |
| sources | jsonb | `[{name, tier, url, published_at}]` — tier ∈ {1.0, 0.6, 0.2} |
| story_count | int | ≥1; increments on merge (FR-008) |
| symbols | text[] | affected assets from headline tagging |
| fused_sentiment | float | tier+freshness weighted ∈ [−1, 1] (FR-010) |
| polarization | float | ∈ [0,1]; > 0.40 ⇒ veto (FR-011) |
| fresh_weight | float | exp-decay applied at decision time (FR-009) |
| half_life_minutes | int | 15 (crypto) / 60 (commodity) |
| created_at / window_end | timestamptz | clustering window = 45 min |

Validation: merge only when Jaccard(title 3-grams) ≥ 0.82 and Δt ≤ 45 min. Events older than
2× half-life are non-tradeable.

## 2. `futures_trade_signals` (altered)

New/changed columns:

| Column | Type | Rules |
|---|---|---|
| catalyst_event_id | bigint NULL FK → catalyst_events | set when created from clustered event |
| model_confidence | float NULL | AI signed confidence (FR-012) — renamed semantics of old catalyst_sentiment |
| catalyst_sentiment | float | now = fused news score (FR-012); legacy rows untouched (R6) |
| profile | varchar(16) NOT NULL default 'CRYPTO' | 'CRYPTO' \| 'COMMODITY' |
| take_profit_1 / take_profit_2 | float | tp1 always set; tp2 set when runner exists (currently NULL → now used) |
| tp1_close_fraction | float | default 0.6 (FR-004) |
| atr_at_entry | float | 1h ATR snapshot — audit trail for param derivation (FR-002/003) |
| rejected_reason | varchar(32) NULL | for would-be signals logged without opening (FR-020) |
| recomputed | boolean default false | set by R9 migration (FR-017) |
| decay_state | varchar(16) | `NONE` → `BREAKEVEN` → `CLOSED` (FR-005) |

State transitions (status): `ACTIVE → CLOSED` via {STOP_LOSS, TP1_TP2, TIME_EXIT, MANUAL_EXIT,
DECAY_EXIT, EVENT_BLACKOUT, DEDUP}; rejection path never creates ACTIVE: `evaluated → REJECTED`
(writes only entry_filter_log).

## 3. `risk_profiles` (new table, seeded)

| Column | CRYPTO default | COMMODITY default |
|---|---|---|
| horizon_min / horizon_max | 60 / 60 | 240 / 720 |
| sl_atr_mult / sl_swing_offset | 2.0 / 0.25 | 2.5 / 0.25 |
| tp1_atr_mult / tp2_atr_mult / tp1_close_frac | 1.0 / 1.8 / 0.6 | 1.2 / 2.2 / 0.6 |
| decay_breakeven_at_min / decay_flat_at_min | 30 / 40 | 120 / 180 |
| target_hourly_vol_pct | 3.5 | 1.2 |
| risk_per_trade_pct | 1.5 | 1.0 |
| freshness_half_life_min | 15 | 60 |
| liq_buffer_min | 4.0 | 4.0 |
| blackout_calendar | (empty) | NFP/CPI/FOMC ±30m, EIA ±15m, weekend-gap guard Fri 16:45 ET |
| weekend_flat | false | true |

Editable via config file (FR-019); table stores effective values for audit.

## 4. `entry_filter_log` (new table)

| Column | Type | Notes |
|---|---|---|
| id | bigserial PK | |
| created_at | timestamptz | |
| symbol / direction | text | candidate that was rejected |
| catalyst_event_id | bigint NULL | |
| rule | varchar(32) | `CHASE_BLOCKED` \| `NO_VOLUME` \| `NO_TREND` \| `OI_TRAP` \| `POLARIZED` \| `STALE_NEWS` \| `EVENT_BLACKOUT` \| `LIQ_BUFFER` \| `NO_NEWS` |
| detail | jsonb | rule-specific metrics (z-score, volume ratio, polarization…) |

## 5. TradeOutcome / PerformanceSummary (derived)

- Outcome already stored per signal (`exit_price`, `exit_time`, `realized_roi_pct`,
  `realized_pnl_usd`, `recomputed`).
- PerformanceSummary = query-time aggregation grouped by `profile` and `symbol`:
  `win_rate = count(roi>0)/count(closed)`, `avg_win`, `avg_loss`, `payoff = |avg_win/avg_loss|`,
  `expectancy = win_rate×avg_win + (1−win_rate)×avg_loss` (FR-018).

## Migration notes

1. `CREATE TABLE catalyst_events, risk_profiles, entry_filter_log`
2. `ALTER futures_trade_signals ADD COLUMN profile, model_confidence, atr_at_entry,
   tp1_close_fraction, recomputed, decay_state, catalyst_event_id, rejected_reason`
3. Backfill per R9 (idempotent WHERE guards) — runs once, sets `recomputed=true`
4. Existing open signals adopt profile='CRYPTO' and current effective defaults
