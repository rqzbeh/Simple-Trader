# Phase 1 Contracts: Signal Optimization API Additions

Base: existing REST under `/api/v1` (JSON, same auth as current handlers). All changes are
**additive** — existing consumers keep working (extra fields only; no removals/renames on the wire).

## 1. `GET /api/v1/signals/futures` (extended)

Query: `status`, `profile` (`CRYPTO`|`COMMODITY` — NEW, default all), `limit` (unchanged).

Each item gains fields:

```jsonc
{
  "id": 123,
  // ...all existing fields unchanged...
  "profile": "CRYPTO",                    // NEW
  "model_confidence": 0.74,               // NEW (old "confidence" semantics preserved)
  "catalyst_sentiment": -0.11,            // now fused news score; legacy rows may hold confidence
  "catalyst_event": {                     // NEW, null when unclustered
    "id": 88, "story_count": 5, "headline": "...",
    "sources": [{"name":"Reuters","tier":1.0}],
    "fused_sentiment": -0.62, "polarization": 0.18, "fresh_weight": 0.71
  },
  "take_profit_1": 84910.0,               // now always populated for new signals
  "take_profit_2": 85620.0,               // runner; may be null
  "tp1_close_fraction": 0.6,
  "atr_at_entry": 412.5,                  // NEW audit field
  "decay_state": "BREAKEVEN",             // NEW: NONE | BREAKEVEN
  "recomputed": false                     // NEW: true for migrated historical rows
}
```

Backward-compat rule: unknown fields MUST be ignored by old clients (standard JSON).

## 2. `GET /api/v1/signals/summary` (NEW)

Query: `profile` (optional), `since` (optional ISO date).

```jsonc
{
  "profile": "COMMODITY",
  "closed": 42, "wins": 21, "losses": 20, "flat": 1,
  "win_rate": 0.50,
  "avg_win_pct": 3.12, "avg_loss_pct": -4.05,
  "payoff_ratio": 0.77,
  "expectancy_pct": -0.17,
  "total_pnl_usd": -840.2,
  "stop_out_rate": 0.19, "tp1_hit_rate": 0.55
}
```

Errors: `401` auth (existing), `400` invalid profile.

## 3. `GET /api/v1/signals/filters` (NEW)

Query: `limit` (≤200, default 50), `rule` (optional), `symbol` (optional).

```jsonc
[
  {
    "id": 9, "created_at": "2026-09-26T10:02:11Z",
    "symbol": "XRP/USDT", "direction": "LONG",
    "catalyst_event_id": 88,
    "rule": "CHASE_BLOCKED",
    "detail": {"z_vs_vwap": 2.4, "threshold": 2.0}
  }
]
```

`rule` enum (closed set): `CHASE_BLOCKED`, `NO_VOLUME`, `NO_TREND`, `OI_TRAP`, `POLARIZED`,
`STALE_NEWS`, `EVENT_BLACKOUT`, `LIQ_BUFFER`, `NO_NEWS`.

## 4. `GET /api/v1/risk-profiles` (NEW)

```jsonc
[ { "profile": "CRYPTO", "horizon_min": 60, "sl_atr_mult": 2.0,
    "tp1_atr_mult": 1.0, "tp2_atr_mult": 1.8, "tp1_close_fraction": 0.6,
    "decay_breakeven_at_min": 30, "decay_flat_at_min": 40,
    "target_hourly_vol_pct": 3.5, "risk_per_trade_pct": 1.5,
    "freshness_half_life_min": 15, "liq_buffer_min": 4.0,
    "weekend_flat": false, "blackout_calendar": [] } ]
```

Read-only view of effective config (tuning happens via config file per FR-019).

## 5. SSE `/api/v1/stream` (extended event)

Existing `signal` event unchanged. NEW optional event emitted for audit UI:

```
event: filter_rejected
data: {"symbol":"SOL/USDT","direction":"LONG","rule":"NO_VOLUME","detail":{...},"ts":"..."}
```

Clients that don't handle it ignore it (SSE unknown-event rule).

## 6. Telegram

No contract change (silent sends already shipped; message bodies unchanged).
