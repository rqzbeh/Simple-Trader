# Tasks: spec-017

- [X] T001 config: RequiredEnvKeys + SampleRequiredEnv + presence check aggregate error; delete getEnv/getEnvFloat/getEnvInt defaults
- [X] T002 config: Load parses from required map; presence+parse tests rewritten (TestLoadConfig_MissingListsKeys, Sample→Load success)
- [X] T003 early_exit: drop struct defaults, require 5 keys; tests updated
- [X] T004 timeframe: drop hardcoded live sets; main calls LoadTimeframeConfig fatal-on-error; test
- [X] T005 ai/jev: remove typesafe fallbacks, add JEV_BASE_URL ErrConfigMissing; update 4 call sites
- [X] T006 db: require DB_MAX_CONNS/DB_MIN_CONNS with explicit error
- [X] T007 main: config failure = FATAL (remove warn-and-continue)
- [X] T008 server: newSignalService nil cfg = FATAL; getAppConfig loses hardcoded branch; DefaultSignalConfig deleted
- [X] T009 telegram: POST persists to .env (UpsertEnv+Setenv); unit test asserts file content
- [X] T010 settings allowlist + validators: TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, AI_BASE_URL (+ live apply where supported)
- [X] T011 compose: :? required inputs, remove :- value defaults (both files)
- [X] T012 .env.example regenerated: every required key with sample value; optional keys commented
- [X] T013 README required/optional tables updated (derived from SampleRequiredEnv)
- [X] T014 backfill VPS + local .env with required keys (former defaults), sync both VPS files
- [X] T015 gates G23-G27 added; suite + -race + docker build green; CI matrix green
- [X] T016 deploy VPS (image pull loop), boot verify: health, config keys, telegram persist, timeframe set active, missing-key drill (local)
- [X] T017 converge: spec evidence appended, gates reverify ALL MET
