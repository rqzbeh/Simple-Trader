# Research: Signal System Optimization (TA, News Fusion, Commodities)

Date: 2026-09-25 · Status: research only, no code changes
Method: codebase trace (file:line) + external primary research (sources per claim)
Context: 33 closed signals → 9 STOP_LOSS (−8.6% avg), 23 TIME_EXIT (bugged, now backfilled to +1.64% avg), 1 DEDUP. TP never hit. True expectancy ≈ −1.36%/trade.

---

## Part 1 — How the system actually works (verified in code)

### 1.1 News pipeline

| Question | Answer | Evidence |
|---|---|---|
| Sources | 8 RSS feeds: YahooFinance, CoinDesk, CoinTelegraph, Decrypt, WhaleAlerts, on-chain researchers, politician trades, Trump-crypto. Poll 60s. | `internal/market/news_crawler.go:46-55,76` |
| Retention | 100 newest in memory (bootstraps 100 from `news_articles` on start); full history in `news_articles`. `news_items` is dead legacy table. | `news_crawler.go:97-115,176-189` |
| Headlines sent to AI | **All matches from the 15 newest articles** — NOT just first sentiment match. | `internal/server/signal_handlers.go:176-181,516-520` |
| Match rule | `HeadlinesForSymbol`: base ticker/asset name, OR 18 macro keywords ("war", "rate cut", "tariff"…), OR broad crypto terms / exposure group ("gold","oil"). No sentiment filter at selection (bull+bear both kept). | `internal/market/news_sentiment.go:112-173` |
| To the AI | Joined bullet list of ALL matched headlines + precomputed lexicon sentiment + 10 indicator values, one symbol per call. | `internal/ai/client.go:140-189`, `internal/trader/signals.go:160-186` |
| catalyst_sentiment column | NOT lexicon: it is the AI's **confidence** signed by direction (fallback ±0.6). Explains all the ±0.7 values. | `internal/trader/signals.go:324-344` |
| Cross-coin reasoning | None by AI (one symbol per request). Correlation gates applied in Go before the call. | `internal/ai/types.go:34-42`, `signal_handlers.go:322-342` |

**Direct answer to "does AI read all news or just the first matching one":** it reads *all keyword-matched headlines within the newest 15 articles* in one prompt. Failure modes are different: (a) 15-item newest window can miss the 3rd story that changes context, (b) keyword matching can flood the prompt with one repeated story (8 feeds syndicate), (c) no clustering/dedup/decay — a 3-hour-old headline weighs the same as a 1-minute-old one.

### 1.2 Parameters: entry / SL / TP / leverage / allocation

| Parameter | Actual logic | Evidence |
|---|---|---|
| Entry | **Market order at current ticker price when AI says BUY/SELL. No TA timing, no pullback/volume/OI confirmation.** | `trader/signals.go:215,345`, `signal_handlers.go:396` |
| Stop loss | `NATR(14) × 1.5`, clamped [0.6%, 2.5%]. Observed ≈1.1% → −8.6% ROE at 8x. Vol-scaled exists but multiplier is fixed. | `trader/signals.go:216-229,49-50` |
| Take profit | AI suggestion clamped [1.5%, 8%], then **R:R floor 2.5 forces TP = 2.5 × SL distance ≈ +2.75% price**. 1h empirical 90th-pct move is 0.6–1.1% → TP mathematically unreachable. TP2 column unused (NULL). | `trader/signals.go:230-270`, `ai/types.go:53` |
| Leverage | AI-suggested but any value outside [1, 8] is reset to 8 → effectively fixed 8x for every asset. | `trader/signals.go:273-276` |
| Capital allocation | Sound: fixed-fractional 1.5% equity risk → quantity = risk / SL-distance → margin clamped to 20% equity and to tier budget. Observed $8,000/trade = tier-slot budget. | `trader/futures_math.go:133-171`, `trader/signals.go:286-312`, `signal_handlers.go:256-263` |
| Time exit | 60-min hard close; active stop is level-triggered each reconcile tick. (Was writing ROI=0 — fixed in ac29b2c.) | `signal_handlers.go:780-790` |

### 1.3 Technical analysis inventory (already built — but bypassed at entry)

