-- Spec 012 / US7 (FR-022): record the decision-time indicator measurements
-- that produced each signal, so closed-trade outcomes can be attributed to
-- the indicators that were actually bold in the decision. Nullable: legacy
-- rows predate the snapshot and must not be back-filled (Amendment A1).
ALTER TABLE futures_trade_signals
    ADD COLUMN IF NOT EXISTS indicator_snapshot JSONB;
