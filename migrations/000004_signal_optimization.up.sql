-- 000004_signal_optimization.up.sql
-- Trading System Optimization (spec 012): catalyst events, risk profiles,
-- entry filter audit log, and futures_trade_signals additions.

BEGIN;

-- 1. Clustered news catalyst events (spec FR-008..FR-011)
CREATE TABLE IF NOT EXISTS catalyst_events (
    id BIGSERIAL PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    headline TEXT NOT NULL,
    sources JSONB NOT NULL DEFAULT '[]'::jsonb,
    story_count INTEGER NOT NULL DEFAULT 1,
    symbols TEXT[] NOT NULL DEFAULT '{}',
    fused_sentiment DOUBLE PRECISION NOT NULL DEFAULT 0,
    polarization DOUBLE PRECISION NOT NULL DEFAULT 0,
    fresh_weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    half_life_minutes INTEGER NOT NULL DEFAULT 15,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    window_end TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '45 minutes'
);
CREATE INDEX IF NOT EXISTS idx_catalyst_events_created ON catalyst_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_catalyst_events_fingerprint ON catalyst_events (fingerprint);

-- 2. Risk profiles: effective parameters per asset class (seeded at startup, FR-019)
CREATE TABLE IF NOT EXISTS risk_profiles (
    profile VARCHAR(16) PRIMARY KEY,
    horizon_min INTEGER NOT NULL,
    horizon_max INTEGER NOT NULL,
    decay_breakeven_at_min INTEGER NOT NULL,
    decay_flat_at_min INTEGER NOT NULL,
    sl_atr_mult DOUBLE PRECISION NOT NULL,
    sl_swing_offset DOUBLE PRECISION NOT NULL,
    sl_min_pct DOUBLE PRECISION NOT NULL,
    sl_max_pct DOUBLE PRECISION NOT NULL,
    tp1_atr_mult DOUBLE PRECISION NOT NULL,
    tp2_atr_mult DOUBLE PRECISION NOT NULL,
    tp1_close_fraction DOUBLE PRECISION NOT NULL,
    target_hourly_vol_pct DOUBLE PRECISION NOT NULL,
    liq_buffer_min DOUBLE PRECISION NOT NULL,
    risk_per_trade_pct DOUBLE PRECISION NOT NULL,
    freshness_half_life_min DOUBLE PRECISION NOT NULL,
    weekend_flat BOOLEAN NOT NULL DEFAULT FALSE,
    blackout_calendar JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Entry filter audit log (spec FR-020)
CREATE TABLE IF NOT EXISTS entry_filter_log (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    symbol VARCHAR(32) NOT NULL,
    direction VARCHAR(8) NOT NULL,
    catalyst_event_id BIGINT NULL REFERENCES catalyst_events(id),
    rule VARCHAR(32) NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_entry_filter_log_created ON entry_filter_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_entry_filter_log_rule ON entry_filter_log (rule);

-- 4. futures_trade_signals additions (data-model.md §2)
ALTER TABLE futures_trade_signals
    ADD COLUMN IF NOT EXISTS catalyst_event_id BIGINT NULL REFERENCES catalyst_events(id),
    ADD COLUMN IF NOT EXISTS model_confidence DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS profile VARCHAR(16) NOT NULL DEFAULT 'CRYPTO',
    ADD COLUMN IF NOT EXISTS atr_at_entry DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS tp1_close_fraction DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS recomputed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS decay_state VARCHAR(16) NOT NULL DEFAULT 'NONE',
    ADD COLUMN IF NOT EXISTS rejected_reason VARCHAR(32) NULL;

ALTER TABLE futures_trade_signals
    ADD CONSTRAINT chk_futures_signals_profile
        CHECK (profile IN ('CRYPTO', 'COMMODITY')),
    ADD CONSTRAINT chk_futures_signals_decay_state
        CHECK (decay_state IN ('NONE', 'BREAKEVEN', 'CLOSED'));

CREATE INDEX IF NOT EXISTS idx_futures_signals_profile ON futures_trade_signals (profile);

COMMIT;