RSI-14, MACD, SuperTrend, Bollinger, **ATR/NATR**, Garman-Klass, Parkinson, Kaufman ER, CMF, VWAP, OBI, CVD, divergence, market regime (ATR ratio), confluence score, Thompson-Sampling dynamic weights injected into the prompt. `internal/indicators/*`.
**Gap: none of this gates the entry or sizes the trade — it is prompt telemetry only.** No open-interest data anywhere.

### 1.4 Commodities status

- **10 commodity symbols already registered** ("CORE" tier): PAXG/XAU/XAG gold-silver, COPPER, XPT, XPD, OIL (WTI), BRENT, ALU, NG. Screener exempts them from crypto volume/spread limits. `internal/market/assets.go:21-139`, `screener.go:104-118`
- Allocator already reserves **45% of equity to Tier-2 Core Commodities** and a MacroRegimeEngine adjusts tiers on geopolitical/inflation stress. `trader/allocator.go:24-94`
- **Yet 0 of 38 signals were commodities.** Open question for spec: why CORE generates no signals (news match path? scanUniverse order? slot budget? AI HOLD?). Candidate: RSS feeds are crypto-centric; `GetExposureGroup` needs explicit "gold"/"oil" words in a headline.
- Crypto TTL (60 min) and parameters are applied uniformly — commodities need longer horizon per Part 2.

---

## Part 2 — External research: what right looks like

### 2.1 Entry: catalyst + technical confirmation
- Never market-buy raw headline sentiment. Require confluence: direction aligned with 1h/4h 20-EMA and session VWAP; 5m catalyst-candle volume > 2.5× 20-period SMA; OI-delta confirmation (price↑+OI↑ = real flow; price↑+OI↓ = squeeze, do not buy).
- Anti-chasing: if price > 2σ above 15m VWAP at signal time → no market entry, limit at 5m EMA(9).
- Event tiers: Tier-1 (regulatory/listing/macro) weight 1.0, trend 2–6h; Tier-2 weight 0.5, 30–90m; Tier-3 (rumors/influencers) discard.
- Sources: tradingview.com/scripts/vwap, changelly.com/crypto-volume-indicators, arxiv.org/html/2606.12210

### 2.2 Stop loss: volatility + structure
- Ban fixed %. SL = 1.8–2.2× 1h ATR (BTC/ETH ATR ≈0.45–0.75% → SL 0.9–1.5%; high-beta alts ATR ≈0.8–1.2% → SL 1.6–2.4%), placed behind 15m swing ± 0.25×ATR buffer, never at round numbers.
- Trigger on 5m close or mark-price to ignore wick-outs.
- Time-decay exit: news momentum half-life 15–25 min. If PnL < +0.5R at minute 30 → breakeven; if < 0 at minute 40 → close. Do not ride a dead trade to the 60m bell.
- Sources: trendrider.net/stop-loss-strategies-crypto, chartscout.io/stop-loss-crypto, backtestbrewery.github.io/alpha-decay-trading

### 2.3 Take profit: distribution-grounded
- Empirical 1h MFE (Binance): BTC median 0.18% / p90 0.63%; ETH 0.25% / 0.84%; SOL 0.34% / 1.08%. A +2.5–3.2% TP is a 4-sigma event in 60 minutes — explains 0% hits.
- TP1 = 1.0× 1h ATR (≈+0.5–0.7% → +4–5.6% ROE @8x), close 60%; move SL to entry+fees; runner = TP2 at 1.8× ATR or trail 5m EMA(9).
- Expectancy with TP1 +5% ROE, SL −6% ROE, 60% win rate: +0.6%/trade.
- Sources: journalplus.co/take-profit-calculator, scheller.gatech.edu (crypto variance, Lee-Wang 2024), stephenperrenod.substack.com

### 2.4 Leverage: volatility-targeted
- `lev = min(8, target_hourly_vol 3.5% / asset_1h_ATR%)` → BTC/ETH 7–8x, SOL-class 2.5–3x.
- Liquidation buffer rule: (1/lev − MMR) / SL_distance ≥ 4.0.
- Sources: mudrex.com/learn/volatility-crypto-futures, grandalgo.com/liquidation-calculator, tradefundrr.com/liquidation-price

