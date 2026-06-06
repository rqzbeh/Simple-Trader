# Secret Formula — Internal Investment System

**Simple-Trader** (internally called the "Secret Formula") is a production-grade, 24/7 autonomous portfolio management backend for an investment company.

It maintains a disciplined **Core** allocation (~55% target risk) in Gold and Silver for capital preservation, inflation hedging, and risk minimization, while running an **Alpha** book (~45%) in Crypto, Forex, and Oil that aggressively seeks short-term "massive profits" from news, on-chain flows, and public high-impact sentiment catalysts — especially **mandatory politician trading disclosures**.

The system now also includes **full dedicated support for the Iranian market** (Tehran Stock Exchange / Iran Fara Bourse) via a completely separate module for news, algorithms, and data sources. This is because the Iranian market is largely decoupled from global trade — it can grow or behave differently despite wars, new sanctions, or global turmoil, driven by local dynamics, Codal disclosures, and a distinct economic "world view".

The system is deliberately built to run mostly on **free public data sources** and continuously learns from every outcome using real machine learning (regret tracking, cause attribution, weight persistence, auto-retraining, and veto logic). It is a **hybrid**: real AI/LLM API keys are fully supported for news analysis when available, with a powerful knowledge-driven heuristic fallback for zero-key operation.

> **Philosophy**: Buffett-level capital protection + Jim Simons-level statistical edge from public data + relentless improvement from mistakes.

---

## Core Philosophy

- **Core Bucket (Gold/Silver)**: Preservation first. Gold acts as the ultimate moat and inflation/risk-off hedge. Position sizing increases when value is high (dovish policy, risk-off headlines).
- **Alpha Bucket (Crypto / Forex / Oil + Iranian Stocks/ETFs)**: High-conviction, short-horizon (2h–24h) ideas driven by news flow, whale on-chain moves, and **public politician disclosures** (or local equivalents).
- **Iranian Market (Dedicated & Separated)**: Stocks, ETFs, bonds, fixed income funds, and Islamic Treasury Bonds. Uses its own news sources (Eghtesad News, Codal.ir filings), algorithms, and data paths because the market often diverges from global ones. Codal acts as the primary "disclosure" source for high-signal local events.
- **Public Edge Sources** (all free):
  - Politician disclosures (STOCK Act PTRs, congressional filings).
  - On-chain whale activity (public blockchain explorers).
  - High-quality public RSS (macro, energy, crypto policy).
  - **Iranian-specific (separated)**: Eghtesad News RSS + Codal.ir reports (company financials, major events, "local insider-like" alpha). Supports Persian + English keywords.
- **Risk Architecture**: Hard bucket targets + per-class caps + daily loss / drawdown circuit breakers + automatic Gold/Silver hedging overlay.
- **Learning System**: Every loss is attributed to explicit causes from the decision audit. Causes become persistent negative weights. Repeated mistakes trigger regret vetoes. Human review tags drive auto-retraining.

---

## Key Features

- **PortfolioAllocator** — Computes desired risk for Core vs Alpha with concentration guards and rebalance suggestions (now includes Iranian asset classes).
- **RiskEngine** — Pre-trade checks, circuit breakers, daily loss limits, max drawdown pause.
- **HedgeManager** — Automatically recommends increasing Gold/Silver when Alpha risk is high or risk-off regime is detected.
- **Public Politician Disclosures** — Dedicated free fetcher for STOCK Act / PTR signals.
- **Dedicated Iranian Market Module** (`iran.py`) — Fully separated:
  - News input (Eghtesad + Codal-focused processor with local "different world view").
  - Algorithms (Codal event strength, sanctions-resilience scoring, Iran oil beta — market can grow despite global bad news).
  - Chart/data sources (hooks for tsetmc.com, ifb.ir, Codal public data; graceful news-driven fallback).
  - Supports: Iranian stocks, ETFs, bonds, fixed income funds, Islamic Treasury Bonds (اوراق خزانه اسلامی).
