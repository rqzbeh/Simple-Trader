# Feature Specification: Trading System Optimization

**Feature Branch**: `012-trading-system-optimization`

**Created**: 2026-09-26

**Status**: Draft

**Input**: User description: "Trading System Optimization: evidence-based entry/SL/TP/leverage/sizing, news fusion, and commodities section — all 8 gaps G1-G8 from docs/RESEARCH-trading-system-optimization.md"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Entries only fire on confirmed setups (Priority: P1)

An operator watches the live signal feed. A catalyst headline arrives, but instead of a market
entry being issued instantly, the system checks technical confirmation: price relative to the
session average and trend, a genuine volume expansion around the catalyst, and an anti-chase rule
that blocks buying into an extreme short-term spike. Only setups that pass every gate produce a
signal; everything else is recorded as a filtered (skipped) entry with the reason shown to the
operator.

**Why this priority**: Entries are the root of the whole book. Currently signals enter at the
market instantly on news sentiment, which buys local tops; every downstream metric (stop-out
rate, expectancy) inherits this defect. Fixing entries without fixing exits still yields value,
hence P1 as a standalone slice.

**Independent Test**: Can be fully tested by replaying the last 7 days of headlines + candles:
the system must produce fewer, higher-quality entries than today, and every rejected entry must
appear in a filter log with a machine-readable reason.

**Acceptance Scenarios**:

1. **Given** a headline arrives while price is more than 2 standard deviations above its short-term average, **When** the catalyst would otherwise trigger a BUY, **Then** no signal is issued and a "CHASE_BLOCKED" rejection is recorded.
2. **Given** a headline arrives but the 5-minute volume around it is below the confirmation threshold, **When** evaluation runs, **Then** no signal is issued and a "NO_VOLUME" rejection is recorded.
3. **Given** a headline arrives and price is on the correct side of the trend filter with volume expansion, **When** evaluation runs, **Then** exactly one signal is issued for that symbol.

---

### User Story 2 - Exits match measurable market behavior (Priority: P1)

An operator reviews a closed signal and sees a stop placed at a distance derived from the
asset's current volatility (not a fixed percentage), a first take-profit target that is actually
reachable within the holding horizon (partial exit), an automatic move to break-even after the
first target, and an active time-decay rule that closes or protects a trade that never moved
within its first half-hour — instead of riding a loser to the clock.

**Why this priority**: Today the take-profit sits ~4 standard deviations away (0% hit rate in 33
trades) and the stop sits inside normal hourly noise (27% stop-out rate at −8.6% each). These two
numbers alone make expectancy negative. This is the highest-leverage fix after entries.

**Independent Test**: Simulate the new exit rules over the recorded 33-signal history and the
last 30 days of candles: take-profit must be reachable (target hit rate in the empirically
expected band), and the simulated expectancy must be strictly greater than the current −1.36%
per trade.

**Acceptance Scenarios**:

1. **Given** a new signal, **When** stop and targets are calculated, **Then** distances are expressed as multiples of the asset's measured true range and fall inside the asset's observed hourly movement distribution.
2. **Given** an open signal that reaches its first target, **When** the target fills, **Then** a partial close occurs and the remaining position's stop moves to break-even plus fees.
3. **Given** an open signal whose unrealized result is still below +0.5R at minute 30, **When** the decay check runs, **Then** the stop moves to break-even; if still unprofitable at minute 40, the position closes.
4. **Given** any closed signal, **When** the outcome is displayed, **Then** the recorded return equals the computed return from recorded entry and exit prices — never a placeholder zero.

---

### User Story 3 - News decisions use the whole story, not one headline (Priority: P2)

An operator sees that a single event reported by several outlets has been clustered into one
catalyst: the signal shows how many sources reported it, how fresh it is, and how consistent the
coverage is. If coverage is contradictory (mixed bullish and bearish within the window), the
system vetoes trading instead of picking a side. Syndicated copies of one story can no longer
open multiple overlapping positions.

