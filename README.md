# Secret Formula — Internal Investment System

**Simple-Trader** (internally called the "Secret Formula") is a production-grade, 24/7 autonomous portfolio management backend for an investment company.

It maintains a disciplined **Core** allocation (~55% target risk) in Gold and Silver for capital preservation, inflation hedging, and risk minimization, while running an **Alpha** book (~45%) in Crypto, Forex, and Oil that aggressively seeks short-term "massive profits" from news, on-chain flows, and public high-impact sentiment catalysts — especially **mandatory politician trading disclosures**.

The system is deliberately built to run mostly on **free public data sources** and continuously learns from every outcome using real machine learning (regret tracking, cause attribution, weight persistence, auto-retraining, and veto logic).

> **Philosophy**: Buffett-level capital protection + Jim Simons-level statistical edge from public data + relentless improvement from mistakes.

---

## Core Philosophy

- **Core Bucket (Gold/Silver)**: Preservation first. Gold acts as the ultimate moat and inflation/risk-off hedge. Position sizing increases when value is high (dovish policy, risk-off headlines).
- **Alpha Bucket (Crypto / Forex / Oil)**: High-conviction, short-horizon (2h–24h) ideas driven by news flow, whale on-chain moves, and **public politician disclosures**.
- **Public Edge Sources** (all free):
  - Politician disclosures (STOCK Act PTRs, congressional filings) — lagged but official and high-signal for Crypto (Trump/WLFI), Oil/Energy policy, macro (Fed, tariffs).
  - On-chain whale activity (public blockchain explorers).
  - High-quality public RSS (macro, energy, crypto policy).
- **Risk Architecture**: Hard bucket targets + per-class caps + daily loss / drawdown circuit breakers + automatic Gold/Silver hedging overlay.
- **Learning System**: Every loss is attributed to explicit causes from the decision audit. Causes become persistent negative weights. Repeated mistakes trigger regret vetoes. Human review tags drive auto-retraining.

---

## Key Features

- **PortfolioAllocator** — Computes desired risk for Core vs Alpha with concentration guards and rebalance suggestions.
- **RiskEngine** — Pre-trade checks, circuit breakers, daily loss limits, max drawdown pause.
- **HedgeManager** — Automatically recommends increasing Gold/Silver when Alpha risk is high or risk-off regime is detected.
- **Public Politician Disclosures** — Dedicated free fetcher for STOCK Act / PTR signals. Treated as high-impact Alpha catalysts with strict hedging requirements.
- **Advanced ML Loop** (real & auditable):
  - `regret_table` + `cause_weights` persisted in DB.
  - Cause attribution from `decision_audit` + outcome + knowledge rules (e.g. `insufficient_hedge`, `ignored_politician_bearish_disclosure`).
  - `CauseWeightPersister` applies penalties inside the scorer.
  - Regret veto in the signal creation gate.
  - Auto-retrain from human judgment tags (`review-mistakes --tag "ignored_hedge,low_conf"`).
  - Enriched scorer features (whale flag, politics flag, Buffett value proxy, hedge ratio at entry, etc.).
- **Beautiful Internal Dashboard** (FastAPI + Tailwind + Chart.js on `:8080`):
  - Hero P&L metrics (Realized / Unrealized MTM / Total).
  - Equity curve + recent trade P&L charts ("how are our trades doing?").
  - Rich trade log: open vs closed status, entry/current prices, live mark-to-market P/L, filters.
  - Core/Alpha allocation pie + per-asset risk bars.
  - Special public signals card (whales + politician disclosures).
  - ML insights (active cause penalties + recent regrets).
- **Free / Zero-Key Mode** — Full functionality using yfinance (metals/oil/forex), CoinGecko (crypto), public RSS, public blockchain explorers, and a powerful embedded heuristic + knowledge base (no LLM keys required).
- **24/7 Service** — systemd-ready long-running process with urgent opportunity/risk Telegram alerts.
- **Paper Execution** — Realistic simulation with slippage (CCXT-ready for live).
- **Full Audit Trail** — Every signal stores `decision_audit` JSON containing allocator state, hedge suggestion, regime, knowledge rationale, and causes.
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
- `yfinance` — free high-quality data for Gold (GC=F), Silver, Oil (CL=F), Forex.
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

