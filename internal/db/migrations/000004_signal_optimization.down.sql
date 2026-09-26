-- 000004_signal_optimization.down.sql

BEGIN;

ALTER TABLE futures_trade_signals
    DROP CONSTRAINT IF EXISTS chk_futures_signals_profile,
    DROP CONSTRAINT IF EXISTS chk_futures_signals_decay_state;

ALTER TABLE futures_trade_signals
    DROP COLUMN IF EXISTS catalyst_event_id,
    DROP COLUMN IF EXISTS model_confidence,
    DROP COLUMN IF EXISTS profile,
    DROP COLUMN IF EXISTS atr_at_entry,
    DROP COLUMN IF EXISTS tp1_close_fraction,
    DROP COLUMN IF EXISTS recomputed,
    DROP COLUMN IF EXISTS decay_state,
    DROP COLUMN IF EXISTS rejected_reason;

DROP TABLE IF EXISTS entry_filter_log;
DROP TABLE IF EXISTS risk_profiles;
DROP TABLE IF EXISTS catalyst_events;

COMMIT;
