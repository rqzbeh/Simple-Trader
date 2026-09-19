-- 000001_init_schema.up.sql
-- Core PostgreSQL schema for Simple-Trader (Go + React PWA rewrite)

BEGIN;

-- 1. Market Data Candles (Fast OHLCV storage)
CREATE TABLE IF NOT EXISTS candles (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    timeframe VARCHAR(10) NOT NULL,
    open_time TIMESTAMPTZ NOT NULL,
    open NUMERIC(20, 8) NOT NULL,
    high NUMERIC(20, 8) NOT NULL,
    low NUMERIC(20, 8) NOT NULL,
    close NUMERIC(20, 8) NOT NULL,
    volume NUMERIC(24, 8) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_candle UNIQUE(symbol, timeframe, open_time)
);
CREATE INDEX IF NOT EXISTS idx_candles_symbol_time ON candles(symbol, timeframe, open_time DESC);

-- 2. News Items (Global RSS / API feeds, SHA-256 deduplicated)
CREATE TABLE IF NOT EXISTS news_items (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(64) NOT NULL,
    headline TEXT NOT NULL,
    summary TEXT,
    url TEXT,
    published_at TIMESTAMPTZ NOT NULL,
    asset_tag VARCHAR(32),
    sentiment_score REAL DEFAULT 0.0,
    hash VARCHAR(64) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_news_published ON news_items(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_news_asset_tag ON news_items(asset_tag);

-- 3. Dynamic Indicator Weights (Learned & Adjusted per Asset & Market Regime)
CREATE TABLE IF NOT EXISTS indicator_weights (
    id SERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    regime VARCHAR(32) NOT NULL DEFAULT 'NORMAL', -- 'BULL', 'BEAR', 'VOLATILE', 'RANGING'
    indicator_name VARCHAR(64) NOT NULL,          -- 'RSI', 'MACD', 'SUPER_TREND', 'BBANDS', 'VWAP', etc.
    weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    win_count INT NOT NULL DEFAULT 0,
    loss_count INT NOT NULL DEFAULT 0,
    cumulative_pnl NUMERIC(16, 4) NOT NULL DEFAULT 0.0,
    last_updated TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_weight UNIQUE(symbol, regime, indicator_name)
);
CREATE INDEX IF NOT EXISTS idx_weights_symbol_regime ON indicator_weights(symbol, regime);

-- 4. Trading Signals (AI + Confluence Generated)
CREATE TABLE IF NOT EXISTS signals (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    side VARCHAR(8) NOT NULL,                  -- 'BUY' or 'SELL'
    bucket VARCHAR(16) NOT NULL,               -- 'CORE' or 'ALPHA'
    entry_price NUMERIC(20, 8) NOT NULL,
    stop_loss NUMERIC(20, 8) NOT NULL,
    take_profit NUMERIC(20, 8) NOT NULL,
    confidence REAL NOT NULL,
    confluence_score REAL NOT NULL,
    ai_reasoning TEXT,
    indicator_snapshot JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',-- 'OPEN', 'EXECUTED', 'CANCELLED', 'EXPIRED'
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_signals_symbol_status ON signals(symbol, status);
CREATE INDEX IF NOT EXISTS idx_signals_created_at ON signals(created_at DESC);

-- 5. Trades & Positions (Executions, Live PnL, Close Audit)
CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    signal_id BIGINT REFERENCES signals(id) ON DELETE SET NULL,
    symbol VARCHAR(32) NOT NULL,
    side VARCHAR(8) NOT NULL,
    bucket VARCHAR(16) NOT NULL,
    position_size NUMERIC(20, 8) NOT NULL,
    entry_price NUMERIC(20, 8) NOT NULL,
    entry_time TIMESTAMPTZ NOT NULL,
    exit_price NUMERIC(20, 8),
    exit_time TIMESTAMPTZ,
    stop_loss NUMERIC(20, 8) NOT NULL,
    take_profit NUMERIC(20, 8) NOT NULL,
    realized_pnl NUMERIC(16, 4) DEFAULT 0.0,
    return_pct REAL DEFAULT 0.0,
    exit_reason VARCHAR(64),                   -- 'TP_HIT', 'SL_HIT', 'CIRCUIT_BREAKER', 'EXPIRED', 'MANUAL'
    root_cause VARCHAR(128),                   -- Attribution: 'FALSE_BREAKOUT', 'OVERBOUGHT_REVERSAL', etc.
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',-- 'OPEN', 'CLOSED'
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_trades_status ON trades(status);
CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol);
CREATE INDEX IF NOT EXISTS idx_trades_entry_time ON trades(entry_time DESC);

-- 6. Fine-Tuning Dataset (Exportable JSONL pairs)
CREATE TABLE IF NOT EXISTS fine_tune_records (
    id BIGSERIAL PRIMARY KEY,
    trade_id BIGINT REFERENCES trades(id) ON DELETE CASCADE,
    symbol VARCHAR(32) NOT NULL,
    prompt_system TEXT NOT NULL,
    prompt_user TEXT NOT NULL,
    assistant_response TEXT NOT NULL,
    trade_outcome VARCHAR(16) NOT NULL,        -- 'WIN' or 'LOSS'
    exported BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_finetune_exported ON fine_tune_records(exported);

COMMIT;
