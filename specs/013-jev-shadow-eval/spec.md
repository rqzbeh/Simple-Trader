# Feature Specification: Jev + 9Router Decision Core

**Feature Branch**: `013-jev-shadow-eval`

**Created**: 2026-09-27

**Status**: v3.0 — decision-core unification (folded shadow evaluation)

**Input**: User description: "spec-013: Jev (TypeSafe) + 9Router decision core; news gathering and technical analysis become context-only feeders with no agency; remove old LLM path, keyword lexicon, fallbacks, and any racing/conflicting decision logic; shadow evaluation of the core against outcomes"

## Context

**Architecture law (v2):**

1. **Exactly one decision core**: Jev + 9Router. Every trading decision (entry, exit, news impact) is owned by this pair. Nothing else votes.
2. **Everything else is context**: news gathering, indicators, gates, LSTM model — read-only feeders that build one state object. No thresholds, vetoes, weights, or heuristics with independent agency.
3. **Routing**: Jev answers first by default (fast, typed, cheap). When Jev confidence is below a configured threshold, or the question needs free text (thesis, Telegram), 9Router is invoked — 9Router's answer is final when invoked. Escalation is designed routing, never error masking. Failures are explicit errors (FR-007), never substituted outputs.
4. **Futures vocabulary (hard constraint)**: all judgments use position intent `LONG`/`SHORT`/`NO_TRADE`. Order-side words (`BUY`/`SELL`) exist only in the execution layer (short entry = SELL order, short exit = BUY order). The core never sees order sides.

**Removed by this spec**: keyword lexicon (`AnalyzeNewsSentiment`, `bullishTerms`/`bearishTerms`), `fallbackHeuristic` (11 call sites in `internal/ai/client.go`), the legacy LLM decision path as autonomous judge, local LSTM as decision input (demoted to statistics feeder), and any gate/confluence/sentiment-veto logic acting as an independent decider.

**LSTM role (clarified)**: the trained model contributes only historical fact: success/failure rates of indicator signals and their most impactful features, as context fields. It never decides.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decision core owns entry (P1)

On each cycle the context layer builds one state object; Jev answers entry intent; low-confidence cases escalate to 9Router; the resulting typed decision flows to execution. No other component can change or veto it.

**Why this priority**: defines the new architecture; everything else depends on it.

**Independent Test**: Drive cycles with fixed state; verify exactly one record shows core decision + routing path (jev-direct vs 9router-escalated); verify no gate/heuristic/LSTM path altered the outcome (grep test: removed symbols absent).

**Acceptance Scenarios**:
1. **Given** state S, **When** Jev confidence ≥ threshold, **Then** decision = Jev choice, route = `jev_direct`, latency <1s.
2. **Given** state S, **When** Jev confidence < threshold, **Then** 9Router is invoked, its answer is the decision, route = `escalated`, both steps recorded.
3. **Given** Jev times out, **Then** explicit error with component+cycle_id; no heuristic substitution (`fallbackHeuristic` deleted — compile-verified).
4. **Given** shadow flag on, **When** decision produced, **Then** outcome-linked record written; orders byte-identical to flag off.

### User Story 2 - News feeds the core (P2)

Headline clusters are classified by the core (Jev Choice first, 9Router escalation) with semantic understanding; result is a context field for entry/exit plus a standalone news record. Keyword lexicon gone.

**Why this priority**: news is the highest-quality context input; already user-approved for replacement (Option A).

**Independent Test**: Feed clusters incl. negation/slang cases; assert core labels, explicit errors on failure, and zero references to deleted lexicon.

**Acceptance Scenarios**:
1. **Given** cluster C, **When** core classifies, **Then** label ∈ {BULLISH, BEARISH, NEUTRAL, MIXED} with distribution, stored with baseline/outcome.
2. **Given** classification failure, **Then** explicit error surfaces; no neutral default, no keyword path (deleted).

### User Story 3 - Exit owned by core (P3)

For each open position, the core judges exit-now with the same Jev-first/9Router-escalated routing; existing ATR/SL levels become context fields (suggested levels), not independent triggers.

**Why this priority**: exits affect expectancy but depend on US1 routing existing.

**Independent Test**: Open positions + force low Jev confidence; verify escalated exit decision recorded, route logged, no non-core trigger fired.

**Acceptance Scenarios**:
1. **Given** open position P, **When** core judges, **Then** exit decision + route recorded, realized outcome joinable.
2. **Given** ATR stop level provided as context, **Then** core receives it as data; no code path executes it without a core decision.

### User Story 4 - Evidence report (P4)

Comparison report over ≥14 days / ≥500 records per type: routing distribution, agreement vs realized outcomes, latency, cost, calibration — the keep/adopt/reject evidence gate.

**Why this priority**: SC-005 evidence gate; needed before tuning thresholds or expanding automation.

**Independent Test**: Generate records; assert report fields present and computable from stored data alone.

### Edge Cases

