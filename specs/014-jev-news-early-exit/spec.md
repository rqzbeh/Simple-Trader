# Feature Specification: Jev News-Driven Early Trade Exit

**Feature Branch**: `014-jev-news-early-exit`

**Created**: 2026-09-28

**Status**: Draft

**Input**: User description: "spec-014: Jev news-driven early trade exit — when fresh news changes the core's conclusion, close the position before SL/TP hits, with Telegram notification of the early exit"

## Context

Spec-013 built the decision core (Jev-first, 9Router-escalated) for entries, exits-on-schedule, and news classification. Today an open position waits for price to hit SL/TP or a scheduled exit judgment. Breaking news that flips the thesis (e.g. long BTC, then "SEC sues exchange") does nothing until price already took the loss.

This feature: while a position is open, new/updated news clusters are re-judged by the core against the **current position** (direction, entry, ATR levels). If the core flips its conclusion — position should no longer be held — the system closes the position immediately (a managed early exit, distinct from stop-loss/take-profit), records why, and notifies the operator via Telegram.

Architecture law from spec-013 continues to hold: the core is the only decision-maker; execution engine only converts intent to orders; explicit errors, zero fallbacks.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - News-triggered early close (P1)

While a position is open, a new headline cluster arrives, the core re-judges the position and answers "do not hold anymore"; the system closes the position at market conditions in the same evaluation cycle, marks the exit reason as news-driven (not SL/TP), and the operator receives a Telegram message naming the cluster, the flip direction, and the realized outcome.

**Why this priority**: the core value of the feature — stop losses before price does.

**Independent Test**: Simulated open position + injected headline cluster + forced core flip → position closed with news exit reason, Telegram message recorded (mock), no SL/TP path involved.

**Acceptance Scenarios**:
1. **Given** open LONG position P and a new cluster C, **When** core judges "hold=false / flip", **Then** P closes with exit reason `NEWS_EARLY_EXIT`, before any SL/TP price touches.
2. **Given** the same, **When** close completes, **Then** a Telegram message is sent containing symbol, direction closed, cluster headline summary, core confidence, and realized PnL.
3. **Given** core judgment fails (timeout/error), **When** cluster arrives, **Then** explicit error logged, position stays open, no close attempted (FR-007, no silent skips).
4. **Given** multiple clusters in one cycle, **When** evaluated, **Then** positions judged once per cycle (batch), not once per headline.

### User Story 2 - Guardrails on early exits (P2)

Early exit obeys hard guards: minimum holding time, maximum early exits per day, and cooldown after an early exit — so news noise cannot churn the book. Every guard rejection is logged with reason.

**Why this priority**: prevents whipsaw from noisy headlines; protects from rate/cost blowups.

**Independent Test**: Inject rapid-fire clusters → at most the configured number of early closes per day; guard rejections visible in logs/records.

**Acceptance Scenarios**:
1. **Given** position age < minimum hold, **When** flip judged, **Then** no close, guard rejection recorded with reason.
2. **Given** early-exit budget exhausted for the day, **When** flip judged, **Then** no close, explicit rejection record.
3. **Given** core confidence below configured floor, **When** flip judged, **Then** no close, record shows insufficient-confidence reason.

### User Story 3 - Evidence & report (P3)

Every early-exit judgment (close, skip-with-reason, error) is recorded with cluster id, core answer, confidence, route, and — once known — realized outcome, joinable with the existing evidence report.

**Why this priority**: Constitution VIII — tune guards/thresholds from recorded evidence.

**Independent Test**: Generate judgments → records queryable; report includes news-exit counts and outcomes.

**Acceptance Scenarios**:
1. **Given** any early-exit evaluation, **When** it completes, **Then** a record exists with judgment values and guard decisions.
2. **Given** outcome later known, **When** backfilled, **Then** report shows early-exit win/loss vs held-to-SL/TP comparison.

### Edge Cases