- **Advanced ML Loop** (real & auditable):
  - `regret_table` + `cause_weights` persisted in DB.
  - Cause attribution from `decision_audit` + outcome + knowledge rules (e.g. `insufficient_hedge`, `ignored_politician_bearish_disclosure`, Iran-specific resilience factors).
  - `CauseWeightPersister` applies penalties inside the scorer.
  - Regret veto in the signal creation gate.
  - Auto-retrain from human judgment tags (`review-mistakes --tag "ignored_hedge,low_conf"`).
  - Enriched scorer features (whale flag, politics flag, Buffett value proxy, technical indicators + meta-indicators, regret frequency, and Iran-specific features).
- **Expanded Indicators Layer** (`indicators.py`): RSI, MACD, Bollinger, enhanced ATR + regime, volume metrics, confluence score + public meta-indicators. Regime-adaptive. Indicator-aware sizing. Cached for performance.
- **Hybrid LLM + Heuristic Analysis**:
  - **Full support for AI API keys** (Groq, OpenAI-compatible, Gemini, etc.). When keys are configured, news (including Iranian items) is analyzed by the weighted LLM pool with rate limits, provider fallbacks, and structured prompts enriched by the knowledge base.
  - Automatic graceful fallback to powerful `HeuristicAnalyzer` (knowledge-driven, no keys needed) when no API keys are present or all providers fail.
  - Iranian news benefits from both: LLM analysis when available + dedicated Iran algorithms in heuristic.
- **Beautiful Internal Dashboard** (FastAPI + Tailwind + Chart.js on `:8080`):
  - Hero P&L metrics (Realized / Unrealized MTM / Total).
  - Equity curve + recent trade P&L charts.
  - Rich trade log: open vs closed status, entry/current prices, live mark-to-market P/L, filters (now includes Iranian assets).
  - Core/Alpha allocation pie + per-asset risk bars (includes IRAN_* classes).
  - Special public signals card (whales + politician disclosures + Iranian Codal/Eghtesad signals — marked as using separate logic).
  - ML insights + system health indicators.
- **Free / Zero-Key Mode** — Full functionality using yfinance, CoinGecko, public RSS, blockchain explorers, and advanced heuristic + knowledge base (LLM keys optional).
- **24/7 Service** — systemd-ready long-running process with urgent opportunity/risk Telegram alerts.
- **Paper Execution** — Realistic simulation with slippage (CCXT-ready for live).
- **Full Audit Trail** — Every signal stores `decision_audit` JSON (allocator state, hedge, regime, knowledge rationale, causes, and Iran-specific features).
- **Rich CLI** — `allocate`, `risk-report`, `hedge`, `backtest-book`, `ml-status`, `retrain`, `review-mistakes`, etc.

---

## Installation

### 1. Clone & Environment

```bash
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

python -m venv .venv
# Linux/macOS
source .venv/bin/activate
# Windows
# .venv\Scripts\Activate.ps1

pip install -r requirements.txt
```

**Key optional but highly recommended packages** (already in requirements):
- `yfinance` — free high-quality data for Gold, Silver, Oil, Forex.
- `fastapi` + `uvicorn` — for the internal dashboard.
- `scikit-learn` — for the online ML scorer (falls back gracefully).
- `ccxt` — only if you want live execution bridge.

### 2. Minimal Configuration (Free Mode)

Create a `.env` file (or set environment variables):

```env
DATABASE_PATH=simple_trader.db
BACKTEST_MODE=false
LOG_LEVEL=INFO

# Optional — only needed for alerts
# TELEGRAM_BOT_TOKEN=...
# TELEGRAM_CHAT_ID=...

# AI/LLM API Keys (optional — system works fully without them)
# GROQ_API_KEY=...
# OPENAI_API_KEY=...
# GEMINI_API_KEY=...

# No paid news API or AlphaVantage required for core operation (global or Iranian).
```

### 3. Initialize Database

The database is created automatically on first run.

---

## Quick Start

### Run the Full Service (recommended)

```bash
python -m simple_trader.service --interval 300 --dashboard-port 8080
```

