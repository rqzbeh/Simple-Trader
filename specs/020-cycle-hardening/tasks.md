# Tasks: spec-020

## Part A — event-driven (after Part B verified)
- [ ] TA01 cluster-triggered evaluation queue (dedupe + 60s debounce) wired from catalyst insert
- [ ] TA02 SCAN_INTERVAL_MINUTES optional (unset = event-driven only); main.go stops hardcoding 2m
- [ ] TA03 per-symbol overlap guard in scanner
- [ ] TA04 tests: trigger enqueues once, debounce, no double-run

## Part B — staged targets & legacy sweep (in progress)
- [X] TB001 signals.go: fix ATR units (drop /100), document fraction semantics
- [X] TB002 signals.go: regime override sets TP2=tpm, TP1=tpm×baseRatio (computed pre-override)
- [X] TB003 signals.go: TP bounds clamp (MIN/MAX_TAKE_PROFIT_PCT) + ParameterClamps records
- [X] TB004 signals.go: ordering invariant TP2 beyond TP1 else nil (direction-aware)
- [X] TB005 formatter.go: real timeframe + horizon label; drop hardcoded 2-Hour string
- [X] TB006 tests: units, ordering (3 regimes), clamps+records, nil path, formatter label
- [X] TB007 gates G36-G38; suite + -race; docker build
- [X] TB008 CI, push, deploy, live verify rows show TP2>TP1
- [X] TB009 converge + reverify

## Part C — hardened tests
- [X] TC01 TestSignalContract_Invariants property suite (matrix in spec)
- [X] TC02 TestFormatterShowsRealTimeframe + no-fabrication contract
- [X] TC03 legacy test sweep (entry fixtures, dead symbols, hardcoded-claim asserts)
- [ ] TC04 gates G36-G39 green; suite + -race; docker build; CI; Docker+Playwright; deploy; converge
