# Quickstart validation: spec-016

## Prereqs
spec-013 core live, `TYPESAFE_API_KEY`, stack up, settings persistence working.

## Scenarios
- **V1 unit**: boot validation — empty set / unknown option / missing profile → startup error listing key (FR-203).
- **V2 unit**: question builder — criteria keys == configured set exactly; ALPHA never offers 12h.
- **V3 unit**: answer validation — mocked Jev returns `1h` ∈ set → persist path; returns `2h` (not in set) → typed error, entry aborted (SC-202 zero silent accepts).
- **V4 integration**: forced batch response → signal row carries timeframe + distribution + confidence; horizon == profile[timeframe].HorizonMin (not 60/240 static).
- **V5 decay**: signal timeframe=15m → decay evaluated on 15m cadence; legacy NULL row → mapped + logged once (FR-206).
- **V6 settings**: PUT `TIMEFRAME_SET_ALPHA=1h,4h` → .env updated + next entry offers only 1h/4h (restart-survivable).
- **V7 live** (`-tags=liveapi`): real Jev answers timeframe in batch; latency delta vs entry-only request ≤ noise (SC-205).

## Evidence gate
SC-203: after 2 weeks, ALPHA chosen-horizon median ≤6h for news entries; SC-204: decay cadence matches chosen timeframe for 100% new signals.
