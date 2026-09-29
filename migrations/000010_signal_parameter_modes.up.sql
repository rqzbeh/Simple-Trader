-- Spec 015: Jev-Managed Trade Parameters with Optional Overrides
ALTER TABLE futures_trade_signals
    ADD COLUMN IF NOT EXISTS parameter_modes JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS parameter_values JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS parameter_distributions JSONB,
    ADD COLUMN IF NOT EXISTS parameter_clamps JSONB;
