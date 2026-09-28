# Research: spec-014 early exit (Phase 0)

- **Decision**: one batched core call per evaluation cycle covering all open positions (state per position inside one request; Jev supports multi-question batches — spec-013 research rule: independent questions in ONE request).
  - Rationale: ≤5 positions; single call ≤0.6s total; cheaper than per-position calls.
  - Alternative: per-position call — rejected (latency × N, cost × N).
- **Decision**: judgment question = `hold_now` Noul with criteria describing position direction, entry, ATR levels, fresh cluster; verdict mapping: noul ≥ confidence floor AND "hold" sense → hold; else `DO_NOT_HOLD` after guards. Wording inverts so the model is asked "should this position be CLOSED now?" (noul=true means close) — avoids negation confusion; document in criteria.
  - Alternative: Choice HOLD/DO_NOT_HOLD — equivalent; Noul chosen for calibrated single probability.
- **Decision**: dedup key = (position_id, cluster_id); store in judgment table; clusterer already dedups headlines.
- **Decision**: notification = fire-and-forget after successful close; exactly one explicit attempt; failure → error record linked to judgment (no retry loop → no duplicate-close risk, SC-104).
  - Alternative: retry with idempotency key — overkill for single-instance engine holding e.mu lock during close.
- **Decision**: guard defaults seed as configurable-only (no hardcoded): `EARLY_EXIT_MIN_HOLD_MIN=30`, `EARLY_EXIT_MAX_PER_DAY=3`, `EARLY_EXIT_COOLDOWN_MIN=60`, `EARLY_EXIT_CONF_FLOOR=0.75`, `EARLY_EXIT_ENABLED=true` — values go to `.env` (settings persistence allows UI tuning; Constitution VIII: tune from SC-105 evidence, defaults are starting points not proven params).
- **Decision**: kill switch checked first (FR-109); recording continues, closes disabled.
- **Unknown resolved**: close path — reuse `ExecutionEngine.closePositionLocked` via existing `ForceClosePosition(symbol, price, "NEWS_EARLY_EXIT")`; engine already accepts arbitrary exit reason (verified `closePositionLocked` takes `exitReason`).
- **Telegram format**: reuse existing resolution-message formatter shape (symbol, side, PnL) + cluster headline + confidence + route fields.
