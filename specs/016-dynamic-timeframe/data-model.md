# Data Model: spec-016

## Signal row additions (migration 000009)

| Column | Type | Notes |
|---|---|---|
| timeframe | text | one of configured set for its bucket; NULL = legacy row |
| timeframe_distribution | jsonb | full Jev distribution e.g. {"15m":0.1,"1h":0.7,"4h":0.2} |
| timeframe_confidence | double precision | from Jev confidence |

Validation on insert: timeframe ∈ bucket set (else entry errors — FR-202); distribution keys = exact set.

## TimeframeProfile (Go map, replaces static at entry)

key: interval label (`15m,30m,1h,2h,4h,6h,12h,1d`) → { HorizonMin, BEOffsetMin, FlatOffsetMin }
Seeded: 15m→(45,15,30), 1h→(120,30,60), 4h→(360,90,180), 12h→(720,180,360) — values from research news-decay evidence; Constitution VIII: tune from SC-203/SC-204 data.

## BucketOptionSets (env, settings-PUT keys)

`TIMEFRAME_SET_ALPHA=15m,1h,4h` · `TIMEFRAME_SET_CORE=1h,4h,12h`
Boot validation: non-empty, every option ∈ known-interval set, entries parse to TimeframeProfile — else startup error.

## Legacy migration

Rows with timeframe IS NULL: at first reconciler touch write {CRYPTO→1h, COMMODITY→4h} + log `legacy_timeframe_mapped` (FR-206, explicit record not silent).

## State transitions

Entry: question built from set → Jev batch → validate ∈ set → persist with signal (atomic with insert) | error aborts entry.
No lifecycle changes post-entry; decay reads stored value.
