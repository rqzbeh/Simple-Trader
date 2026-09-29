# Tasks: spec-019

- [X] T001 config: TIMEFRAME_SET_* + EARLY_EXIT_* removed from RequiredEnvKeys; SampleOptionalEnv added
- [X] T002 timeframe: empty env = unrestricted; ValidateTimeframe accepts KnownIntervals when unrestricted; tests
- [X] T003 early_exit: absent keys = managed (no error); present = validated; tests updated
- [X] T004 news/early-exit batch: managed guard questions (min_hold/cooldown/budget/floor) batched with close_now; caps + clamp records
- [X] T004b early-exit real state: build non-null state from real position/cluster data (zero-fake-data), pass to jev.Evaluate, test non-null state serialization
- [X] T005 guards consume per-cycle resolved values (override → verbatim); distribution recorded
- [X] T006 settings PUT allowlist unchanged (overrides still writable; empty = clear → managed)
- [X] T007 .env.example: move 6 keys to optional section with commented science-based samples
- [X] T008 README optional section updated
- [X] T009 gates G32-G35; suite + -race green; docker build
- [X] T010 CI green; push; deploy VPS; boot verify (missing keys OK, early exit works)
- [X] T011 converge + gates reverify
