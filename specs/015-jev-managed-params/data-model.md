# Data Model: spec-015

## Signal row additions (migration 000010, ROOT migrations/)

| Column | Type | Notes |
|---|---|---|
| parameter_modes | jsonb | {"min_rr":"user_override","leverage":"core_managed",...} — all 6 keys always present |
| parameter_values | jsonb | {"min_rr":2.5,"leverage":5,...} final applied values |
| parameter_distributions | jsonb | core-mode only: {"leverage":{"5x":0.6,"8x":0.3,...},"confidence":0.8,...} — NULL/absent for override params |
| parameter_clamps | jsonb | {"leverage":{"requested":25,"applied":12,"bound":"exchange_max"}} — only when clamped |

Validation: parameter_modes covers exactly the 6 phase-1 keys; override mode ⇒ values equal configured env; core mode ⇒ distributions present.

## ParamRegistry (Go, single source)

```go
type ParamSpec struct {
  Key      string  // "MIN_RISK_TO_REWARD_RATIO"
  Mode()   Mode    // Override | Managed — reads live config (empty/absent = Managed)
  Bounds   Bounds  // hard min/max (FR-304) — leverage/conviction/rr/atr mults
  Question func(state) ai.JevQuestion  // nil never (managed-only builders)
  Clamp    func(core any) (applied float64/string, clamped bool, err error)
}
```

Registry keys (phase 1): `MIN_RISK_TO_REWARD_RATIO`, `DEFAULT_LEVERAGE`, `MAX_RISK_PER_TRADE_PCT`, `ATR_REGIME` (derived: SLAtrMult/TP1AtrMult as one Choice), `CLUSTER_DECAY` (news), `CONFLUENCE_MIN` (acceptance score).

Must-stay excluded by absence + test guard list: `MAX_DRAWDOWN_LIMIT_PCT`, `MAX_CONCURRENT_SIGNALS`, `MAKER_FEE_RATE`, `TAKER_FEE_RATE`, `MAX_SLIPPAGE_PCT`, `MAX_TRADE_MARGIN_PCT`, liquidation buffer.

## Batches (contract detail: contracts/managed-params.md)

- **Entry batch** (existing): + `min_rr_accept`(Score), `leverage`(Choice), `conviction`(Score), `atr_regime`(Choice), `confluence`(Score) — added ONLY for Managed params (override ⇒ question omitted — FR-301 core not asked).
- **News batch** (existing classifier call extended or cluster loop): `decay`(Choice FAST_BREAKING/MACRO_THEMATIC) per cluster when Managed.

## State transitions

Signal insert atomic with mode records (same tx as timeframe columns pattern). Modes immutable post-insert; settings change affects next cycle only (FR-308).