**Why this priority**: Today one "Altcoins rally" headline syndicated across feeds produced six
consecutive long entries that all stopped out. News quality gates direction quality; P2 because
entries already carry a technical gate from Story 1 (defense in depth), but the repeated-entry
failure is expensive enough to rank above cosmetics.

**Independent Test**: Feed five near-identical headlines of one event and one contradictory
headline set: exactly one catalyst event is formed for the first set (with story count 5), and
zero trades for the polarized set.

**Acceptance Scenarios**:

1. **Given** multiple outlets publish the same event within the clustering window, **When** headlines are processed, **Then** one catalyst event exists with an accurate source count and only one position may open per symbol per event.
2. **Given** a headline older than the freshness limit, **When** a signal is evaluated, **Then** its weight has decayed below the tradeable threshold and it cannot alone justify a trade.
3. **Given** bullish and bearish headlines for the same asset within the window are nearly balanced, **When** evaluation runs, **Then** the system vetoes the trade with a "POLARIZED" reason.

---

### User Story 4 - Commodities get their own trading section (Priority: P2)

An operator opens a dedicated Commodities view (gold, silver, oil, copper, gas, …) with its own
signal list, longer time horizon, and status around market sessions and scheduled reports. A
geopolitical or inventory event produces a commodity signal with a multi-hour lifetime, and the
operator can see at a glance that no commodity position will be opened into a scheduled-report
blackout or held over a market closure. Commodity performance is reported separately from crypto.

**Why this priority**: 45% of equity is already reserved for commodities, yet zero of 38 signals
were commodity signals — capital is allocated to a channel that never fires. Value is unlocking
existing capital and serving the operator's stated need for a separate, longer-horizon section.

**Independent Test**: With a gold-catalyst headline and green session state, a commodity signal
appears in the Commodities section with a multi-hour expiry; during a report blackout or closed
session, no commodity signal can be created; crypto signals are unaffected.

**Acceptance Scenarios**:

1. **Given** a gold-specific catalyst during an open market session, **When** evaluation runs, **Then** a commodity signal is created with the commodity horizon (not the crypto 1-hour horizon) and appears in the Commodities section.
2. **Given** the clock is inside a scheduled-report blackout window, **When** a matching catalyst arrives, **Then** no signal is created and the event is logged as "EVENT_BLACKOUT".
3. **Given** an open commodity position approaching a market closure, **When** the pre-close deadline arrives, **Then** the position is closed flat before the closure.
4. **Given** commodity and crypto histories, **When** performance is viewed, **Then** each asset class shows its own win rate, payoff, and expectancy.

---

### User Story 5 - Honest, complete performance reporting (Priority: P1)

An operator reviews trade history and sees every closed trade with a real return value: winners
show positive percentages, losers negative, and none show "+0.00% ($0.00)". A summary panel
shows win rate, average win, average loss, and expectancy per asset class so the operator can
judge whether the system has positive edge.

**Why this priority**: The reporting bug disguised the book's true state (16 of 33 trades were
favorable but displayed as zero) and is a prerequisite for trusting every other measurement in
Stories 1–4. Ships with Story 1 but tracked separately because it also retroactively corrects
existing records.

**Independent Test**: Query all closed signals: zero records have a zero return when entry and
exit prices differ; the summary numbers reconcile with per-trade rows.

**Acceptance Scenarios**:

1. **Given** a closed signal whose exit price differs from entry, **When** the history renders, **Then** the displayed percentage equals (exit/entry − 1) × leverage × 100 with correct sign for long and short.
2. **Given** historically closed signals that were recorded with zero returns, **When** migration runs, **Then** their returns are recomputed from recorded prices and marked as recomputed.
3. **Given** the closed-signal summary, **When** displayed, **Then** win rate, average win, average loss, and expectancy are shown per asset class.

---

