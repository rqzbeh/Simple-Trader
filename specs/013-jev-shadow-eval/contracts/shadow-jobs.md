# Contracts: spec-013 shadow evaluation

## 1. Jev client (`internal/ai/jev.go`)

```
POST https://api.typesafe.ai/v1/systemone
Authorization: Bearer <TYPESAFE_API_KEY>
Content-Type: application/json
{ "state": <object|string>, "model": "jev-latest", "questions": { <id>: Question } }
```

Question types (official API):
- `{"type":"choice","instructions":...,"criteria":{opt:desc}}` → `{type:"choice", choice, probabilities{opt:float}, confidence}`
- `{"type":"noul","instructions":...,"criteria":{true:..,false:..}}` → `{type:"noul", noul:float}`
- `{"type":"score","instructions":...,"criteria":[levels...]}` → `{type:"score", score:float, legend, probabilities, confidence}`

Typed Go errors (FR-007, no fallbacks):
- `ErrJevAuth` (401/403), `ErrJevTimeout`, `ErrJevSchema` (answer missing/mismatched type), `ErrJevRateLimit`, `ErrJevUnavailable` (5xx/network)
Each error carries: component=`jev`, cycle_id, HTTP status, cause. NEVER returned answers substituted with defaults.

Design rules (research.md, sourced):
- State pre-computes all math/indicators/timestamps in Go; Jev never does arithmetic.
- Only cycle-relevant fields in state (context rot filter).
- Criteria use concrete situations + `not_for` negative boundaries.
- Independent questions batched in ONE request.

### Question set (final wording in research.md §Proposed question set)
- `entry` choice: LONG / SHORT / NO_TRADE (futures position intent, FR-005)
- `exit_now` noul
- `news_impact` choice: BULLISH / BEARISH / NEUTRAL / MIXED
- `severity` score: [No move, Minor, Tradeable, Market-moving]
- `has_catalyst` noul

## 2. LLM news classifier (replaces keyword lexicon — Option A, FR-013)

```
POST <NINEROUTER_URL>/v1/chat/completions  (existing client, structured output)
model: configured classifier model
messages: system (label definitions + negation/slang rules + balanced few-shot) + user (cluster JSON)
response_format: JSON schema, field order: evidence, reasoning, label  ← research.md: label-last prevents rationalization
```

Output: `{evidence:[str], reasoning:str, label:BULLISH|BEARISH|NEUTRAL|MIXED, confidence:float}`
Errors: same explicit-error pattern (`ErrLLMClassifier*`), cycle_id included. `shadow:llm_news_enabled=false` ⇒ explicit config error at startup — keyword lexicon is DELETED, never consulted.

## 3. Shadow orchestrator (`internal/trader/shadow.go`)

```
JudgeEntry(ctx, cycleID, symbol, state) error   // async goroutine; never blocks live path
JudgeExit(ctx, cycleID, positionID, state) error
JudgeNews(ctx, cycleID, clusterID, headlines) error
```
- Fires AFTER live decision committed (post-hook), goroutine + bounded queue; queue-full ⇒ explicit error record.
- Reads config flags per type (FR-010).
- Persists ok/error record always (FR-002/003/004/007).

## 4. Report API (`internal/server/shadow_handlers.go`)

```
GET /api/admin/shadow/report?days=14&type=entry|exit|news
→ { pairs, agreement_rate, latency{p50,p95} per judge, cost_per_1k, calibration[{bucket, predicted, realized}] }
```
Admin auth only. Read-only (FR-011).

## 5. Migration

`migrations/NNN_shadow_judgments.sql` — ROOT migrations/ only (memory: root dir feeds Docker+compose; sync from internal/db/migrations or schema silently skips).
