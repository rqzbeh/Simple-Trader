# spec-018: Evidence-complete decision core (feed Jev, split questions, trust the numbers)

**Input**: user 2026-09-29 — "ok then do it" adopting `docs/RESEARCH-jev-confidence.md`: evidence starvation, trust asymmetry, docs anti-patterns. Constraints: codebase-memory-mcp + unlazy + skills; **no mock/fake product data** (real data only, omit-if-unknown); `.env` stays source of truth (spec-017); **more required keys → optional** (queued as spec-019 after this).

## Problem (measured)
- 749/749 entry finals = NO_TRADE (24h). Jev directional 30× (conf 0.44-0.76) all escalated, all vetoed by 9Router HOLD at conf 0.10-0.25.
- Jev's state omits every evidence pillar its criteria demand: no headline texts, no SuperTrend/price/trend, gates `{}`, fake `liq_buffer` string, count-instead-of-text news (Jev cannot count).
- `NO_TRADE` catch-all inside a relative Choice absorbs probability → NO_TRADE 0.90+, direction starved.

## User stories
- **US1**: Jev judges from the evidence the pipeline already has: real headline texts, real catalyst cluster meta (fused sentiment/freshness/sources), semantic buckets precomputed in Go, hydrated pre-AI gate results, price/trend facts, session time, explicit trading policy. Unknown fields are OMITTED, never fabricated.
- **US2**: The mega-choice is split per docs: `direction` = relative `Choice{LONG,SHORT}` (no catch-all, no numeric clauses, structured rubrics with backtick state paths) + `edge` = absolute `Noul` (viability given the real news/tech evidence). Code composes NO_TRADE.
- **US3**: Escalation stops vetoing stronger answers: final = the **higher-confidence** of composed-Jev vs 9Router; Jev keeps its direction when the escalated brain is less certain. Route records who won.
- **US4**: Every cycle emits `[DECISION]` telemetry (shipped) and shadow rows keep calibration input.

## Functional requirements
- **FR-501**: `StateObject` gains (all real, `omitempty`, never invented): `headlines []string` (from `req.NewsHeadlines`, top 10), `catalysts []` (headlines/story_count/fused_sentiment/freshness/sources from `req.CatalystEvents`), `sentiment` only when `req.NewsSentiment != nil`, `price`, `change_pct` (when present on quote), `indicators` extended with `supertrend`, `trend` (price vs VWAP bucket), `rsi_zone`, `volume_spike` (precomputed bool vs existing `VolumeRatioMin` constant), `confluence_band`, `regime`, `natr`, `session_hour_utc`, hydrated `gates` map (pre-AI checks that actually ran, e.g. `weekend_gap`, `event_blackout`, `polarization`, `calendar_halt`, `catalyst_matched` — real pass/veto outcomes passed through `DecisionRequest`), and `trading_policy` (entry rules as text).
- **FR-502**: Entry batch replaces `entry` with `direction` (`Choice`, vocab `LONG|SHORT`, structured criteria objects — clauses only reference fields present in state, zero numeric-range comparisons) and `edge` (`Noul` — confirmed setup worth a paper trade given `news.headlines` + `catalysts` + `indicators`). Managed params/timeframe unchanged.
- **FR-503**: Composition in code (policy explicit): `edge.prob >= 0.5` ⇒ final = `direction` with `conf = min(direction.conf, edge.prob)`; `edge.prob < 0.5` ⇒ `NO_TRADE` with `conf = 1 - edge.prob`. Missing/malformed answers = explicit schema error (FR-307 law). `NO_TRADE` never appears in the direction vocabulary.
- **FR-504**: Escalation as today (composed conf < `ROUTING_CONFIDENCE_THRESHOLD`), but final writer = argmax confidence: `escalated` when 9Router wins, `escalated_jev_kept` when composed Jev answer wins (ties keep Jev). Both confidences logged in `[DECISION]`.
- **FR-505**: No fabricated values anywhere in state or questions: absent evidence ⇒ field omitted (JSON `omitempty`) or explicit `"unknown"` text in `trading_policy`/criteria as designed — never invented numbers (zero-fallback law extension).
- **FR-506**: Shadow + calibration keep working: shadow receives the split questions (Route already treats `direction` as primary); signal record and gates after composition unchanged.

## Acceptance
- Unit: state JSON contains real headline text + supertrend + gates (fed from request, asserted on literal input); composition matrix (edge hi/lo × LONG/SHORT); trust-floor matrix (esc conf higher/lower/equal); direction vocab rejects `NO_TRADE`.
- Suite 11/11 + `-race` + Docker image build; gates G28-G31; CI matrix green; VPS deploy: `[DECISION]` lines show `route=escalated_jev_kept` / directional finals appearing; shadow rows record split ids.
- Live `-tags=liveapi` smoke unchanged.
