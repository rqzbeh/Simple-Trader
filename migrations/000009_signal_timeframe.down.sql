ALTER TABLE futures_trade_signals
    DROP COLUMN IF EXISTS timeframe_confidence,
    DROP COLUMN IF EXISTS timeframe_distribution,
    DROP COLUMN IF EXISTS timeframe;
