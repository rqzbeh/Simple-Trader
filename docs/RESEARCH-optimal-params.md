# Research: evidence-based base values for required .env keys (2026-09-29)

**Question** (user): which values are optimal for our goal, grounded in finance/trading/econ science, so the required keys have a scientific base.

**Method**: literature review (Kelly/growth-optimal, drawdown recovery, portfolio concentration, event studies, market microstructure) + exchange fee schedules. Source links per row.

## Recommendations

| Key | Current sample | Recommended base | Evidence |
|---|---|---|---|
| MAX_RISK_PER_TRADE_PCT | 0.02 | **0.01–0.015** | 5-loss run at 2% ⇒ 9.6% DD trips the 8% breaker; prop-desk norm 0.5–1.5% [Thorp 2008](https://rybn.org/halloffame/PDFS/2008_Understanding_Kelly_New.pdf) |
| MIN_RISK_PER_TRADE_PCT | 0.005 | 0.0025–0.005 | Kelly floor vs min-notional quantization (Binance) |
| KELLY_FRACTION | 0.5 | 0.33–0.5 (keep 0.5) | Half-Kelly = 75% of max growth, 50% less variance; overestimating p by 5% makes full Kelly negative-EV [MacLean/Thorp/Ziemba 2011](https://books.google.com/books/about/The_Kelly_Capital_Growth_Investment_Crit.html) |
| MAX_DRAWDOWN_LIMIT_PCT | 0.08 | 0.08–0.10 (keep) | Recovery non-linearity: −10% ⇒ +11.1%, −20% ⇒ +25%, −50% ⇒ +100% [Grossman & Zhou 1993](https://onlinelibrary.wiley.com/doi/abs/10.1111/j.1467-9965.1993.tb00044.x) |
| MAX_CONCURRENT_SIGNALS | 5 | **3** | crypto cross-ρ≈0.75 ⇒ N_eff = 5/(1+4·0.75) = **1.25 effective bets** [Meucci 2009](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=1358533) |
| MIN_STOP_LOSS_PCT | 0.6 | **1.0–1.2** | 1h ATR: BTC 0.8–1.2%, alts 1.5–3.5% — 0.6% sits inside noise ⇒ whipsaw stops [Kaminski & Lo 2014](https://dspace.mit.edu/entities/publication/bb69ca4b-0cdc-487f-831d-63b2e84fafee) |
| MAX_STOP_LOSS_PCT | 2.5 | 3.5–4.5 | 1.5–2.0× ATR on 1h/4h swings (Wilder) |
| MIN_TAKE_PROFIT_PCT | 1.5 | 2.0–2.5 | target must clear round-trip friction ≥10× (Almgren-Chriss) |
| MIN_RISK_TO_REWARD_RATIO | 2.5 | **1.8–2.0** | breakeven p = 1/(1+R): R2.5⇒28.6%, R2.0⇒33.3%; rigid 2.5 forces stretched targets = signal starvation (Van Tharp expectancy math) |
| MAKER/TAKER_FEE_RATE | 0.0002/0.0005 | keep | exact 2026 VIP0 Binance/Kraken perp schedule |
| MAX_SLIPPAGE_PCT | 0.05 | **0.005** | 5% cap = 500bps on $50M pairs where normal slippage is 1–5bps; a 5% slip on a 2% stop = instant 7% loss (Almgren-Chriss) |
| MAX_TRADE_MARGIN_PCT | 0.20 | 0.10–0.15 | 3–4 concurrent × 20% ⇒ >50% margin at risk; ESMA-style prudence |
| CALENDAR_HALT_MINUTES | 15 | 15–30 (keep 15+) | NFP/CPI/FOMC: spreads ×3–10 and vol clusters 15–30 min post-release [Andersen et al. 2003](https://public.econ.duke.edu/~boller/Published_Papers/aer_03.pdf) |
| SCREENER_MIN_24H_VOLUME | 50M | keep | $50M ≈ $35k/min ⇒ $10–50k clips <2bps impact (Amihud) |
| SCREENER_MAX_SPREAD_BPS | 10 | 6–8 | majors trade 1–3bps; >8bps eats edge (Chordia et al. 2005) |
| SIGNAL_MAX_AGE_MINUTES | 60 | **30–45** | news/order-flow alpha decays to efficiency in 15–30 min [Chordia/Roll/Subrahmanyam 2005](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=282408) |
| CORE_TARGET_PCT / ALPHA_TARGET_PCT | 0.50 / 0.40 | 0.50 / **0.40** | 0.5+0.5 leaves no cash reserve; 10% unallocated survives margin events (core-satellite, Amenc et al. 2004) |
| INITIAL_CAPITAL | 10000 | user-specific | $10k ⇒ $50 risk at 0.5% — above min-notional quantization |

## Top 5 changes worth making
1. `MAX_SLIPPAGE_PCT` 0.05 → **0.005** (5% cap is dangerous)
2. `MIN_RISK_TO_REWARD_RATIO` 2.5 → **1.8–2.0** (starves signals; NOTE: currently core-managed by Jev — applies to override when user sets one)
3. `MIN_STOP_LOSS_PCT` 0.6 → **1.0–1.2** (inside noise today)
4. `MAX_CONCURRENT_SIGNALS` 5 → **3** (correlation ⇒ 1.25 effective bets at 5)
5. `ALPHA_TARGET_PCT` → **0.40** with CORE 0.50 (keep 10% cash buffer)

Applied where: values live in `.env` (spec-017) — adopting a recommendation = edit `.env` / `.env.example` sample, never code.
