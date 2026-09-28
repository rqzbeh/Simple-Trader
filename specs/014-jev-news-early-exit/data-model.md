# Data Model: spec-014

## Table `early_exit_judgments` (migration 000008, ROOT migrations/)

| Field | Type | Notes |
|---|---|---|
| id | bigserial PK | |
| created_at | timestamptz | |
| cycle_id | text NOT NULL | explicit-error correlation |
| position_id | bigint NOT NULL | engine/DB trade id |
| symbol | text NOT NULL | |
| cluster_id | bigint | news cluster ref |
| cluster_headline | text | first headline (audit) |
| verdict | text CHECK IN ('HOLD','DO_NOT_HOLD') | NULL on error rows |
| noul | double precision | close probability [0,1] |
| confidence | double precision | |
| route | text CHECK IN ('jev_direct','escalated') | |
| guards_passed | boolean | |
| guard_reason | text | present when guards_passed=false (min_hold/budget/cooldown/conf_floor/kill_switch) |
| action | text CHECK IN ('closed','guarded_skip','error') | |
| telegram_sent | boolean | NULL unless closed |
| telegram_error | text | explicit if send failed (FR-103) |
| close_error | text | explicit if close failed (retry next cycle) |
| status | text CHECK IN ('ok','error') | error ⇒ error_message non-empty |
| error | text | component+cause |
| outcome | double precision | realized ROI, backfilled |

Constraints: `status='error'` ⇒ error non-empty AND verdict NULL. Unique dedup: UNIQUE(position_id, cluster_id) where action IN ('closed','guarded_skip','error') — one judgment per position/cluster pair (edge case: re-delivery).

Indexes: (position_id, created_at), (cycle_id), (created_at), partial outcome IS NULL.

## GuardConfig (.env keys, UI-editable via settings persistence)

`EARLY_EXIT_ENABLED` (bool, default true), `EARLY_EXIT_MIN_HOLD_MIN` (int ≥0), `EARLY_EXIT_MAX_PER_DAY` (int ≥1), `EARLY_EXIT_COOLDOWN_MIN` (int ≥0), `EARLY_EXIT_CONF_FLOOR` (float 0..1). Startup validation: invalid → startup error (FR-007 pattern, no silent default clamp).

## State transitions (per judgment)

pending → closed | guarded_skip | error (terminal). close_error rows re-evaluable next cycle (dedup key excludes failed closes? NO — dedup UNIQUE applies per pair; close failures get action='error' and allow one re-judgment per cycle until closed or guarded; enforce with ON CONFLICT on (position,cluster, action IN closed/guarded_skip) semantics — plan detail for tasks phase).

## Exit taxonomy

`NEWS_EARLY_EXIT` joins STOP_LOSS/TAKE_PROFIT/MANUAL/... — flows into existing reports (exit_reason filters) unchanged.
