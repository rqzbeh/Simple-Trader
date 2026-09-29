# spec-017: .env as single source of truth — zero hardcoded config values

**Input**: user directives 2026-09-29 — "remove the hardcoded values from codebase, move them to env and call them required, I do not want any hardcoded value inside codebase itself, treat .env as source of truth" + "editing telegram bot settings does not overwrite .env and after every reset it falls back"

## Purpose

Every runtime-tunable value must live in `.env` and be REQUIRED (boot fails with an explicit list of missing keys). No in-code default may substitute for a missing key (zero-fallback law). Settings UI writes must persist to `.env` and survive restarts.

## Scope decision

- **In scope (config values)**: every `getEnv*(key, default)` in `internal/config`, early-exit struct defaults, timeframe live-set defaults + missing boot load, DB pool defaults, Jev base-URL fallbacks, server fallback configs (`DefaultSignalConfig`, `getAppConfig` hardcoded branch, `newSignalService` nil branch), settings PUT allowlist gaps (TELEGRAM_*, AI_BASE_URL).
- **Out of scope (code constants, documented)**: decision vocabularies (LONG/SHORT/NO_TRADE), formulas (Kelly, ATR math), API protocol shapes, `ParamRegistry` safety bounds (FR-304 mechanical bounds stay code), static data (RSS feed URLs, screener pair universe). These are not user-tunable configuration.

## User stories

- **US1**: Operator copies `.env.example` → fills secrets → app boots. Delete any non-optional key → boot fails with `missing required keys: …` naming every key.
- **US2**: Operator edits Telegram token/chat ID in UI → values written to `.env` → survive container restart. Same for every settings-PUT key.
- **US3**: `TIMEFRAME_SET_ALPHA/CORE` from `.env` are active at boot (today they are silently ignored until a settings PUT).
- **US4**: Optional = only: 7 spec-015 Jev-managed params, `UPSTREAM_PROXY_URL`, `JEV_DISABLE`, `SHADOW_*`, `ENV`/`ENV_FILE`, test keys.

## Functional requirements

- **FR-401**: `config.Load()` validates presence (via `os.LookupEnv`) of every required key BEFORE parsing; missing keys reported in ONE error, comma-separated, with remediation text. No default values remain in `internal/config` (`getEnv*(key, default)` helpers deleted).
- **FR-402**: `main.go` treats config load failure as FATAL (no "using defaults" warning path). `config.LoadTimeframeConfig()` runs at boot, fatal on error; `liveAlphaSet/liveCoreSet` hardcoded initializers removed.
- **FR-403**: `NewJevClient`/`Base()` lose the `https://api.typesafe.ai` fallbacks; new required `JEV_BASE_URL` key (sample value = typesafe URL). Empty base URL at call time = explicit `ErrConfigMissing`.
- **FR-404**: DB pool: `DB_MAX_CONNS`/`DB_MIN_CONNS` required (samples 25/5); `db.Open` errors naming the key when missing/invalid.
- **FR-405**: Telegram settings POST persists `TELEGRAM_BOT_TOKEN`+`TELEGRAM_CHAT_ID` to `.env` (atomic `UpsertEnv`) + `os.Setenv` live; restart keeps values.
- **FR-406**: Settings PUT allowlist covers `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `AI_BASE_URL` (with URL validation) in addition to existing keys.
- **FR-407**: `docker-compose*.yml` removes every `:-default` value injection; required compose inputs use `${VAR:?VAR required}`. All other keys reach the process via `config.ApplyEnvFile()` (spec-015 addendum).
- **FR-408**: `config.RequiredEnvKeys()` + `config.SampleRequiredEnv()` are the single source; `.env.example` and README required/optional tables derive from them; tests bootstrap via `SampleRequiredEnv()`.
- **FR-409**: Server fallback paths lose hardcoded values: `DefaultSignalConfig` deleted, `newSignalService` nil-cfg = FATAL wiring error, `getAppConfig` returns zero-`Config` only when `config.Load` fails (prod cannot reach: boot is fatal first).
- **FR-410**: Production `.env` (VPS + local) backfilled with every required key (former default values migrate from code INTO `.env`), then boot verified.

## Acceptance

- `grep -rn 'getEnv\w*(".*", ' internal/config` → 0 (no default-arg calls); no numeric default parameter helpers.
- `grep -rn 'https://api.typesafe.ai' internal/` → 0 (moved to `JEV_BASE_URL` in `.env`/`.env.example`).
- Boot with one key removed → error lists exactly that key; boot with full `.env` → healthy.
- Telegram UI save → `grep TELEGRAM_BOT_TOKEN .env` matches → restart → GET config shows it.
- Suite green, `-race` clean, CI matrix green, deployed VPS boots healthy, System Stats renders.
