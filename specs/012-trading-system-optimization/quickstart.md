# Quickstart Validation: Trading System Optimization

Run from repo root. Prerequisites: Go 1.24+, Bun 1.3+, Docker (Compose stack from
`docker-compose.prebuilt.yml` for local Postgres/Redis).

## 1. Unit + simulation gates (constitution VIII evidence)

```bash
go build ./...
go test ./internal/trader/ ./internal/market/ ./internal/indicators/ ./internal/server/
```

Expected:
- New parameter math tests pass (ATR stops, TP bands, leverage buffer, decay, sizing).
- `replay_test.go` prints simulation report and asserts:
  expectancy > 0, TP1 hit-rate ∈ [40%,80%], stop-rate < 20% over trailing 30 days of klines
  (first run downloads klines via the existing downloader and caches them).

## 2. Database migrations + idempotent PnL backfill

```bash
# with stack up (docker compose -f docker-compose.prebuilt.yml up -d postgres redis)
go test ./internal/db/ -run Migration
docker exec -i simple-trader-postgres psql -U trader -d simple_trader \
  < internal/db/migrations/0000XX_true_pnl_backfill.up.sql   # path finalized in tasks
```

Expected: rows previously `realized_roi_pct = 0` with `exit_price != entry_price` now carry
computed values; rerun is a no-op (`UPDATE 0` second time); `recomputed = true`.

Validation query:

```sql
select count(*) from futures_trade_signals
where status='CLOSED' and exit_price is not null and exit_price <> entry_price
  and coalesce(realized_roi_pct,0) = 0;   -- expected: 0
```

## 3. Entry filter behavior

```bash
bun test                       # web unit tests
go test ./internal/server/ -run FilterLog
```

Expected scenarios:
- chase-blocked, no-volume, polarized, stale-news fixtures each produce one `entry_filter_log`
  row with the matching `rule` and no signal created.
- 5 identical headlines ⇒ 1 catalyst event, `story_count = 5`, at most 1 signal for the symbol.

## 4. API contract smoke test (stack running)

```bash
curl -s localhost:18080/api/v1/signals/futures?limit=1 | jq '.[0] | {profile, recomputed, catalyst_event}'
curl -s localhost:18080/api/v1/signals/summary?profile=CRYPTO | jq '{win_rate, expectancy_pct, tp1_hit_rate}'
curl -s localhost:18080/api/v1/signals/filters?limit=5 | jq '.[].rule'
curl -s localhost:18080/api/v1/risk-profiles | jq '.[].profile'
```

Expected: fields present per `contracts/api.md`; summary numbers reconcile with a manual SQL
count of the same window; old clients unaffected (fields only added).

## 5. Commodities section (manual E2E)

1. Open Commodities view → empty state renders, profile params visible via `risk-profiles`.
2. Inject a gold catalyst (dry-run flag or test feed) during non-blackout time → signal appears
   in Commodities list with horizon 4h (not 1h), `profile = COMMODITY`.
3. Set clock/config into NFP blackout window → same catalyst ⇒ no signal, `EVENT_BLACKOUT`
   row in filters log.
4. Confirm a crypto signal is unaffected by the commodity blackout.

## 6. Honest reporting check

```bash
curl -s localhost:18080/api/v1/signals/futures?status=CLOSED&limit=100 \
  | jq '[.[] | select(.exit_price != null and .exit_price != .entry_price and .realized_roi_pct == 0)] | length'
```

Expected: `0`. Historical rows show `recomputed: true` and non-zero values where price moved.

## 7. CI + deploy

```bash
git push origin 012-trading-system-optimization   # PR → main after review
# merge to main runs: go test + bun test + image build (ci-cd.yml)
# VPS: docker pull ghcr.io/rqzbeh/simple-trader-backend:latest && compose up -d backend
```

Expected: CI green; container healthy; site serves new bundle.
