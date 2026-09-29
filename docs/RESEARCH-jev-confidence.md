# Research: Why the decision core never says LONG (2026-09-29)

**Question** (user): pipeline broken, codebase buggy, or reverse Jev↔9Router? How to raise Jev confidence by feeding it better instead of lowering the threshold?

**Sources**: live TypeSafe docs (`docs.typesafe.ai` — `confidence.md`, `concepts/state.md`, `model-jaggedness/jev-1.13.md`, `primitives/choice.md|score.md`, cookbooks), VPS telemetry (`shadow_decisions`, backend logs, `futures_trade_signals`), code audit (`signals.go`, `state.go`, `param_registry.go`, `client.go`).

## 1. Verdict

| Hypothesis | Verdict | Evidence |
|---|---|---|
| Pipeline broken | **NO** | Mechanically healthy: 2-min scans run, 7-question batches answered in 200-300 ms, Postgres/Redis/SSE intact, gates evaluate (they simply receive 0 BUY/SELL inputs) |
| Codebase buggy | **YES (feeding + routing trust)** — previously fixed: empty escalation payload, decay FR-307, confluence Score-scale (all deployed). Remaining: evidence-starved state + trust asymmetry (below) |
| Reverse routing (9Router first) | **NO — not yet** | Both brains are evidence-limited; 9Router is 23× slower (5.3 s vs 0.23 s), non-batchable, and itself answers HOLD at 0.10-0.25. Fix feeding first, measure, then decide with data |

**The numbers (24 h, entry judgments, `shadow_decisions`):**
- **749 / 749 final decisions = NO_TRADE (100%)**. Zero LONG/SHORT ever became final.
- Jev went directional **30×** (21 LONG / 9 SHORT, probability 0.44-0.76) — **all 30 escalated** (all < 0.70) — **all 30 vetoed to HOLD** by 9Router.
- Jev's own NO_TRADE direct rows: mean confidence **0.89** (307 rows).
- Escalated HOLDs accepted as final at confidence **0.10-0.25** (442 rows, mean 0.159).

## 2. Root cause A — evidence starvation (the feeding gap)

Jev's entry question criteria demand 4 pillars: `Price > EMA`, `SuperTrend green`, `catalyst aligns`, `risk gate passes`. Coverage matrix (state actually sent):

| Criterion clause | In state? | Why |
|---|---|---|
| RSI bands | YES | `indicators.rsi` |
| confluence | YES | `indicators.confluence` |
| `Price > EMA` | **NO** | neither price nor EMA in `StateObject` |
| `SuperTrend` | **NO** | dropped by `snapToMap` (along with Regime, VolumeRatio, NATR, VWAP, CMF, CVD, KaufmanER, Divergence — 12 fields dropped) |
| `catalyst aligns` | **NO** | only headline **count** is fed (`polarity:""`, `score:0`) — headline TEXTS and cluster meta (fused sentiment, freshness, sources) are omitted, though the pipeline computed them |
| `risk gate passes` | **NO** | `gates_as_fields` is `{}` (always) |
| leverage `liq_buffer` | **HALLUCINATED** | question contains hardcoded `"precomputed ok"` string, not real data |
| conviction / atr_regime questions | ignore state entirely | question builders receive but never use context |

Meanwhile the 9Router escalation prompt (`client.go BuildUserPrompt`) gets headlines, catalyst events, price, SuperTrend, sentiment — **the evidence exists; it just never reaches Jev**.

Docs backing: "Low confidence … the state doesn't contain enough to go on" (`confidence.md`); `jev-1.13` **cannot count** (headline count is noise) and **is not a calculator** (numeric clauses like `RSI 40-70` in criteria are anti-patterns); missing state facts cited in criteria → literal-reading failure (`model-jaggedness/jev-1.13.md`).

## 3. Root cause B — routing trust asymmetry

The router accepts 9Router's HOLD at confidence **0.10-0.25** as final, while blocking Jev's LONG at **0.76**. One escalation hop, no floor on the escalated answer. A 0.15-confidence HOLD silently overwrites a 0.76-confidence direction — with zero-fallback law that is a silent degradation of judgment quality.

## 4. Root cause C — question shape (docs)

- `NO_TRADE` inside a **relative Choice** acts as a probability attractor: when evidence is missing or mixed, probability pools on the catch-all → NO_TRADE confidence 0.90+, directional options starve (`choice.md`, `skill_suggestion.md`).
- Multi-clause options (trend AND momentum AND catalyst AND gates in one option description) split probability → low confidence.
- Docs pattern: **split** `direction: Choice{LONG,SHORT}` (relative) from **viability**: `Noul`/`Score` (absolute), combine in CODE with explicit policy — atomic judgments, inspectable, tunable.

## 5. Recommended spec-018 — "evidence-complete decision core"

Do NOT lower `ROUTING_CONFIDENCE_THRESHOLD` (monkey patch). Raise real confidence:

- **US1 State enrichment (all precomputed in Go — no math for the model)**: headline texts (top-N per symbol) + cluster meta (fused_sentiment, freshness, sources — already computed), price/change%, semantic buckets (`trend_vwap`/`supertrend`/`rsi_zone`/`volume_spike`/`confluence_band`), regime, NATR, VWAP position, **hydrated pre-AI gate results**, catalyst age, session/hour, and a `trading_policy` block (entry rules as state, per docs).
- **US2 Question redesign**: `direction` = Choice{LONG, SHORT} (no catch-all); `edge` = Noul ("is there a confirmed setup with catalyst?") or Score; criteria = structured rubrics with backtick state paths (`news.headlines`, `indicators.supertrend`), zero numeric comparisons, zero clauses referencing absent state; `NO_TRADE` computed in code from edge + gates + direction availability.
- **US3 Routing trust**: escalated answer carries its own floor — final = the answer with higher confidence (or escalated conf < Jev baseline ⇒ keep Jev + explicit log). No more 0.15 HOLDs silently beating 0.76 LONGs. Keep Jev-first (latency/cost/batching); reverse only after a 14-day measured comparison (SC-305 pattern).
- **US4 Feed quality (secondary)**: publish timestamps/freshness per headline, extra RSS sources for ALPHA majors; feed `catalyst_events.freshness` into state (computed today, never fed).
- **Logging**: `[DECISION] cycle route jev=choice/conf final=choice/conf thr batch_q headlines` per cycle (shipped) + shadow calibration buckets unchanged.

**Success metric**: fraction of entry judgments with directional final choice > 0 AND mean confidence of directional finals ≥ threshold, over 14 days — decided by data, not by threshold edits.