- Fetches global news + public disclosures + whales + **dedicated Iranian news** (Eghtesad + Codal) periodically.
- Generates signals through the full Allocator → Risk → Hedge → Knowledge → ML gate (Iran assets use separated algorithms where appropriate).
- Serves the dashboard at `http://localhost:8080`.
- Sends urgent Telegram messages when configured.

### One-off Commands

```bash
python main.py fetch                    # Fetch all news (global + Iranian dedicated)
python main.py process --max-news 50    # Generate signals (LLM if keys present, else heuristic + Iran algos)
python main.py allocate                 # Show Core/Alpha allocation (includes Iranian buckets)
python main.py risk-report              # Book risk + breakers
python main.py hedge                    # Gold/Silver overlay suggestions
python main.py ml-status                # Current cause weights, regrets, model health
python main.py review-mistakes --tag "ignored_hedge,low_conf" --signal-id 42 --lesson "Always force hedge on politician disclosures or Iran Codal events"
python main.py retrain                  # Force auto-retrain from tagged regrets
```

### Dashboard

Open `http://your-server:8080` in a browser (protect with VPN, nginx basic auth, or firewall in production).

The dashboard shows:
- Live P&L (realized + unrealized MTM)
- Equity curve
- Detailed open/closed trade log with status and profit/loss (Iranian assets included)
- Risk allocation (Core/Alpha + IRAN_* breakdown)
- Public signals: whales, politician disclosures, **and Iranian Codal/Eghtesad signals** (noted as using separate dedicated logic)
- Active ML penalties, learning progress, and system health indicators

---

## Iranian Market Support (Dedicated & Separated Module)

Because the Iranian market (TSE/IFB) is largely decoupled from global finance:

- Can grow or react positively to domestic resilience even during international sanctions, wars, or negative global news.
- Uses different primary sources and has its own "world view" (heavy emphasis on Codal.ir filings for corporate/gov disclosures, local economic policy via Eghtesad News, rial volatility, oil exports despite pressure, Islamic finance instruments).
- Supported assets: Iranian stocks, ETFs, corporate/government bonds, fixed income funds, and Islamic Treasury Bonds (اوراق خزانه اسلامی).

**Implementation**:
- Fully separate `simple_trader/iran.py` module with:
  - Dedicated news processor (`IranNewsProcessor`) focused on Eghtesad RSS + Codal keyword scanning.
  - Iran-specific algorithms (`IranIndicators`): Codal event strength, sanctions-resilience scoring, nuanced oil beta.
  - Separate data/chart sources (`IranMarketData`) with hooks for local public portals (tsetmc, ifb, Codal). Falls back gracefully to news-driven signals.
- Integrated into the main system (signals flow through unified risk/hedge/ML) but processed with Iran-specific logic when the asset class is `IRAN_*`.
- Iranian news items are still analyzed by real LLMs (if API keys are configured) via the shared pool, in addition to the dedicated Iran algorithms.
- Asset classes map as: Stocks/ETFs → Alpha; Bonds/Funds/Treasury → Core (or as configured).

This separation ensures the system captures local dynamics without polluting global logic.

---

## Hybrid LLM Support (AI API Keys)

Yes — **real AI/LLM API keys are fully supported** for news analysis:

- Configure any combination of providers (Groq, OpenAI-compatible endpoints, Gemini, etc.) in your `.env`.
- The `llm_pool` uses weighted distribution, rate limiting per provider, and automatic fallbacks between providers.
- All news — including dedicated Iranian items — is sent to the LLM pool when keys are present. Prompts are enriched with the knowledge base (which includes detailed Iranian market rules).
- If no keys are set (or all providers fail), the system automatically and transparently falls back to the advanced `HeuristicAnalyzer` (knowledge base + patterns + indicators + Iran-specific rules). This is logged clearly.
- Result: Best of both worlds — high-quality LLM analysis when available, robust zero-cost operation otherwise.

You can run with keys for maximum quality or without for pure free/public data mode.

---

## CLI Reference (Selected)

