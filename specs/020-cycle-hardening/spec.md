# spec-020: cycle & signal-path hardening — event-driven news evaluation + staged-target correctness + legacy sweep

**Input**: user 2026-09-29 — (a) "event-driven: when news comes and shows direction, re-evaluate" replacing blind cadence; (b) "check out EVERYTHING in spec-021 to get rid of legacy/problematic code, harden tests so YOU catch bugs, not me"; (c) "append it directly to spec-020". Status: Part B (staged targets/legacy) implemented in-tree; Part A (event-driven) designed below.

## Part A — event-driven evaluation (replaces cadence tuning)
- **US-A1**: new catalyst cluster for symbol X ⇒ enqueue symbol (dedupe + 60s debounce) ⇒ immediate entry evaluation through the same pipeline; open positions keep re-judging via spec-014 early-exit on the same trigger.
- **US-A2**: `SCAN_INTERVAL_MINUTES` (optional, .env): unset ⇒ event-driven only; set N ⇒ backstop sweep every N minutes (science note: news alpha decays 15-30min — docs/RESEARCH-optimal-params.md). Interval no longer default-hardcoded2m in main.go (spec-017 law: optional key, absence = event-driven-only).
- **US-A3**: scanner overlap guard: a scan/evaluation never runs concurrently with itself for the same symbol (ticker during long run must not stack).

## Part B — staged targets & legacy sweep (was spec-021)
**Input**: user report — AAVE/USDT LONG telegram showed TP2 ($173.11) BELOW TP1 ($175.15), barely above entry ($173.02), asking "what is the research behind this?" Answer: none — it is a bug cluster. Defect convergence (spec-015/016/018 verification caught it live).

## Root causes (verified in code + DB rows 144/145/146)
1. **ATR units, 100× undersized**: `snapshot.NATR = ATR/Price` (fraction), but `signals.go:351` computes `atrPrice = NATR × price / 100` → ATR/100. TP2 = entry + 2.3×(ATR/100) ≈ entry + 0.04% (DB: AAVE ATRAtEntry 0.037 vs true ~3.7). Vol-target code already treats NATR as fraction (`×100`) — the ÷100 line is the lone inconsistent consumer.
2. **Regime override skips TP2**: `atr_regime` sets `TP1AtrMult = tpm` (3-4) but never `TP2AtrMult` (stays 2.3) → TP1 > TP2 whenever Jev picks NORMAL/WIDE.
3. **No TP price bounds ever applied**: `MIN/MAX_TAKE_PROFIT_PCT` are loaded into config but never clamp tp1/tp2 (dead enforcement).
4. **Ordering not invariant**: nothing guarantees TP2 > TP1 after RR-floor stretching.
5. **Telegram lies about timeframe**: formatter hardcodes "2-Hour Swing Setup" while signal timeframe = 15m.

## Requirements
- **FR-701**: `atrPrice = snap.NATR × entryPrice` (fraction semantics documented at both sites).
- **FR-702**: regime override treats `tp_atr_mult` as the RUNNER target: `TP2AtrMult = tpm`, `TP1AtrMult = tpm × (baseTP1/baseTP2)` (preserve staged ratio ≈0.33); SL override unchanged.
- **FR-703**: after RR-floor stretch, clamp tp1/tp2 into `[entry×(1±MIN_TAKE_PROFIT_PCT), entry×(1±MAX_TAKE_PROFIT_PCT)]` by direction; clamp events recorded in `ParameterClamps`.
- **FR-704**: hard invariant LONG: `tp2 ≥ tp1`; if unreachable within MAX_TAKE cap ⇒ `TakeProfit2 = nil` (formatter already omits nil). SHORT mirrored (`tp2 ≤ tp1`).
- **FR-705**: telegram shows the REAL timeframe (`sig.Timeframe`) + derived horizon; legacy nil timeframe ⇒ omit the claim (no hardcoded "2-Hour Swing Setup").
- **FR-706**: tests: units, staged ordering under TIGHT/NORMAL/WIDE, bounds clamp + record, invariant/nil, formatter label.

## Acceptance
Suite green + `-race`; gates G36-G38; docker build; CI; deploy; next signals show TP2 > TP1 (or omitted) and ATRAtEntry ~100× larger; telegram label matches stored timeframe.


## Part C — hardened tests (catch bugs before users do)
- **US-C1**: property suite `TestSignalContract_Invariants`: across {TIGHT,NORMAL,WIDE} × NATR {0.001,0.004,0.05,0.5} × {LONG,SHORT} × min_rr {2.0,4.5}: every emitted signal satisfies — TP within MIN/MAX_TAKE_PROFIT_PCT, TP2 nil-or-strictly-beyond-TP1, SL within MIN/MAX_STOP_LOSS_PCT, rr ≥ min_rr (else explicit take_profit_bound rejection), leverage ≥ 1, ATRAtEntry == NATR×price, clamps JSON valid.
- **US-C2**: formatter contract: real timeframe shown, no fabricated headline/source lines, no hardcoded horizon/RR claims, TP2 line omitted when nil.
- **US-C3**: legacy TEST sweep: fixtures still carrying the removed `entry` mega-question, hardcoded-claim assertions, dead-symbol references (`DefaultSignalConfig`, old BuildState) deleted or updated to current contracts.

## Acceptance
All of: suite 11/11 + `-race`; gates G36-G39 green; docker build; CI; Docker+Playwright verification; deploy; converge reverify.
