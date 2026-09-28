# Feature Specification: Dynamic Trade Timeframe Selection by Decision Core

**Feature Branch**: `016-dynamic-timeframe`

**Created**: 2026-09-28

**Status**: Draft

**Input**: User description: "make trade timeframe dynamic by giving the choice to select to Jev — due to current news and indicators and other stuff, which time frame do you choose? sometimes better 1h, but overall news won't last more than several hours like 3h to 6h max; this rule for ALPHA, CORE may get more choices and time frame"

## Context

Research (`/tmp/timeframe-research.md`, spec-014/015 pipeline): holding horizons are **static** — 60m crypto / 240m commodity hardcoded in risk profiles; candles fetched only at 1h. Evidence: crypto news catalysts sustain 1–6h; commodity catalysts span sessions (4–12h+); best multi-timeframe practice = execution timeframe ~4–5× smaller than structural trend timeframe.

Decision core (Jev) picks the timeframe per trade from a bucket-scoped choice set, given news cluster age/strength + indicators + regime — one extra `Choice` question batched into the existing entry request (~0.5s, no extra call). Position-intent law and single-writer rules unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Core chooses timeframe at entry (P1)

When the core judges an entry, it also answers `timeframe` (Choice from the bucket's set). The chosen timeframe drives the candle interval used for indicators/exit decay for that signal; record stores choice + distribution.

**Why this priority**: the feature itself; everything else (exit alignment) depends on it.

**Independent Test**: fixed state → forced timeframe answer → signal row carries that timeframe and horizon derived from it; alternatives not chosen recorded in distribution.

**Acceptance Scenarios**:
1. **Given** ALPHA entry with fresh 2h-old breaking cluster, **When** core answers `1h`, **Then** signal horizon = profile for 1h (120m) and candle interval = 1h for that signal's indicators.
2. **Given** CORE entry with macro catalyst, **When** core answers `12h`, **Then** horizon follows 12h profile mapping.
3. **Given** core omits/invalid timeframe answer, **When** entry judged, **Then** explicit error (no silent default timeframe) — entry fails loud or proceeds only if timeframe answered (FR-202).
4. **Given** same batch, **When** request sent, **Then** `entry` and `timeframe` answered in ONE call (no latency added beyond batch).

### User Story 2 - Choice sets by bucket (P2)

ALPHA set = `15m / 1h / 4h` (news horizon 3–6h cap respected). CORE set = `1h / 4h / 12h`. Sets configurable via settings persistence; Jev criteria describe each option with horizon + typical catalyst decay.

**Why this priority**: user-defined scope; guards against nonsensical choices (12h for a 2h news scalp).

**Independent Test**: unit — bucket→set mapping; Jev question built with correct criteria per bucket.

**Acceptance Scenarios**:
1. **Given** ALPHA symbol, **When** question built, **Then** only 15m/1h/4h offered.
2. **Given** operator edits sets via settings UI, **When** next entry judged, **Then** new sets used (single source of truth: config file).

### User Story 3 - Timeframe-aware exits (P3)

Exit decay timers and reconciler read the signal's chosen timeframe (not static bucket mapping); per-timeframe BE/flat decay offsets apply.

**Why this priority**: wrong-timeframe decay = premature/late exits (current reconciler bug: static fallback).

**Independent Test**: signal with timeframe=15m → decay evaluated on 15m cadence, not 60m.

**Acceptance Scenarios**:
1. **Given** signal with stored timeframe T, **When** decay check runs, **Then** cadence/offsets derive from T via profile map.
2. **Given** legacy signal without timeframe, **When** reconciled, **Then** explicit legacy-fallback record (one-time migration mapping), not silent guess.

### Edge Cases

- Chosen timeframe shorter than news age (e.g. 15m choice, 5h-old news): core's criteria discourage; recorded distribution shows model uncertainty — no hard block (core decides).
- Candle data missing for chosen interval: explicit error, entry fails loud (no fallback timeframe).
- Choice set misconfigured/empty at boot: startup error (no default set).

## Requirements *(mandatory)*

- **FR-201**: Entry judgment MUST add `timeframe` Choice answered in the same batched core request as position intent (no additional round trip).
- **FR-202**: Missing/invalid timeframe answer → explicit error with component+cycle (zero silent default).
- **FR-203**: Bucket choice sets: ALPHA `15m,1h,4h`; CORE `1h,4h,12h` — configurable via settings persistence; empty set at startup = startup error.
- **FR-204**: Signal row MUST persist chosen timeframe + full distribution (evidence record, SC calibration).
- **FR-205**: Horizon for the signal derives from chosen timeframe via profile map (timeframe → horizon/BE/flat offsets), replacing static bucket horizon at entry.
- **FR-206**: Exit-decay/reconciler MUST evaluate per signal's stored timeframe; legacy rows without timeframe get recorded migration mapping (not silent behavior change).
- **FR-207**: Candle/indicator fetch for the chosen interval must exist for the symbol; otherwise explicit error (entry fails loud).
- **FR-208**: Jev criteria per option include horizon minutes and catalyst-decay guidance (ALPHA: news lives ≤6h).
- **FR-209**: Position-intent vocabulary and single-writer law unchanged (spec-013); timeframe is a judgment, guards remain code.

## Key Entities

- **TimeframeChoice**: option label, horizon mapping (m), criteria text; bucket-scoped sets.
- **Signal.timeframe**: persisted choice + distribution (schema addition).
- **Profile map**: timeframe → {horizon_min, be_min, flat_min} — replaces static bucket defaults at entry time.

## Success Criteria *(mandatory)*

- **SC-201**: 100% of new signals carry a core-chosen timeframe (no static-bucket entries) while feature enabled.
- **SC-202**: zero entries proceed without valid timeframe (injection tests: missing answer = 0 silent accepts).
- **SC-203**: ALPHA trades' chosen horizons distribution measurable: median chosen horizon ≤6h for news-driven entries (matches evidence).
- **SC-204**: exit-decay applied at chosen-timeframe cadence for 100% of new signals (legacy rows tracked separately).
- **SC-205**: no measurable latency increase per entry decision (batched — p95 within spec-013 budget).

## Assumptions

- Risk-profile refactor (timeframe-keyed profiles) done in plan phase; existing 60m/240m rows map as legacy.
- Candle provider already supports arbitrary intervals (verified in research) — no data-pipeline change beyond param.
- Research inputs: `/tmp/timeframe-research.md` (omni-skills + codebase-memory + empirical news-decay evidence).
- Settings-persistence allowlist gains the choice-set keys (same PUT config path).