### 2.5 Allocation: risk-based sizing (mostly already implemented)
- Keep fixed-fractional: risk 1.0% (cap 1.5%) of equity per trade; notional = risk / (SL% + 0.1% slippage buffer); margin = notional / leverage.
- Fractional Kelly (¼ Kelly) as upper cap only — crypto kurtosis > 15 punishes full Kelly. Example: p=0.55, b=1.2 → ¼Kelly ≈ 4.4% max capital.
- Sources: journalplus.co/kelly-criterion-guide, kraken.com/futures-position-sizing, altrady.com/kelly-criterion-crypto

### 2.6 News fusion: what to add
- **Cluster, don't first-match**: embedding/MinHash similarity > 0.82 within 45 min → one event, increment `story_count`.
- Source weights: Tier-1 (filings/Reuters/Bloomberg) 1.0, Tier-2 (CoinDesk/The Block) 0.6, Tier-3 (scrapers/Telegram bots) 0.2.
- Freshness decay: exp(−ln2·Δt/15min) for 1h horizon; discard > 45 min old.
- **Polarization veto**: P = 1 − |Σpos − Σneg| / |Σpos + Σneg|; if P > 0.40 in 15-min window → no trade.
- Scope split: macro news adjusts portfolio beta/risk multiplier only; asset-specific news trades the single symbol (veto if BTC 15m correlation opposes).
- Sources: arxiv.org/html/2603.23568v1, pmc.ncbi.nlm.nih.gov/articles/PMC8157256, feedly.com/engineering/clustering-latency

### 2.7 Commodities: separate engine
- Sessions: core liquid window 08:00–11:30 ET; blackout ±15 min around daily settlement 17:00–18:00 ET; **flat by Friday 16:30 ET — never hold weekend gaps**.
- Event blackouts: CL ±15 min around EIA (Wed 10:30 ET); GC/ES ±30 min around NFP/CPI/FOMC.
- Different signal shape: TTL 4–12h (vs 1h crypto), SL 2.5–3.0× ATR (opening-bell expansion), spread guard: abort if bid-ask > 2× median.
- Fundamentals dominate headlines: real yields for gold, inventories/OPEC for oil, war-risk premium as regime overlay (allocator's MacroRegimeEngine already models geopolitical stress).
- Sources: damnpropfirms.com/futures-trading-hours, newyorkcityservers.com/futures-trading-hours, investing.com/economic-calendar

---

## Part 3 — Gap → proposed work (for spec-kit, not yet approved)

| # | Gap (code evidence) | Proposed change |
|---|---|---|
| G1 | Entry is blind market order (`signals.go:215`) | TA gate: VWAP/EMA trend filter + 5m volume spike + anti-chase limit entry |
| G2 | R:R floor 2.5 forces unreachable TP (`signals.go:260-270`) | TP1 = 1.0×ATR partial 60% → breakeven → runner; drop hard R:R floor for 1h horizon |
| G3 | SL multiplier fixed, no structure (`signals.go:220`) | SL = 2.0×ATR behind 15m swing, 5m-close trigger; time-decay (BE @30m, flat @40m) |
| G4 | Leverage ≡ 8 for all (`signals.go:273-276`) | Vol-targeted leverage; liquidation-buffer check ≥4× |
| G5 | News: 15-headline window, no dedup/decay (§1.1) | Clustering + source weights + 15-min half-life + polarization veto; widen window |
| G6 | catalyst_sentiment = AI confidence (§1.1) | Store true fused sentiment separately; keep confidence separate |
| G7 | Commodities: registered, funded (45% tier), but **0 signals in 38** (§1.4) | Diagnose CORE path; separate commodity section: 4–12h TTL, session/event blackouts, weekend-flat rule, own UI section |
| G8 | One headline repeated across 8 feeds → multi-entries (observed 6× LONG on one story) | story_count dedup → one position per event per symbol |

**Open questions for spec clarification:** G7 root cause (needs a debug pass on CORE signal generation); commodities UI = new page vs tab; whether forex is in scope (4 blockers listed in §1.4 source trace).