- News arrives while position in profit near TP: core still decides; guards may block churn but do not force hold.
- Same cluster re-delivered (dedup): judged at most once per position (cluster+position pair identity).
- Telegram down: close still happens (close is the decision; notification failure logged explicitly, not retried silently into duplicates — single explicit retry, then error record).
- Core says flip but execution close fails (market closed, exchange error): position stays open, explicit error surfaced, retry next cycle.
- Weekend commodities: guards unchanged — early exit only reachable when position exists (market open).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-101**: For every open position, each new (deduplicated) news cluster MUST trigger one core judgment: "hold this position?" (typed, Jev-first, escalated per spec-013 routing), batched across positions per cycle.
- **FR-102**: A core "do not hold" verdict MUST close the position in the same cycle with exit reason `NEWS_EARLY_EXIT` (distinct from STOP_LOSS/TAKE_PROFIT/MANUAL).
- **FR-103**: The system MUST send a Telegram message on every executed early exit: symbol, position direction, cluster headline, core confidence, route, realized PnL. Notification failure MUST NOT block or repeat the close — one explicit attempt, failure recorded.
- **FR-104**: Hard guards (configurable, persisted via .env settings): minimum hold time before eligible, max early exits per position-day, post-exit cooldown, minimum core confidence floor. Guard rejections recorded with reason, never silent.
- **FR-105**: Failures (core timeout, Telegram error, close execution error) MUST surface as explicit errors with component + cycle id (spec-013 FR-007). No fallback close, no silent skip, no default verdict.
- **FR-106**: Every judgment (close / guarded-skip / error) MUST be persisted with cluster reference, verdict, confidence, route, guard outcomes — joinable to realized outcome for reporting.
- **FR-107**: Position intent vocabulary only: judgment answers are `HOLD` / `DO_NOT_HOLD` (no order-side words); execution layer performs the close order.
- **FR-108**: Early exits MUST NOT alter SL/TP mechanics for positions not closed early; existing exit paths unchanged.
- **FR-109**: A kill switch (setting) MUST disable early-exit closes entirely (judgment recording continues).

### Key Entities

- **EarlyExitJudgment**: position + cluster + core verdict + confidence + route + guard results + action taken + outcome link.
- **GuardConfig**: min-hold, per-day budget, cooldown, confidence floor — user-configurable settings (single source of truth: environment config, editable in UI per settings-persistence work).
- **Exit reason `NEWS_EARLY_EXIT`**: first-class exit taxonomy value alongside existing reasons.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-101**: News-flip closures happen within one evaluation cycle of cluster arrival (target: seconds-to-minutes after cluster classification, vs waiting hours for price).
- **SC-102**: 100% of executed early exits have a matching Telegram message record and a persisted judgment record (auditable pairing).
- **SC-103**: Zero early exits bypass guards: any injection test of rapid clusters shows budget/cooldown respected (0 violations).
- **SC-104**: Notification failures never produce duplicate closes (0 double-close incidents in failure-injection tests).
- **SC-105**: After 30 days, report answers: early exits vs held-to-target outcome comparison per symbol — data sufficient to tune guards (Constitution VIII).
- **SC-106**: Core verdict latency stays within the news pipeline cadence (verdict available before next cluster batch).

## Assumptions

- Spec-013 decision core + news pipeline are prerequisites (cluster classification already live).
- Telegram bot integration exists (entry/resolution broadcasts today) — reuse channel; no new bot.
- Settings persistence to environment config ships with spec-013 follow-on work; GuardConfig uses the same mechanism (UI-editable, redeploy-surviving).
- Kill switch default: ON (early exits enabled) once evidence window passes; implementation ships enabled for paper/sim flow, off for any live-orders flow until SC-105 evidence exists.
- Paper/simulated execution context (current deployment); "close" = engine position close path already used for manual close.
- One core judgment per position per cycle regardless of headline count (FR-101 batching).
