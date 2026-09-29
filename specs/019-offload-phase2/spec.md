# spec-019: offload phase 2 — more required keys become optional (Jev + 9Router choose)

**Input**: user 2026-09-29 — "offload as many required to optional and let jev + 9router choose the values most of the time" (constraints: real data only, .env source of truth, no fabricated values).

## Scope (phase 2 of the offload audit)
- **US1**: `TIMEFRAME_SET_ALPHA` / `TIMEFRAME_SET_CORE` become OPTIONAL. Unset/empty ⇒ core picks ANY known interval (the full `KnownIntervals` set — no whitelist restriction). Set ⇒ restricted to the listed subset (today's behavior). Never a code-default set; the known-interval vocabulary is protocol, not a value.
- **US2**: `EARLY_EXIT_MIN_HOLD_MIN`, `EARLY_EXIT_COOLDOWN_MIN`, `EARLY_EXIT_MAX_PER_DAY`, `EARLY_EXIT_CONF_FLOOR` become OPTIONAL + **core-managed**: unset ⇒ answered per news cycle by the decision core (batched with the existing early-exit `close_now` questions — same request, zero extra round trips); set ⇒ user override wins (spec-015 semantics, recorded). `EARLY_EXIT_ENABLED` becomes a toggle: absent = enabled, only explicit `false` disables (documented convention, not a value default).
- **US3**: hard safety caps stay code (FR-304): min_hold ∈ [0,1440], cooldown ∈ [0,1440], max_per_day ∈ [1,10], conf_floor ∈ [0.5,1] — core answers clamped, clamp recorded. Kill switch logic unchanged.
- **US4**: required-list shrinks: those 6 keys leave `RequiredEnvKeys()`; boot still fails for everything else. `.env.example` moves them to the optional section (commented, with recommended samples from docs/RESEARCH-optimal-params.md).

## Functional requirements
- **FR-601**: `LoadTimeframeConfig` on empty env ⇒ unrestricted mode (no error); invalid listed option still = boot error (FR-203 unchanged for SET values).
- **FR-602**: `ValidateTimeframe` accepts any `KnownIntervals` member when unrestricted; restricted set behavior unchanged.
- **FR-603**: `LoadEarlyExitConfig` no longer errors on absent keys: absent ⇒ managed marker; present ⇒ parsed+validated (invalid = boot error, unchanged).
- **FR-604**: news/early-exit batch adds questions ONLY for managed guards (FR-301); resolved values applied per evaluation cycle; distributions recorded; user overrides applied verbatim.
- **FR-605**: `config.Load` required list = 40 keys (the 6 removed); `SampleRequiredEnv` keeps commented samples for the optional set via new `SampleOptionalEnv()`.

## Acceptance
- Tests: unrestricted timeframe picks e.g. "30m"; restricted set unchanged; managed guard answers resolve+clamp (25/day → 10); override wins; absent-key boot succeeds; invalid value boot fails.
- Suite green, gates G32-G35, CI green, VPS deploy: boot with .env missing those keys, early exit still functioning.
