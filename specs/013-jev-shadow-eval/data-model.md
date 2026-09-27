# Data Model: spec-013 Jev Shadow Evaluation

## Entity: Shadow Judgment Record (table `shadow_judgments`)

One row = one shadow evaluation attempt (success or failure).

| Field | Type | Notes |
|---|---|---|
| `id` | bigserial PK | |
| `created_at` | timestamptz | cycle time |
| `judgment_type` | text, CHECK in (`entry`,`exit`,`news`) | US1/US2/US3 |
| `cycle_id` | text | correlates all judgments of one decision cycle; also appears in error messages (FR-007) |
| `symbol` | text NULL | NULL for news clusters |
| `news_cluster_id` | bigint NULL | FK to cluster store when `judgment_type='news'` |
| `state_ref` | text | pointer/hash of input state (kline window id / cluster id / position id) — reproducibility, not full payload duplication |
| `judge` | text, CHECK in (`jev`,`llm`) | which model produced the record |
| `choice` | text NULL | `LONG/SHORT/NO_TRADE`, `BULLISH/BEARISH/NEUTRAL/MIXED`; NULL for failures |
| `noul` | double precision NULL | exit-now / catalyst probability |
| `score` | double precision NULL | severity rubric position |
| `probabilities` | jsonb NULL | full distribution (Choice/Score) |
| `confidence` | double precision NULL | 0..1, Jev-derived from distribution |
| `baseline_choice` | text NULL | existing pipeline's decision for same cycle (LLM decision for entry/exit, LLM news classifier for news) — the comparison pair (FR-008) |
| `latency_ms` | integer | end-to-end |
| `input_tokens` / `output_tokens` | integer | usage (FR-009 cost report) |
| `status` | text, CHECK in (`ok`,`error`) | |
| `error` | text NULL | explicit reason: component + cause (FR-007). Mandatory when `status='error'`, forbidden when `ok` |
| `outcome` | double precision NULL | realized outcome joined later: entry/exit → trade ROI; news → forward return window (SC-006) |

Indexes: `(judgment_type, created_at)`, `(cycle_id)`, `(status, created_at)`, partial index on `outcome IS NULL` for pending joins.

Validation rules (enforced in store layer, tested):
- `status='error'` ⇒ `error` non-empty, `choice/noul/score` NULL.
- `status='ok'` ⇒ type-specific value present (`entry`: choice ∈ {LONG,SHORT,NO_TRADE}; `exit`: noul ∈ [0,1]; `news`: choice ∈ 4-class set).
- `judge='jev'` and `probabilities` present ⇒ keys of `probabilities` match allowed choice set (schema-validity proof, SC-001).

## Entity: Shadow Config (Redis dynamic config, keys under `shadow:`)

| Key | Type | Meaning |
|---|---|---|
| `shadow:enabled` | bool | global kill switch |
| `shadow:type:entry` / `:exit` / `:news` | bool | per judgment type (FR-010) |
| `shadow:llm_news_enabled` | bool | live LLM news classifier toggle (false ⇒ explicit config error, NOT keyword fallback — FR-007a) |

## Entity: Comparison Pair (derived view, not stored)

Join `shadow_judgments s` with trade/cluster outcomes:
- agreement: `s.choice = s.baseline_choice` rate per `judgment_type`
- latency: median/p95 of `latency_ms` per `judge`
- cost: `input_tokens` × price − `output_tokens`×0 per 1,000 judgments
- calibration: bucket `confidence` (or choice probability), compare vs `outcome` frequency (SC-006)

Report thresholds for keep/adopt/reject are read from this view; gating logic lives in spec evidence protocol (SC-005), not hardcoded.

## State transitions (per record)

`pending` (in-flight, not persisted) → `ok` or `error` (terminal). No updates after terminal except `outcome` backfill job (separate, additive).

## Jev request/response contract (see contracts/jev.md)

- Request: `{state, model:"jev-latest", questions:{entry/exit/news...}}` → all three types are independent questions in ONE request when co-triggered (parallel design rule).
- Response: `answers.{id}.{type, choice|noul|score, probabilities, confidence}`, `usage`.
