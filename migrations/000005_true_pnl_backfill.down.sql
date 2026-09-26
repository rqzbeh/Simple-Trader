-- 000005_true_pnl_backfill.down.sql
-- Data backfill: intentionally forward-only. Reverting would re-zero real
-- outcomes (forbidden by constitution VIII), so only the flag is cleared.

BEGIN;
UPDATE futures_trade_signals SET recomputed = FALSE WHERE recomputed = TRUE;
COMMIT;
