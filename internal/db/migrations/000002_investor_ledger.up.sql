-- 000002_investor_ledger.up.sql
-- Investor Capital Ledger, Portfolio NAV, News Articles, and Crypto Screener Tables

BEGIN;

-- 1. Investors Profile Table (Non-login administrative tracking)
CREATE TABLE IF NOT EXISTS investors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    contact_tag VARCHAR(255) NOT NULL,
    notes TEXT DEFAULT '',
    total_deposited NUMERIC(18, 4) NOT NULL DEFAULT 0.0000,
    total_withdrawn NUMERIC(18, 4) NOT NULL DEFAULT 0.0000,
    pool_units NUMERIC(24, 8) NOT NULL DEFAULT 0.00000000,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, FROZEN, CLOSED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_investors_status ON investors(status);
CREATE INDEX IF NOT EXISTS idx_investors_contact ON investors(contact_tag);

-- 2. Investor Capital Transactions (Immutable Financial Ledger)
CREATE TABLE IF NOT EXISTS investor_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investor_id UUID NOT NULL REFERENCES investors(id) ON DELETE CASCADE,
    tx_type VARCHAR(32) NOT NULL, -- DEPOSIT, WITHDRAWAL, PROFIT_PAYOUT, FEE
    amount NUMERIC(18, 4) NOT NULL,
    pool_units NUMERIC(24, 8) NOT NULL,
    nav_at_execution NUMERIC(18, 6) NOT NULL,
    notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_investor_tx_investor_id ON investor_transactions(investor_id);
CREATE INDEX IF NOT EXISTS idx_investor_tx_type ON investor_transactions(tx_type);
CREATE INDEX IF NOT EXISTS idx_investor_tx_created ON investor_transactions(created_at DESC);

-- 3. Portfolio NAV Snapshot History
CREATE TABLE IF NOT EXISTS portfolio_nav_history (
    id BIGSERIAL PRIMARY KEY,
    total_equity NUMERIC(18, 4) NOT NULL,
    tier1_cash_reserve NUMERIC(18, 4) NOT NULL,
    tier2_core_equity NUMERIC(18, 4) NOT NULL,
    tier3_alpha_equity NUMERIC(18, 4) NOT NULL,
    total_units NUMERIC(24, 8) NOT NULL,
    nav_per_unit NUMERIC(18, 6) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_portfolio_nav_recorded_at ON portfolio_nav_history(recorded_at DESC);

-- 4. News Articles Ingested (Live RSS / API feeds)
CREATE TABLE IF NOT EXISTS news_articles (
    content_hash CHAR(64) PRIMARY KEY,
    title TEXT NOT NULL,
    source VARCHAR(64) NOT NULL,
    url TEXT NOT NULL,
    sentiment_score NUMERIC(6, 4) NOT NULL,
    polarity VARCHAR(16) NOT NULL, -- BULLISH, BEARISH, NEUTRAL
    key_phrases TEXT[] NOT NULL DEFAULT '{}',
    published_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_news_articles_published ON news_articles(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_news_articles_polarity ON news_articles(polarity);

-- 5. Crypto Screener Snapshots
CREATE TABLE IF NOT EXISTS crypto_screener_snapshots (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(50) NOT NULL,
    price NUMERIC(18, 6) NOT NULL,
    volume_24h NUMERIC(18, 2) NOT NULL,
    bid_ask_spread_bps NUMERIC(10, 2) NOT NULL,
    status VARCHAR(20) NOT NULL, -- ACTIVE, FROZEN, DISQUALIFIED
    rejection_reason VARCHAR(255) DEFAULT '',
    screened_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crypto_screener_symbol ON crypto_screener_snapshots(symbol);
CREATE INDEX IF NOT EXISTS idx_crypto_screener_screened ON crypto_screener_snapshots(screened_at DESC);

COMMIT;
