# Contracts: spec-014

## 1. Core judgment (Jev, batched per cycle)

```
questions: {
  "close_now": { type: "noul",
    instructions: { question: "Should this open position be CLOSED immediately because fresh news invalidated it?",
                    position: {...direction, entry, atr_sl, atr_tp, age_min},
                    cluster: { headline, age_min, labels } },
    criteria: { true: "News invalidates the thesis — close now", false: "Thesis intact — hold" } }
}
```
Batch: one request, one `close_now` question per open position (ids `close_now:<position_id>`). Verdict: noul ≥ EARLY_EXIT_CONF_FLOOR... — wait: noul(true=close) ≥ threshold ⇒ DO_NOT_HOLD candidate → guards; else HOLD. Errors → typed DecisionError (component=early-exit, cycle) → action=error row.

## 2. Guards (order, first fail wins, recorded)

kill_switch → min_hold → cooldown → daily_budget → confidence_floor. All evaluated in Go (code owns workflow — Jev only judges). Result: guards_passed bool + guard_reason.

## 3. Close

`ForceClosePosition(symbol, price, "NEWS_EARLY_EXIT")` under engine lock. Failure → action=error + close_error, judgment retained, retried next cycle (dedup excludes prior close failures).

## 4. Telegram

```
🔴 EARLY EXIT — NEWS
Symbol: BTC/USDT (closed LONG)
Reason: SEC sues exchange... [cluster]
Confidence: 0.82 · route: jev_direct
PnL: +$142.10 (+1.4%) · exit: NEWS_EARLY_EXIT
```
One send attempt post-close; failure → telegram_error on row (close already done).

## 5. Settings keys added to PUT allowlist

EARLY_EXIT_ENABLED, EARLY_EXIT_MIN_HOLD_MIN, EARLY_EXIT_MAX_PER_DAY, EARLY_EXIT_COOLDOWN_MIN, EARLY_EXIT_CONF_FLOOR (same whitelist/validation path as spec-013 follow-on).

## 6. Migration

`migrations/000008_early_exit_judgments.up.sql|.down.sql` — ROOT only.
