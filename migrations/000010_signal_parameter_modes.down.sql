ALTER TABLE futures_trade_signals
    DROP COLUMN IF EXISTS parameter_clamps,
    DROP COLUMN IF EXISTS parameter_distributions,
    DROP COLUMN IF EXISTS parameter_values,
    DROP COLUMN IF EXISTS parameter_modes;
