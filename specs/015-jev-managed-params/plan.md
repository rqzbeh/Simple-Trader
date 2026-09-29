# Implementation Plan: Jev-Managed Trade Parameters with Optional Overrides

**Branch**: `015-jev-managed-params` | **Date**: 2026-09-28 | **Spec**: [spec.md](./spec.md)

**Input**: `/specs/015-jev-managed-params/spec.md`

## Summary

Introduce a **parameter registry**: each phase-1 judgment parameter (min R:R, leverage, conviction, ATR regime, cluster decay, confluence acceptance) maps to (a) an optional override env key, (b) a core question builder, (c) hard mechanical bounds. Resolution per cycle: **override set → user value, core not asked; unset → core answer, clamped to bounds, distribution recorded**. Managed questions ride the existing batched Jev requests (entry batch / news batch). Per-parameter mode persisted on the signal (`user_override` | `core_managed`). Must-stay-code params (audit) excluded structurally.

## Technical Context

**Language/Version**: Go 1.24 (CGO_ENABLED=0)

**Primary Dependencies**: spec-013 DecisionRouter/JevClient batched Evaluate; spec-016 timeframe pattern (question builder + validation + profile map); settings persistence PUT allowlist; `/tmp/jev-offload-audit.md` classification

**Storage**: PostgreSQL — migration `000010_signal_parameter_modes` (ROOT migrations/): `parameter_modes JSONB` + `parameter_values JSONB` on signals; override keys in `.env` (settings PUT)

**Testing**: Go unit (registry resolution, clamp, batch inclusion) + integration (set/unset cycles, record contents); live `-tags=liveapi`

**Target Platform**: Linux VPS Docker Compose

**Performance Goals**: zero added latency — parameter questions batched into existing entry/news requests (FR-309)

**Constraints**: user override absolute (FR-301) · zero-fallback explicit errors (FR-307) · hard bounds never delegated (FR-304/305) · native Go only

**Scale/Scope**: 6 phase-1 params × {registry entry, question builder, clamp, mode record}; touches signals.go sizing/SL-TP paths, news cluster path, settings allowlist, SystemStats UI mode display

## Constitution Check

| # | Gate | Status | Notes |
|---|------|--------|-------|
| I | Pure Go | ✅ | registry + builders pure Go |
| II | Redis/PG/SSE | ✅ | modes in PG signal row; override keys via settings/.env |
| III | PWA | ✅ | settings card shows per-param mode (existing UI pattern) |
| IV | No Iranian market | ✅ | |
| VI | TDD | ✅ | registry/resolution/clamp tests first |
| VII | Spec-Kit | ✅ | this artifact |
| VIII | Evidence params | ✅ | SC-305 evidence window; override path preserves today's measured values; core modes start in paper flow |

**Violations**: none.

## Project Structure

### Documentation (this feature)

```text
specs/015-jev-managed-params/
├── plan.md  research.md  data-model.md  quickstart.md
├── contracts/managed-params.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/trader/
├── param_registry.go      # NEW: ParamSpec{name, EnvKey, Bounds, Question, Clamp, Resolve}
├── param_registry_test.go # NEW (TDD)
├── signals.go             # MODIFY: sizing/SL-TP/confluence read Resolve() instead of static cfg when managed
└── managed_params.go      # NEW: core answers → clamped values + mode record
internal/config/
└── config.go              # MODIFY: override keys loaded (present/absent mode detection)
internal/db/ + migrations/
└── 000010_signal_parameter_modes.up.sql|.down.sql   # ROOT only
internal/server/
├── system_handlers.go     # MODIFY: 6 override keys → PUT allowlist (optional values, empty clears)
└── (report)               # MODE counts per param (SC-305 input)
web/src/components/SystemStatsView.tsx  # MODIFY: per-param mode display ("core-managed" vs value)
```

**Structure Decision**: follow-the-caller; registry lives in `trader` beside signals; no new packages.

## Complexity Tracking

No violations.
