# Quickstart validation: spec-013

## Prereqs
- `TYPESAFE_API_KEY` in env (exists in `/home/redsnow/.env`)
- `NINEROUTER_URL`/`NINEROUTER_KEY` set (existing)
- `docker compose up` stack healthy

## Scenarios

### V1 — Unit: typed errors, no silent substitution
`go test ./internal/ai/ -run TestJev` — mock 401/timeout/malformed-answer ⇒ assert `ErrJev*` with component+cycle id, zero default-answered records.
`go test ./internal/market/ -run TestSentiment` — **must fail compile/absence**: `AnalyzeNewsSentiment` and word lists deleted (FR-013).

### V2 — Live smoke: Jev contract
Scripted call (contract §1) with real key: expect HTTP 200, `answers.entry.choice ∈ {LONG,SHORT,NO_TRADE}`, probabilities sum≈1, latency <1s. (Manual; guarded test `TestJevLive` runs only with `-tags=liveapi`.)

### V3 — Shadow non-interference (SC-001)
Run signal cycle with `shadow:enabled=true` and `false`; diff emitted orders + live decision records ⇒ byte-identical. Shadow table gains rows only when enabled.

### V4 — News replacement (Option A)
Feed known cluster (e.g. "Fed cuts rates 50bps") through live news path ⇒ LLM classifier label returned; no keyword path reachable (grep test: no `bullishTerms` references). Kill LLM ⇒ explicit error surfaces in logs/UI, NOT neutral/keyword value.

### V5 — Persistence & report
After N cycles: `SELECT judgment_type, status, count(*) FROM shadow_judgments GROUP BY 1,2` shows both `ok` and `error` rows; `GET /api/admin/shadow/report` returns agreement/latency/cost/calibration JSON.

### V6 — Config toggles (FR-010)
Flip `shadow:type:news=false` ⇒ news shadow stops, entry continues; global off ⇒ zero records, live unaffected.

## Evidence gate (SC-005)
Window: ≥14 days AND ≥500 `ok` pairs per type. Decision per type: adopt / keep shadow / reject — from report data only.