- Jev confident and 9Router also invoked? **Forbidden** — one final writer per decision (route recorded).
- Escalation failure after Jev low-confidence: explicit error; decision NOT downgraded to Jev guess or heuristic — cycle marked failed for core, surfaced.
- Empty/degenerate state (no candles, no headlines): explicit error naming missing inputs; no "neutral" fabrication.
- Config threshold unreachable/missing: startup error (no silent default).
- LSTM model file missing: explicit feeder error recorded in state; core still runs with field marked error (stats are context, not decision).
- Credentials: server-side env only (FR-006).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST have exactly one decision path for entry, exit, and news impact: Jev-first, 9Router-escalated. No other component may emit, veto, or modify a decision.
- **FR-002**: Every decision MUST record: state reference, route (`jev_direct`/`escalated`), full probability distribution, confidence, latency, usage/cost, and the core's final answer.
- **FR-003**: Jev MUST be consulted first for entry, exit, and news classification; 9Router MUST be invoked when confidence < configured threshold or free text is required; 9Router's answer is final when invoked; both steps recorded.
- **FR-004**: News classification vocabulary: `BULLISH/BEARISH/NEUTRAL/MIXED`. Entry vocabulary: `LONG/SHORT/NO_TRADE` (FR-005 position-intent law).
- **FR-005**: Position-intent vocabulary only in the core; order-side conversion stays solely in the execution layer.
- **FR-006**: Credentials server-side env only; never logged, committed, or exposed.
- **FR-007**: All failures MUST surface as explicit errors (component + reason + cycle_id). Forbidden: heuristic substitution, default/neutral injection, silent downgrade, retry-until-appears. This applies to Jev, 9Router, feeders, and report.
- **FR-007a**: `fallbackHeuristic` and all 11 call sites MUST be deleted (compile-verified test). No new fallbacks in this feature.
- **FR-013**: Keyword lexicon (`AnalyzeNewsSentiment`, word lists) MUST be deleted. 9Router LLM classifier (structured output, evidence→reasoning→label order) is the semantic classifier paired under the core, not an independent agent.
- **FR-014**: Technical indicators, gates, and LSTM MUST be demoted to context feeders: their outputs are data fields in the state object; any prior independent agency (thresholds, vetoes, confluence weights as decision rules) MUST be removed from the decision path.
- **FR-015**: LSTM feeder MUST contribute only historical statistics (indicator success/failure rates, feature impact) as labeled context fields; MUST NOT vote, weight, or block a decision.
- **FR-016**: Routing threshold MUST be configurable at runtime; missing threshold MUST be a startup error.
- **FR-017**: Shadow/outcome evaluation (≥14 days or ≥500 records/type) MUST produce agreement, latency, cost, routing distribution, and outcome-conditioned calibration per judgment type.
- **FR-018**: Live promotion beyond current scope (auto-tuning thresholds, new automations) requires follow-up spec gated on FR-017 evidence.

### Key Entities

- **State Object**: single build per cycle — market data, indicators (values only), news labels, LSTM stats, ATR levels, gates-as-fields. Feeders cannot act on it.
- **Core Decision**: final answer + route + distribution + confidence + cost + cycle_id.
- **Routing Threshold**: confidence cutoff separating `jev_direct` from `escalated`.
- **Feeder Result**: value or explicit error per feeder, embedded in state.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of decisions carry exactly one route record; zero decisions originate outside the core (verified by code scan + tests: removed symbols absent).
- **SC-002**: ≥95% of cycles complete `jev_direct` within 1s; escalated cycles recorded with both latencies.
- **SC-003**: Evidence report computable from stored data: agreement with realized outcomes, latency p50/p95, cost per 1,000 decisions, calibration buckets.
- **SC-004**: Daily cost per judgment type measurable.
- **SC-005**: After ≥14 days AND ≥500 records/type, team makes keep/tune/reject decision per type from report data only (Constitution VIII).
- **SC-006**: Core probabilities shown calibrated: records claiming ~70% success materialize near 70%.
- **SC-007**: Zero silent-degradation incidents: every injected failure test produces a surfaced explicit error (chaos test suite).

## Assumptions

- TypeSafe key already provisioned (`TYPESAFE_API_KEY`); 9Router endpoint existing (`NINEROUTER_*`).
- Removal of autonomous paths is complete-scope, not flag-guarded: deleted code stays deleted (Constitution IV-style purge).
- Evaluation runs on current paper/simulated flow; thresholds start from research.md defaults and tune only via FR-017 evidence.
- Migration lands in root `migrations/` (dual-directory rule).
- Trader-facing UI changes out of scope; admin JSON report only.

## Convergence (2026-09-29) — no-signal defect

**Defect**: escalation closure in `server.go` ignored its payload and sent `Analyze(DecisionRequest{Symbol: "escalated", IndicatorSnap: {}})` — 9Router evaluated an empty body, answered `HOLD`, which mapped to `NO_TRADE` and overwrote Jev's directional choice. With `ROUTING_CONFIDENCE_THRESHOLD=0.75` (set 2026-09-28 12:44) above Jev's max observed confidence (0.64), every candidate escalated → 304 HOLD:route=escalated → zero signals since 2026-09-28 11:17.

**Fix (FR-003/FR-007)**:
- `trader.EscalationPayload(payload)` normalizes inputs: `ai.DecisionRequest` passes through; `StateObject` (shadow Route path) is converted via `stateToRequest` (inverse of snapToMap); any other type = explicit error — never an empty body.
- Trade path now passes the FULL entry `req` (symbol, indicators, headlines, bucket, catalysts) to `Escalate`.
- Regression: `TestEntryPath_EscalationReceivesRealPayload`, `TestEscalationPayload_*`.

**Evidence**: `go test -run "Escalation" ./internal/trader/` → 4 PASS; full suite 11/11.
