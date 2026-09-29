# Plan: spec-018

**Branch**: main | **Date**: 2026-09-29 | Input: spec.md + docs/RESEARCH-jev-confidence.md

## Technical context
Go 1.24. Evidence already present on `ai.DecisionRequest` (Quote, IndicatorSnap, NewsHeadlines, CatalystEvents, NewsSentiment) — state builder currently discards it. TypeSafe docs: relative Choice vs absolute Noul, confidence = distribution concentration, structured instructions with backtick paths.

## Structure
```text
internal/trader/state.go            # StateObject + BuildState: headlines/catalysts/buckets/gates/policy (real, omitempty)
internal/trader/signals.go          # judgeEntryCore: gates pass-through in decReq, direction+edge batch, composition (FR-503), trust argmax (FR-504)
internal/trader/entry_questions.go? # EntryQuestions → direction + edge (kept in signals.go)
internal/trader/signals.go+tests    # semantic bucket helpers (rsi_zone etc.) pure functions
internal/server/signal_handlers.go  # pre-AI gate outcomes + sentiment into DecisionRequest
internal/ai/types.go                # DecisionRequest: Gates map (pass-through)
specs/018-evidence-core/            # spec/plan/tasks/research(=RESEARCH doc)/contracts/quickstart
GATES.md                            # G28-G31
```

## Constitution
Pure Go ✅ · evidence = state fields only (feeders have no agency) ✅ · TDD ✅ · spec-kit ✅ · evidence params: confidence policy constants documented (0.5 coin-flip floor, min() composition) ✅ · no fabricated data (omit-if-unknown) ✅

## Complexity
No violations. Order: state → questions → composition → trust → tests → gates → CI → deploy.
