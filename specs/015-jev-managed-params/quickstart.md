# Quickstart validation: spec-015

## Prereqs
spec-013 core live, settings persistence working, stack up.

## Scenarios
- **V1 unit**: registry — all-override mode: 6 params → "user_override", zero questions built, values == env (SC-302).
- **V2 unit**: all-unset mode → questions built only for managed set; core answers clamped to bounds; `clamped=true` recorded when out of bounds (SC-304).
- **V3 unit**: must-stay guard — attempting registry lookup of drawdown/fees/margin → not present (test list).
- **V4 integration**: mocked batch response with param answers → signal row has parameter_modes/values/distributions JSON complete; missing managed answer → explicit error, no legacy constant (SC-301, FR-307).
- **V5 mixed**: set MIN_RR=2.5, leave others empty → record shows per-param modes mixed correctly; UI card shows mode list.
- **V6 settings flip**: PUT "" to clear a key → next cycle mode flips to core_managed within one cycle (SC-306); PUT value back → override immediately.
- **V7 live** (`-tags=liveapi`): real batch with managed params — latency delta vs baseline ≤ noise (SC-303).
- **V8 clamp drill**: mock leverage answer 25x → applied 12x (exchange max), parameter_clamps entry present.

## Evidence gate (SC-305)
14 days: report per param core_managed vs user_override outcomes → decide defaults.
