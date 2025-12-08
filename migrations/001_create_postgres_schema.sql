-- migrations/001_create_postgres_schema.sql
-- SQL script to bootstrap a Postgres schema equivalent to the SQLite schema used by Simple-Trader.
-- Note: This script is designed for Postgres >= 9.5+ and creates JSONB columns where appropriate.
-- Run on a Postgres server and point your application to use the resulting DB.
--
-- Important: For a production migration, test the migration on a staging copy of your DB first,
-- verify types, and prepare any further schema adjustments (indexes, constraints, partitioning).
--
BEGIN;

-- News table
CREATE TABLE IF NOT EXISTS news (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT NOT NULL,
    url TEXT,
    title TEXT,
    content TEXT,
    published_at TIMESTAMPTZ,
    asset TEXT,
    hash TEXT UNIQUE,
    fetched_at TIMESTAMPTZ,
    processed BOOLEAN DEFAULT FALSE,
    raw_json JSONB,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_news_published ON news (published_at);
CREATE INDEX IF NOT EXISTS idx_news_asset ON news (asset);
CREATE INDEX IF NOT EXISTS idx_news_processed ON news (processed);

-- Analysis table (LLM analyses)
CREATE TABLE IF NOT EXISTS analysis (
    id BIGSERIAL PRIMARY KEY,
    news_id BIGINT NOT NULL REFERENCES news(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    analysis_json JSONB,
    confidence REAL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_analysis_news_id ON analysis(news_id);

-- Market data (cached OHLC)
CREATE TABLE IF NOT EXISTS market_data (
    id BIGSERIAL PRIMARY KEY,
    symbol TEXT NOT NULL,
    timeframe TEXT NOT NULL,
    start_ts BIGINT NOT NULL,
    open NUMERIC,
    high NUMERIC,
    low NUMERIC,
    close NUMERIC,
    volume NUMERIC,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(symbol, timeframe, start_ts)
);

CREATE INDEX IF NOT EXISTS idx_market_data_symbol_time ON market_data(symbol, timeframe, start_ts);

-- Signals table
CREATE TABLE IF NOT EXISTS signals (
    id BIGSERIAL PRIMARY KEY,
    news_id BIGINT REFERENCES news(id) ON DELETE SET NULL,
    symbol TEXT NOT NULL,
    side TEXT NOT NULL, -- 'long' or 'short'
    entry_price NUMERIC NOT NULL,
    stop_loss NUMERIC NOT NULL,
    take_profit NUMERIC NOT NULL,
    leverage INTEGER,
    position_size NUMERIC,
    risk_amount NUMERIC,
    rr REAL,
    timeframe_hours INTEGER,
    status TEXT DEFAULT 'open',
    analysis_ids JSONB,
    created_at TIMESTAMPTZ DEFAULT now(),
    expires_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    close_reason TEXT,
    outcome TEXT,
    pnl NUMERIC
);

CREATE INDEX IF NOT EXISTS idx_signals_status ON signals(status);
CREATE INDEX IF NOT EXISTS idx_signals_symbol ON signals(symbol);

-- Trades table (actual executions / recorded results)
CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    signal_id BIGINT REFERENCES signals(id) ON DELETE CASCADE,
    executed_at TIMESTAMPTZ NOT NULL,
    executed_price NUMERIC NOT NULL,
    exit_at TIMESTAMPTZ,
    exit_price NUMERIC,
    pnl NUMERIC,
    outcome TEXT,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_trades_signal_id ON trades(signal_id);

-- Tuning stats table
CREATE TABLE IF NOT EXISTS tuning_stats (
    id BIGSERIAL PRIMARY KEY,
    pattern_name TEXT NOT NULL,
    symbol TEXT,
    wins INTEGER DEFAULT 0,
    losses INTEGER DEFAULT 0,
    avg_rr REAL DEFAULT 0.0,
    avg_hold_time_seconds REAL DEFAULT 0.0,
    last_updated TIMESTAMPTZ DEFAULT now(),
    UNIQUE(pattern_name, symbol)
);

CREATE INDEX IF NOT EXISTS idx_tuning_stats_pattern ON tuning_stats(pattern_name);
CREATE INDEX IF NOT EXISTS idx_tuning_stats_symbol ON tuning_stats(symbol);

-- LLM usage telemetry
CREATE TABLE IF NOT EXISTS llm_usage (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT,
    request_ts BIGINT,
    request_size INTEGER,
    response_tokens INTEGER,
    estimated_cost REAL DEFAULT 0.0,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Runtime parameters (runtime knobs / tuning)
CREATE TABLE IF NOT EXISTS runtime_params (
    key TEXT PRIMARY KEY,
    value TEXT,
    description TEXT,
    last_updated TIMESTAMPTZ DEFAULT now()
);

-- Tuning history (audit trail for auto-applied changes)
CREATE TABLE IF NOT EXISTS tuning_history (
    id BIGSERIAL PRIMARY KEY,
    pattern_name TEXT,
    symbol TEXT,
    change TEXT,
    old_value TEXT,
    new_value TEXT,
    notes TEXT,
    applied_by TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tuning_history_pattern ON tuning_history(pattern_name);
CREATE INDEX IF NOT EXISTS idx_tuning_history_symbol ON tuning_history(symbol);

COMMIT;
