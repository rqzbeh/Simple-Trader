# Implementation Plan: Dynamic Trade Timeframe Selection

**Branch**: `016-dynamic-timeframe` | **Date**: 2026-09-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/016-dynamic-timeframe/spec.md`

## Summary

Add `timeframe` as a second Choice question batched into the spec-013 entry request. Bucket-scoped option sets (ALPHA `15m/1h/4h`, CORE `1h/4h/12h`, config keys via settings persistence). Signal persists chosen timeframe + distribution; horizon derives from a timeframe-keyed profile map (replacing static 60m/240m bucket horizons at entry); exit-decay/reconciler switch to per-signal timeframe with recorded legacy migration.

## Technical Context

**Language/Version**: Go 1.24 (CGO_ENABLED=0)

**Primary Dependencies**: spec-013 `DecisionRouter`/`JevClient.Evaluate` (batched questions), `internal/config/risk_profiles.go` (static horizons today), candle provider already supports arbitrary intervals (`internal/market/binance_historical.go:63`), settings persistence PUT allowlist

**Storage**: PostgreSQL — signal row gains `timeframe` + `timeframe_distribution` columns (migration `000009`, ROOT migrations/)

**Testing**: Go unit (profile map, question building, invalid-answer errors) + integration (forced answers through entry path); live `-tags=liveapi`

**Target Platform**: Linux VPS Docker Compose

**Performance Goals**: zero added latency — timeframe rides the existing batched request (spec-013 p95 budget unchanged)

**Constraints**: explicit errors only (missing timeframe = loud entry failure, FR-202/207); empty choice set = startup error (FR-203); no silent legacy behavior change — unmigrated rows recorded (FR-206)

**Scale/Scope**: 2 buckets × 3 options; ~6 file touch points (profiles, signals, question builder, reconciler, migration, config allowlist)

## Constitution Check

| # | Gate | Status | Notes |
|---|------|--------|-------|
| I | Pure Go | ✅ | |
| II | Redis/PG/SSE | ✅ | new columns audited in PG |
| III | PWA | ✅ | no required UI (config keys editable via existing settings card) |
| IV | No Iranian market | ✅ | |
| VI | TDD | ✅ | question builder + profile map + decay tests first |
| VII | Spec-Kit | ✅ | this artifact |
| VIII | Evidence params | ✅ | horizon maps derive from research evidence (/tmp/timeframe-research.md); choice sets tunable via config, SC-203 measures distribution before defaults harden |

**Violations**: none.

## Project Structure

### Documentation (this feature)

```text
specs/016-dynamic-timeframe/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/timeframe.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/config/
├── risk_profiles.go        # MODIFY: TimeframeProfile map {horizon,be,flat}; legacy bucket rows
└── timeframe_config.go     # NEW: bucket→option sets from env (startup error if empty)
internal/trader/
├── signals.go              # MODIFY: batch "timeframe" question; persist choice+distribution; horizon from map
├── timeframe.go            # NEW: question builder (criteria per FR-208) + answer validation
└── timeframe_test.go       # NEW (TDD)
internal/server/
├── signal_handlers.go      # MODIFY: applyDecayState reads signal.Timeframe (FR-206)
└── system_handlers.go      # MODIFY: allowlist TIMEFRAME_SET_ALPHA / TIMEFRAME_SET_CORE
migrations/
└── 000009_signal_timeframe.up.sql|.down.sql    # ROOT only
```

**Structure Decision**: follow-the-caller; no new packages.

## Complexity Tracking

No violations.