### User Story 6 - Alert prices stay readable for micro-cap assets (Priority: P1)

An operator receives a Telegram entry alert for a low-priced token and sees three clearly
different prices for entry, stop loss, and take profit. Previously the fixed two-decimal format
collapsed prices like 0.000002334544 into "0.00", so entry, stop, and target looked identical
and the alert carried no actionable information.

**Why this priority**: The alert is the operator's execution surface. A degenerate price string
directly causes wrong fills or distrust of every other number in the pipeline; it is a small,
self-contained P1 fix.

**Independent Test**: Format an entry and a resolution alert for a signal whose prices differ
below the 1.0 threshold: every price in the message is non-degenerate and mutually distinct.

**Acceptance Scenarios**:

1. **Given** a signal with entry 0.000002334544, stop 0.0000022862, and target 0.000002684726, **When** the entry alert renders, **Then** three distinct price strings appear and none is "0.00".
2. **Given** a BTC-scale price, **When** the alert renders, **Then** the price keeps a familiar two-decimal form.
3. **Given** a closed trade exit price, **When** the resolution alert renders, **Then** entry and exit prices use the same adaptive precision and remain distinguishable.

---

### User Story 7 - Indicator weights learn from real trade outcomes (Priority: P2)

An operator expects the system to get smarter: indicators that were bold (directionally
aligned) in decisions that later won should gain weight; those aligned with losing trades
should lose weight. Today the learning loop receives empty decision-time indicator values, so
most indicator statistics never update and a few update incorrectly (microstructure updates on
every trade regardless of direction). This story records the decision-time indicator
measurements with each signal and makes the outcome attribution direction-aware and
unknown-safe, and makes the manual weight controls in the UI actually persist.

**Why this priority**: Weight learning is the compounding edge of the system, but it does not
change which trades fire today; P2 behind the entry/exit/reporting fixes.

**Independent Test**: Close trades with recorded snapshots across wins and losses: only the
indicators that were directionally aligned at entry update their statistics; trades without a
recorded snapshot update nothing; manual slider saves change the served weights.

**Acceptance Scenarios**:

1. **Given** a long signal recorded with SuperTrend BULL, positive MACD histogram, and RSI above 50, **When** the trade closes as a win, **Then** the aligned indicators' statistics improve and the unaligned ones do not change.
2. **Given** a trade whose recorded snapshot is missing (legacy rows), **When** it closes, **Then** no indicator statistic changes.
3. **Given** a losing trade whose recorded snapshot shows microstructure flow against the entry, **When** the outcome is recorded, **Then** microstructure statistics update only for trades whose flow agreed with the entry direction.
4. **Given** the operator moves a weight slider and saves, **When** the next decision is evaluated, **Then** the saved weights are the ones served to evaluation.

---

### Edge Cases

