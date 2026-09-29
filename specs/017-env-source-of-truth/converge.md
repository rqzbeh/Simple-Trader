# Convergence: spec-017 (2026-09-29)

All 17 tasks implemented. Evidence:

- **FR-401**: `internal/config` has zero `getEnv*(key, default)` calls; `RequiredEnvKeys()` (46 keys) + `SampleRequiredEnv()` + `validateRequiredEnv` (presence + type) + `validateRequiredSemantics` (ranges/bands) produce ONE aggregated boot error. `TestLoadConfig_MissingListsKeys` proves every missing key is named.
- **FR-402**: `main.go` `log.Fatalf` on config/timeframe errors (removed warn-and-continue); `LoadTimeframeConfig()` wired at boot; `liveAlphaSet/liveCoreSet` hardcoded initializers deleted.
- **FR-403**: `api.typesafe.ai` gone from `internal/`+`cmd/` (sample only); `JEV_BASE_URL`/`JEV_MODEL` required; `Evaluate` fails `ErrConfigMissing` on empty base.
- **FR-404**: `DB_MAX_CONNS`/`DB_MIN_CONNS` required; `db.NewStore` errors naming the key.
- **FR-405**: `UpdateTelegramConfigHandler` persists token+chat ID via `UpsertEnv` + `os.Setenv` + cfg update; `TestTelegramConfigHandlers` covers it (ENV_FILE temp).
- **FR-406**: allowlist += `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `AI_BASE_URL` (+URL validator, `Client.SetBaseURL` live apply).
- **FR-407**: both compose files: backend env = PORT/DATABASE_URL/REDIS_URL with `:?` required + ENV/ENV_FILE; postgres init keys `:?`; only `BACKEND_IMAGE:-` remains (deploy convenience, docker-compose.yml).
- **FR-408**: `.env.example` completed (POSTGRES_* + missing required samples appended); README required/optional tables rewritten.
- **FR-409**: `DefaultSignalConfig` → `SampleSignalConfig` (samples from `SampleRequiredEnv`); `newSignalService` nil-cfg = `log.Fatalf`; `getAppConfig` zero-config branch; `initialCap`/`impact` fallbacks deleted (semantic validation instead); `RoutingThreshold` reads env first (source of truth).
- **FR-410**: local `.env` + both VPS `.env` files backfilled (14 local / 34 VPS keys each, in sync); VPS boot verified in deploy step.

## Addendum — TypeSafe/Jev docs audit (user request, docs.typesafe.ai + typesafe@typesafe-ai skill)

Verified against live docs (`/api`, `/primitives/score`, quickstart):

- Request/response contract matches: `POST /v1/systemone`, `{state, model, questions{id:{type,instructions,criteria}}}`, `jev-latest`, Choice criteria = map, Score criteria = ordered array, Noul — **correct as built**.
- **DEFECT FOUND & FIXED**: Score answers return a probability-weighted **level index** (`score`, `probabilities` keyed by level index) — we used the raw index as the semantic value. Confluence threshold clamped to 1.0 → every BUY vetoed (`confluence_threshold`, 6/6 post-deploy). Fix: `LevelValues` per Score param + `evFromAnswer` (EV = Σ p×levelValue); legacy undistributed raw scores keep documented per-param semantics; mock server now returns docs-shaped answers (probabilities + legend).
- Confluence question anchored: carries `current_confluence` + `scale_note` (rubric bands on the real 0.25-0.50 indicator scale).
- Confidence-threshold routing matches the docs' confidence-guidance pattern (thresholds tuned on own data).
