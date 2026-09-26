-- 000005_true_pnl_backfill.up.sql
-- Spec 012 / US5 (SC-001): TIME_EXIT rows were persisted with hardcoded 0/0
-- before fix ac29b2c. Recompute true PnL/ROI from recorded prices, exactly
-- like CalculateFuturesPnL with margin = allocated capital:
--   roi% = +/- (exit/entry - 1) * leverage * 100
--   pnl  = allocated_capital_usd * roi% / 100
-- WHERE guards make the statement idempotent (second run updates 0 rows).

BEGIN;

-- 1. Rows still carrying placeholder zeros (pre-deploy data)
UPDATE futures_trade_signals
SET realized_roi_pct =
        (CASE WHEN direction = 'LONG'
              THEN (exit_price / entry_price - 1)
              ELSE (1 - exit_price / entry_price)
         END) * leverage * 100,
    realized_pnl_usd =
        allocated_capital_usd *
        ((CASE WHEN direction = 'LONG'
                THEN (exit_price / entry_price - 1)
                ELSE (1 - exit_price / entry_price)
           END) * leverage),
    recomputed = TRUE
WHERE status = 'CLOSED'
  AND exit_reason = 'TIME_EXIT'
  AND realized_roi_pct = 0
  AND exit_price IS NOT NULL
  AND exit_price > 0
  AND exit_price <> entry_price;

-- 2. Rows corrected manually on 2026-09-25 before this migration existed:
--    mark them recomputed so the UI can distinguish migrated history.
--    (Migration runs exactly once, so future computed TIME_EXIT rows are safe.)
UPDATE futures_trade_signals
SET recomputed = TRUE
WHERE status = 'CLOSED'
  AND exit_reason = 'TIME_EXIT'
  AND realized_roi_pct IS NOT NULL
  AND realized_roi_pct <> 0
  AND recomputed = FALSE
  AND exit_time < '2026-09-26 00:00:00+00';

COMMIT;
