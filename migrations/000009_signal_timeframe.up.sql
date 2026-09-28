-- Spec 016: Dynamic trade timeframe selection by decision core
-- NULL values indicate legacy rows predating spec-016.
ALTER TABLE futures_trade_signals
    ADD COLUMN IF NOT EXISTS timeframe TEXT,
    ADD COLUMN IF NOT EXISTS timeframe_distribution JSONB,
    ADD COLUMN IF NOT EXISTS timeframe_confidence DOUBLE PRECISION;
