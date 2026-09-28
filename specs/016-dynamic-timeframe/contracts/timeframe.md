# Contracts: spec-016

## 1. Batched question (joins spec-013 entry request)

```json
"timeframe": {
  "type": "choice",
  "instructions": {
    "question": "Best execution timeframe for this trade given catalyst age and structure?",
    "catalyst_age_hours": "<precomputed>", "regime": "<precomputed>",
    "not_for": "interval plumbing; code fetches candles for the chosen label"
  },
  "criteria": {
    "15m": "Scalp: catalyst <1h old or volatile breakout; horizon 45m",
    "1h": "Standard: fresh news 1-3h; horizon 120m; news dies ≤6h",
    "4h": "Macro/structural: catalyst persists; horizon 360m",
    "12h": "Session-scale commodity catalyst; horizon 720m"   // CORE only
  }
}
```
Criteria keys generated FROM configured set (never hand-listed — config owns options).

## 2. Answer validation

`choice ∈ set(bucket)` AND distribution keys == set → else `ErrJevSchema` wrapped component=`timeframe` cycle → entry aborts loud. No default.

## 3. Persist (with signal insert, same tx)

`timeframe`, `timeframe_distribution`, `timeframe_confidence` written atomically; failure = whole entry fails (no timeframe-less signal).

## 4. Decay/reconciler

`applyDecayState(signal)` → lookup profile by `signal.timeframe`; NULL → legacy map + `legacy_timeframe_mapped` log + backfill; horizon/BE/flat offsets from profile.

## 5. Settings allowlist additions

`TIMEFRAME_SET_ALPHA`, `TIMEFRAME_SET_CORE` — same PUT validation path (membership in known-interval set, non-empty).

## 6. Migration

`migrations/000009_signal_timeframe.up.sql|.down.sql` — ROOT only (dual-directory rule).
