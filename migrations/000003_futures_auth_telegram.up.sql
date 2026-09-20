-- 000003_futures_auth_telegram.up.sql
-- Two-Sided Futures Trading, Dynamic Macro Allocation, Auth Security, Telegram Signals, and ML Training Runs

BEGIN;

-- 1. Administrative Users Table
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'ADMIN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username);

-- 2. Encrypted System Secrets (AES-GCM-256 for Telegram tokens, API keys, etc.)
CREATE TABLE IF NOT EXISTS encrypted_system_secrets (
    key_name VARCHAR(64) PRIMARY KEY,
    encrypted_payload TEXT NOT NULL,
    nonce VARCHAR(32) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Two-Sided Futures Trade Signals
CREATE TABLE IF NOT EXISTS futures_trade_signals (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    direction VARCHAR(8) NOT NULL CHECK (direction IN ('LONG', 'SHORT')),
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, CLOSED, CANCELLED, PENDING
    catalyst_headline TEXT NOT NULL,
    catalyst_source VARCHAR(64) NOT NULL,
    catalyst_sentiment DOUBLE PRECISION NOT NULL,
    entry_price DOUBLE PRECISION NOT NULL,
    stop_loss DOUBLE PRECISION NOT NULL,
    take_profit_1 DOUBLE PRECISION NOT NULL,
    take_profit_2 DOUBLE PRECISION NULL,
    leverage INTEGER NOT NULL DEFAULT 1,
    risk_reward_ratio DOUBLE PRECISION NOT NULL,
    allocated_capital_usd DOUBLE PRECISION NOT NULL,
    allocated_capital_pct DOUBLE PRECISION NOT NULL,
    exit_price DOUBLE PRECISION NULL,
    exit_time TIMESTAMPTZ NULL,
    exit_reason VARCHAR(16) NULL, -- TP1, TP2, SL, MANUAL
    realized_pnl_usd DOUBLE PRECISION NULL,
    realized_roi_pct DOUBLE PRECISION NULL,
    telegram_dispatched BOOLEAN NOT NULL DEFAULT FALSE,
    telegram_resolved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_futures_signals_symbol ON futures_trade_signals(symbol);
CREATE INDEX IF NOT EXISTS idx_futures_signals_status ON futures_trade_signals(status);
CREATE INDEX IF NOT EXISTS idx_futures_signals_created ON futures_trade_signals(created_at DESC);

-- 4. Dynamic Macroeconomic Regimes
CREATE TABLE IF NOT EXISTS macro_regimes (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stress_score DOUBLE PRECISION NOT NULL,
    geopolitical_risk_index DOUBLE PRECISION NOT NULL,
    inflation_rate_pct DOUBLE PRECISION NOT NULL,
    interest_rate_bias VARCHAR(16) NOT NULL, -- DOVISH, NEUTRAL, HAWKISH
    target_tier1_cash_pct DOUBLE PRECISION NOT NULL,
    target_tier2_alpha_pct DOUBLE PRECISION NOT NULL,
    target_tier3_core_pct DOUBLE PRECISION NOT NULL,
    rationale TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_macro_regimes_timestamp ON macro_regimes(timestamp DESC);

-- 5. Machine Learning Training Runs (Real Binance Data Calibration)
CREATE TABLE IF NOT EXISTS ml_training_runs (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    timeframe VARCHAR(16) NOT NULL,
    sample_count INTEGER NOT NULL,
    date_start TIMESTAMPTZ NOT NULL,
    date_end TIMESTAMPTZ NOT NULL,
    training_loss DOUBLE PRECISION NOT NULL,
    directional_accuracy DOUBLE PRECISION NOT NULL,
    weights_snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ml_training_runs_symbol ON ml_training_runs(symbol);
CREATE INDEX IF NOT EXISTS idx_ml_training_runs_created ON ml_training_runs(created_at DESC);

COMMIT;
