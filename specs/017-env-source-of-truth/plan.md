# Plan: spec-017

**Branch**: main | **Date**: 2026-09-29

## Technical context
Go 1.24; stdlib only (`os.LookupEnv`, `fmt.Errorf` aggregation). Storage: `.env` via `config.UpsertEnv` (atomic) + `config.ApplyEnvFile` (startup hydration, spec-015 addendum). Compose interpolation for `DATABASE_URL`/`PORT`/`REDIS_URL` only.

## Structure
```
internal/config/config.go        # RequiredEnvKeys/SampleRequiredEnv, presence check, delete getEnv* defaults
internal/config/early_exit_config.go  # delete struct defaults → require keys
internal/config/timeframe_config.go   # delete hardcoded live sets
internal/config/config_test.go        # rewrite defaults tests → presence/sample tests
internal/ai/jev.go               # remove typesafe fallbacks; JEV_BASE_URL missing = ErrConfigMissing
internal/db/db.go                # require DB_MAX_CONNS/DB_MIN_CONNS
cmd/trader/main.go               # FATAL on config error + boot LoadTimeframeConfig
internal/server/server.go        # JEV_BASE_URL getenv; newSignalService nil cfg = FATAL
internal/server/system_handlers.go    # allowlist TELEGRAM_*, AI_BASE_URL + validators
internal/server/telegram_handlers.go  # persist to .env + live
internal/trader/signals.go       # getAppConfig zero-config (no hardcoded values), DefaultSignalConfig deleted
docker-compose*.yml              # :? required, drop :- defaults
.env.example / README.md         # required/optional derived from SampleRequiredEnv
specs/017-env-source-of-truth/   # this artifact
```

## Constitution
Pure Go ✅ · PG/Redis via env ✅ · PWA settings unchanged ✅ · TDD (presence/persist tests first) ✅ · Spec-Kit ✅ · Evidence params: bounds come from required env, no code substitution ✅

## Complexity
No violations. Scope note: FR-401 exception list (vocab/formulas/bounds/static data) documented in spec.
