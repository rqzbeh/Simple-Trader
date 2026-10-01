# spec-022: engine health truthfulness + signal persistence integrity

**Input**: user 2026-10-01 — "system stats still reported as down although telegram bot sends signals"; engines diagnosed false-down (gateway 17/17, Jev 12/12 upstream OK).

## Root causes (verified)
1. **Cold boot = DOWN**: `SystemStatsView.getHealthStatus` treats `total==0`/`success==0` as DOWN (`web/src/components/SystemStatsView.tsx:44-57`). No boot warmup probe exists, so counters start 0/0 after every restart.
2. **Idle = DOWN**: event-driven scanner (spec-020) can idle >5 min between catalysts → `last_ok_at` staleness check labels engines "DOWN (>5M STALE)". Truthful states: `STANDBY` (no data yet), `IDLE` (last ok >5m, zero fails), `DEGRADED` (recent fails), `HEALTHY`.
3. **Signal rows lost while Telegram fired**: during the Postgres-auth outage the nil store silently skipped INSERTs yet Telegram still dispatched — Telegram claims signals that were never persisted (today's rows unrecoverable). Data restored separately (volume orphaned by compose project rename `repo_postgres_data` → `simple-trader_postgres_data`; content copied, max id 154).

## Requirements
- **FR-801** (frontend): `getHealthStatus` → 4 states: `STANDBY` when `total==0 && fail==0` (boot, no traffic yet); `IDLE` when `success>0 && fail==0 && last_ok>5m`; `DEGRADED` when `fail>0` within window; `HEALTHY` otherwise. Never DOWN unless fail>0 with no success in window.
- **FR-802** (backend): startup warmup probe — on boot, fire one lightweight 9Router + one Jev ping (async, non-blocking, timeout bounded) to populate `last_ok_at`/latency immediately.
- **FR-803** (persistence integrity): signal INSERT happens in `EvaluateMarketSignal` BEFORE Telegram dispatch; if store is nil/insert fails → explicit error, NO position creation, NO Telegram message. Zero-fallback: never announce what wasn't persisted.
- **FR-804**: gates G42-G45 + Docker+Playwright verify BOTH engine cards show non-DOWN states after fresh boot; unlazy reverify.

## Non-goals
No scan-interval default change (per user: optional keys stay unset). No dependency upgrades (separate Dependabot track).