- Catalyst arrives while the market feed has no fresh price → entry must be deferred, not entered at a stale price.
- Volatility measurement unavailable (e.g., first candles of a new listing) → fall back to the configured minimum stop distance, flag the signal as "FALLBACK_PARAMS".
- Stop distance would exceed the maximum allowed loss budget → position size shrinks; if even minimum size exceeds budget, the signal is not issued.
- Leverage needed for target risk exceeds the liquidation-buffer safety floor → leverage capped and size adjusted accordingly.
- Headline clusters span the symbol boundary (e.g., BTC + ETH same event) → each symbol gets at most one position; shared event id recorded.
- Commodity signal is open when an unscheduled geopolitical shock closes markets → position closes at the pre-close deadline rule.
- Time-exit fires while price feed is momentarily unavailable → exit retried with last confirmed price; never persisted as zero.
- Recomputation migration runs twice → idempotent; already-recomputed records are skipped.
- Micro-cap price below 1.0 formatted with fixed 2 decimals → all alert prices collapse to the same string → adaptive precision required (FR-021).
- Trade closes with no recorded indicator snapshot (legacy rows) → attribution is skipped entirely; no statistic moves on unknown data (FR-023).
- Indicator measurement exists but was neutral at entry (e.g. flat histogram) → no update for that indicator; neutrality is not evidence.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST gate every entry on technical confirmation: trend/session-average alignment, minimum volume expansion around the catalyst, and an anti-chase rule that blocks entries when price is in the extreme upper band of its recent range.
- **FR-002**: System MUST derive stop-loss distance from the asset's measured true range with a structure offset behind the recent swing, and MUST NOT use a single fixed percentage across all assets.
- **FR-003**: System MUST define take-profit targets from the empirical distribution of the asset's favorable movement over the holding horizon; the first target must be reachable with historical hit probability consistent with a winning book (target band: 40–80%).
- **FR-004**: System MUST support a two-stage exit: partial close at the first target, automatic move of the remaining position's stop to break-even plus fees.
- **FR-005**: System MUST apply active time-decay exits: protect (breakeven) a trade that has not reached +0.5R by the decay checkpoint, and close a still-unprofitable trade at the hard decay checkpoint, both before the full holding horizon ends.
- **FR-006**: System MUST set leverage per asset from a target-volatility formula (capped at the configured maximum) and MUST verify that distance-to-liquidation divided by stop distance is at or above the configured safety floor before issuing a signal.
- **FR-007**: System MUST size each position by fixed-fractional risk (configurable equity percentage per trade) divided by the actual stop distance, bounded by per-tier capital budgets; sizing MUST NOT depend on fixed dollar slots alone.
- **FR-008**: System MUST cluster near-identical headlines within a rolling window into a single catalyst event with a source count, and MUST allow at most one open position per symbol per catalyst event.
- **FR-009**: System MUST weight catalyst freshness with an exponential half-life decay appropriate to the holding horizon and MUST discard events older than the configured maximum age.
- **FR-010**: System MUST assign source-authority weights (primary filings/newswires highest, aggregators lowest) when aggregating an event's score.
- **FR-011**: System MUST veto trading when bullish and bearish coverage of the same asset within the window is near-balanced (polarization above threshold), recording the veto reason.
- **FR-012**: System MUST store fused news sentiment and model confidence as separate values; confidence MUST NOT be presented as news sentiment.
- **FR-013**: Diagnose why Core commodity symbols produce no signals despite registered symbols and reserved capital, and fix the root cause so eligible commodity catalysts produce signals.
- **FR-014**: System MUST run commodity signals on a commodity profile: longer holding horizon than crypto, wider volatility-based stops, session-open/close and scheduled-report blackout windows, and a mandatory flat-before-closure rule.
- **FR-015**: System MUST provide a dedicated Commodities view: separate signal list, horizon and blackout status, and asset-class-separated performance.
- **FR-016**: System MUST persist every closed trade's return and profit computed from recorded entry/exit prices and leverage; placeholder or zero values are permitted only when prices are genuinely identical.
- **FR-017**: System MUST recompute historically zeroed returns from recorded prices in a one-time, idempotent correction and mark corrected records.
- **FR-018**: System MUST expose per-asset-class and aggregate performance summaries: win rate, average win, average loss, payoff ratio, expectancy.
- **FR-019**: All parameter formulas (stop, target, leverage, size, decay checkpoints) MUST be configuration-driven with documented defaults, so operators can retune without code changes.
- **FR-020**: Every rejected or vetoed entry MUST be logged with a machine-readable reason for audit and for measuring filter quality.
- **FR-021**: System MUST render every price in outgoing alerts (entry, stop loss, take profit, exit) with precision adaptive to the price magnitude, so prices below 1.0 keep enough significant digits to stay non-degenerate and mutually distinct; prices at or above scale keep conventional formatting.
- **FR-022**: System MUST persist the decision-time indicator measurements (trend, momentum, money-flow, efficiency, microstructure) that produced each signal, and MUST attribute a closed trade's outcome to indicator weights using those recorded measurements only.
- **FR-023**: System MUST update an indicator's weight statistic only when its recorded measurement exists and directionally agreed with the entry; unknown or missing measurements MUST NOT change any statistic; microstructure updates MUST be direction-aware instead of unconditional.
- **FR-024**: Manual weight controls exposed by the UI MUST persist their changes to the weights served to evaluation, or be presented as read-only; silent local-only edits are not allowed.