# No LLM keys, no paid news API, no AlphaVantage required for core operation.
```

### 3. Initialize Database

The database is created automatically on first run. You can also run:

```bash
python main.py status
```

---

## Quick Start

### Run the Full Service (recommended)

```bash
python -m simple_trader.service --interval 300 --dashboard-port 8080
```

- Fetches news + public disclosures + whales periodically.
- Generates signals through the full Allocator → Risk → Hedge → Knowledge → ML gate.
- Serves the dashboard at `http://localhost:8080`.
- Sends urgent Telegram messages when configured.

### One-off Commands

```bash
python main.py fetch                    # Fetch news + public disclosures
python main.py process --max-news 50    # Generate signals
python main.py allocate                 # Show Core/Alpha allocation
python main.py risk-report              # Book risk + breakers
python main.py hedge                    # Gold/Silver overlay suggestions
python main.py ml-status                # Current cause weights, regrets, model health
python main.py review-mistakes --tag "ignored_hedge,low_conf" --signal-id 42 --lesson "Always force hedge on politician disclosures"
python main.py retrain                  # Force retrain from tagged regrets
```

### Dashboard

Open `http://your-server:8080` in a browser (protect with VPN, nginx basic auth, or firewall in production).

The dashboard shows:
- Live P&L (realized + unrealized MTM)
- Equity curve
- Detailed open/closed trade log with status and profit/loss
- Risk allocation
- Public politician & whale signals
- Active ML penalties and learning progress

---

## CLI Reference (Selected)

| Command              | Purpose |
|----------------------|---------|
| `portfolio`          | Current book risk/exposure snapshot |
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

- `news_fetcher.py` + `politician_disclosures.py` — Public RSS + on-chain + mandatory disclosure sources.
- `heuristic_analyzer.py` — Full free replacement for LLM (strong asset-specific + regime logic).
- `portfolio_allocator.py` + `risk_engine.py` + `hedge_manager.py` — Institutional-style risk framework.
- `signal_manager.py` — The "secret formula" gate + cause attribution + regret logging.
- `scorer.py` — Online learning with cause-weight penalties.
- `web_dashboard.py` — Self-contained beautiful internal UI.
- `service.py` — Production long-running process with urgent monitoring.
- `knowledge_base.py` — Embedded professional rules (Kelly, hedging math, Buffett moats, Simons statistical factors).

Every signal carries a full `decision_audit` JSON for auditability and cause attribution.

---

## Continuous Learning (The Real Edge)

The system treats every loss as data:

1. At signal creation: full context is stored (`decision_audit`).
2. On loss/timeout: causes are automatically attributed (e.g. `insufficient_hedge`, `ignored_politician_bearish_disclosure`).
3. Causes are written to `regret_table` and update persistent `cause_weights`.
4. Negative weights penalize the scorer for similar future situations.
5. 3+ recent bad causes on the same symbol/Alpha can trigger a **regret veto** (signal blocked or heavily discounted).
6. Human review (`review-mistakes --tag ...`) provides labeled data for `auto_retrain_from_reviews`.

This loop is designed to make the system genuinely better over time — exactly the "Warren Buffett capital preservation + Jim Simons rigorous data-driven improvement" goal.

---

## Deployment (VPS / systemd)

See `simple-trader.service` example in the repo root.

Typical steps:
1. Clone to `/opt/simple-trader`
2. Create venv + `pip install -r requirements.txt`
3. Configure `.env`
4. `sudo cp simple-trader.service /etc/systemd/system/`
5. Edit paths and user in the service file
6. `sudo systemctl daemon-reload && sudo systemctl enable --now simple-trader`
7. Monitor: `journalctl -u simple-trader -f`

Dashboard: `http://your-vps-ip:8080` (never expose publicly without strong auth).

---

## Important Disclaimers & Ethics

- **Politician disclosures** are **public, mandatory, lagged filings** (STOCK Act). They are sentiment catalysts only — never treated as guaranteed "insider" information.
- Always cross-verify, size small in Alpha, and maintain Gold hedges.
- This is an **internal company tool only**. Not financial advice. Not for public distribution.
- Past performance (even with ML) does not guarantee future results. You are responsible for all risk management and execution decisions.

---

## License

MIT License. Internal use for the investment company.

---

**Made for serious internal use. Designed to protect capital first and compound an edge second — while getting demonstrably better with every reviewed mistake.**

For questions or internal support, open an issue or contact the team directly.