| Command              | Purpose |
|----------------------|---------|
| `portfolio`          | Current book risk/exposure snapshot (includes Iranian) |
| `allocate`           | Allocator target vs current + rebalance suggestions |
| `risk-report`        | RiskEngine state, breakers, drawdown |
| `hedge`              | HedgeManager gold/silver recommendations |
| `backtest-book`      | Full Core + Alpha + hedge book simulation |
| `ml-status`          | Cause weights, recent regrets, scorer health |
| `retrain`            | Force auto-retrain from tagged regrets |
| `review-mistakes`    | Review losses + add human judgment tags |
| `record-trade`       | Manually record outcome for a signal |
| `run`                | Continuous fetch → process → close → tune loop |

Run `python main.py --help` for the full list.

---

## Architecture Highlights

- `news_fetcher.py` + `politician_disclosures.py` + `iran.py` — Public RSS + on-chain + mandatory disclosures + **dedicated separated Iranian news/algorithms**.
- `heuristic_analyzer.py` — Full free replacement (or complement) for LLM. Strong asset-specific + regime logic, including Iran-specific rules.
- `portfolio_allocator.py` + `risk_engine.py` + `hedge_manager.py` — Institutional-style risk framework.
- `signal_manager.py` — The "secret formula" gate + cause attribution + regret logging (with Iran features).
- `scorer.py` + `indicators.py` — Online learning with cause-weight penalties + rich technical/meta indicators.
- `web_dashboard.py` — Self-contained beautiful internal UI.
- `service.py` — Production long-running process with urgent monitoring.
- `knowledge_base.py` — Embedded professional rules (Kelly, hedging math, Buffett moats, Simons statistical factors, + detailed Iranian market guidance).
- `llm_pool.py` — Hybrid LLM support with graceful heuristic fallback.

Every signal carries a full `decision_audit` JSON for auditability and cause attribution (including Iran-specific fields when applicable).

---

## Continuous Learning (The Real Edge)

The system treats every loss as data:

1. At signal creation: full context is stored (`decision_audit`, including Iran features).
2. On loss/timeout: causes are automatically attributed.
3. Causes are written to `regret_table` and update persistent `cause_weights`.
4. Negative weights penalize the scorer for similar future situations.
5. 3+ recent bad causes on the same symbol/Alpha can trigger a **regret veto**.
6. Human review (`review-mistakes --tag ...`) provides labeled data for `auto_retrain_from_reviews`.

This loop is designed to make the system genuinely better over time — exactly the "Warren Buffett capital preservation + Jim Simons rigorous data-driven improvement" goal. The separate Iranian algorithms feed the same loop so local patterns are learned distinctly.

---

## Deployment (VPS / systemd)

See `simple-trader.service` example in the repo root.

Typical steps:
1. Clone to `/opt/simple-trader`
2. Create venv + `pip install -r requirements.txt`
3. Configure `.env` (LLM keys optional)
4. `sudo cp simple-trader.service /etc/systemd/system/`
5. Edit paths and user in the service file
6. `sudo systemctl daemon-reload && sudo systemctl enable --now simple-trader`
7. Monitor: `journalctl -u simple-trader -f`

Dashboard: `http://your-vps-ip:8080` (never expose publicly without strong auth).

---

## Important Disclaimers & Ethics

- **Politician disclosures** (US) and **Codal filings** (Iran) are **public, mandatory, lagged** sources. They are sentiment catalysts only — never treated as guaranteed "insider" information.
- Always cross-verify, size small in Alpha, and maintain appropriate hedges.
- This is an **internal company tool only**. Not financial advice. Not for public distribution.
- Past performance (even with ML and dedicated Iran logic) does not guarantee future results. You are responsible for all risk management and execution decisions.

---

## License

MIT License. Internal use for the investment company.

---

**Made for serious internal use. Designed to protect capital first and compound an edge second — while getting demonstrably better with every reviewed mistake. Global and Iranian markets are handled with the respect their differences deserve.**

For questions or internal support, open an issue or contact the team directly.