### Key Entities

- **Catalyst Event**: clustered news event — headline set, source list and authority weights, fused sentiment, freshness age, polarization index, symbols affected, story count.
- **Signal**: proposed or open trade — symbol, direction, entry, stop, staged targets, leverage, risk budget, sizing inputs, horizon, profile (crypto/commodity), status, rejection/veto reasons, decision-time indicator snapshot (trend, momentum, money-flow, efficiency, microstructure).
- **Indicator weight statistic**: per-indicator learning state derived from recorded trade outcomes; source of the weights served to evaluation.
- **Risk Profile**: per-asset-class parameter set — horizon, volatility multipliers, decay checkpoints, leverage cap, liquidation-buffer floor, risk-per-trade, blackout/session calendar.
- **Trade Outcome**: closed result — recorded prices, computed return and profit, exit reason, recomputed flag, catalyst event link.
- **Performance Summary**: derived view — per asset class and aggregate win rate, average win/loss, payoff, expectancy.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero closed trades display a zero return when entry and exit prices differ (100% of records correct after migration).
- **SC-002**: Take-profit first target is reached in 40–80% of winning signals within the holding horizon (vs 0% today).
- **SC-003**: Stop-out rate falls below 20% of closed signals (vs 27% today) with average loss per stop smaller than the average win — payoff ratio ≥ 0.7.
- **SC-004**: Simulated expectancy over the trailing 30 days of signals is positive and strictly better than the current −1.36% per trade.
- **SC-005**: One syndicated story yields at most one open position per symbol; contradictory coverage yields zero positions in 100% of test cases.
- **SC-006**: At least one commodity signal is produced per week when a qualifying commodity catalyst occurs (baseline: 0 in 38 signals); no commodity position is ever held across a market closure; zero entries occur inside blackout windows.
- **SC-007**: Operator can view crypto and commodity performance separately, each with win rate, payoff, and expectancy, on one screen.
- **SC-008**: Every rejected entry is auditable with a reason; filter rejection rate is measurable per rule.
- **SC-009**: For any alert whose underlying prices differ, no displayed price is a degenerate rounding (e.g. "0.00") and all displayed prices in that alert are mutually distinct.
- **SC-010**: In a test batch of recorded win/loss trades: every directionally aligned indicator of a winning trade gains weight mass, every directionally aligned indicator of a losing trade loses weight mass, and trades with a missing indicator snapshot change no weight.
- **SC-011**: A manual weight edit, once saved, is observable in the weights the evaluation path serves — no divergence between the UI controls and the served weights.

## Assumptions

- Existing AI direction call remains the catalyst-direction source; this feature gates and sizes around it rather than replacing it.
- Parameter defaults come from `docs/RESEARCH-trading-system-optimization.md` (ATR multiples, half-life, polarization threshold, session windows); exact numbers are tunable per FR-019.
- Backtest/simulation validation uses recorded exchange candles for the trailing 30 days; no new market-data vendor is introduced.
- Historical zero-return correction is one-time and idempotent; already-corrected and genuinely-flat trades (entry == exit) are left untouched.
- Forex remains out of scope (four integration blockers documented in the research file).
- Commodities trade the existing registered commodity symbols; new symbols are out of scope.
- Risk-per-trade default stays at the current 1.5% of equity unless re-tuned via configuration.
- "Market closure" for commodities follows each instrument's published session calendar; unscheduled closures are handled by the pre-close deadline rule only.
