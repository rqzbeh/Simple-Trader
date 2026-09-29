# Contracts: spec-015

## 1. Resolution (every cycle, per param)

```
value, mode, dist, clamped := Resolve(param, coreAnswer)
override set   → (envValue, "user_override", nil, false)   // core never asked
override empty → clamp(coreAnswer) | err → explicit schema/clamp error (FR-307)
```

## 2. Question builders (only added when Managed)

```json
"min_rr_accept":  {type:"score", criteria:["Reject <1.5","Marginal 1.5-2.5","Accept 2.5-4","Strong >4"], instructions:{question:"Minimum R:R to accept this setup", market:"..."}}
"leverage":       {type:"choice", criteria:{"3x":"max safety...","5x":"...","8x":"...","10x":"...","12x":"..."}, instructions:{liq_buffer:"precomputed ok", regime:"..."}}
"conviction":     {type:"score", criteria:["Weak","Moderate","High","Very high"]} → risk fraction scale [0.5%..cap]
"atr_regime":     {type:"choice", criteria:{"TIGHT":"vol contraction...","NORMAL":"...","WIDE":"expansion, wider stops"}}
"confluence":     {type:"score", criteria:["Reject","Weak","Adequate","Strong"]} → acceptance threshold
"decay":          {type:"choice", criteria:{"FAST_BREAKING":"news dies ≤6h...","MACRO_THEMATIC":"persists days..."}}
```

Pre-computed inputs only (research rule): ATR%, regime label, catalyst age, liq-buffer check — in `instructions` as data.

## 3. Clamps (hard, FR-304)

| Param | Bound |
|---|---|
| leverage | [1, exchange max (125)] AND liq-buffer invariant ×4 (code) |
| conviction risk | [MIN_RISK_PER_TRADE_PCT, MAX_RISK_PER_TRADE_PCT] |
| min_rr | [0.5, 10.0] |
| atr multipliers | [MIN_STOP_LOSS_PCT, MAX_STOP_LOSS_PCT], [MIN_TP, MAX_TP] |
| confluence | [0, 1] |
Clamp ⇒ parameter_clamps row entry (visible), value applied.

## 4. Settings PUT (allowlist additions, all optional — empty clears to Managed)

`MIN_RISK_TO_REWARD_RATIO, DEFAULT_LEVERAGE, MAX_RISK_PER_TRADE_PCT, SL_ATR_MULT, TP_ATR_MULT, CLUSTER_DECAY_MODE, CONFLUENCE_MIN` — existing validators; PUT with "" ⇒ delete key (mode→Managed), respond mode map.

## 5. GET config

per-param: `{"min_rr": {"mode":"core_managed"} }` or `{"mode":"user_override","value":2.5}` — SystemStats Decision Core card shows list.

## 6. Migration

`migrations/000010_signal_parameter_modes.up.sql|.down.sql` — ROOT only.
