# Quickstart validation: spec-014

## Prereqs
`TYPESAFE_API_KEY`, Telegram bot env, spec-013 core live, stack `docker compose up -d`.

## Scenarios
- **V1 unit**: guards table test — each guard blocks with correct reason; kill switch blocks all closes but records continue.
- **V2 unit**: verdict mapping — noul ≥ floor → close path invoked; < floor → HOLD record; core error → error row, position untouched.
- **V3 integration**: open paper position + inject cluster (test hook) → close with NEWS_EARLY_EXIT, telegram mock called exactly once (SC-102), judgment row present.
- **V4 failure-injection**: Telegram down → close still happens, telegram_error recorded, **no second close** (SC-104); core timeout → explicit error, no close (FR-105).
- **V5 rapid-fire**: N clusters < budget → budget respected, guarded_skip rows with reason (SC-103).
- **V6 end-to-end**: real Jev call (`-tags=liveapi`), real cluster from crawler, paper close, Telegram on configured channel.
- **V7 settings**: PUT guard key → .env updated + live value applied + restart survives.

## Evidence gate (SC-105)
30 days: report compares NEWS_EARLY_EXIT outcomes vs held-to-SL/TP per symbol → tune guard defaults.
