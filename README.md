# Simple-Trader

Simple-Trader is a modular, production-focused signal generator for crypto and forex instruments. It combines news analysis using multiple LLM providers and 2-hour candlestick pattern detection to generate short-term trade signals. Signals are persisted to a database and can be broadcast via Telegram. This project is intentionally execution-agnostic — it generates signals; placing trades should be handled by your execution system.

Highlights
- Fetches news via RSS and optional news APIs.
- Runs multi-provider LLM analysis in parallel (Groq / Cloudflare / Google / mock).
- Performs 2-hour candlestick pattern detection and ATR-based SL/TP calculation.
- Produces signals with risk-management, minimum R:R enforcement, and suggested position sizing.
- Supports Telegram alerts and an optional Prometheus metrics endpoint.
- Stores news, analyses, market candles, signals, and trades in SQLite (optionally Postgres via migration).
- Lightweight built-in learning/tuning based on recorded trade outcomes.

**⚠️ CRITICAL FOR INVESTMENT / PRODUCTION USE (June 2026 audit)**
This tool **generates ideas/signals only**. It is **NOT** a complete portfolio management or execution system.

Major issues discovered in initial review that could cause large losses or poor decisions in production:
- Crypto market data was previously derived from close prices only (bad high/low for patterns) — **fixed** to prefer real /ohlc endpoint.
- Very limited signal source (news sentiment + classic 2H candlesticks). Easy to overfit, regime dependent, high false positive rate in ranging/choppy markets.
- No true portfolio construction, correlation, or book-level risk (total VaR, sector exposure, drawdown stops). Only per-trade % risk + crude open count. **Improved** with PortfolioManager + total risk budget guard + vol targeting.
- Tuner can auto-adjust (or disable) parameters on small samples — dangerous without heavy oversight. Auto-apply is opt-in and still conservative.
- Paper execution is low-fidelity (candle touch simulation, no realistic fills/funding/latency).
- Heavy LLM reliance without source verification, credibility scoring, or hallucination guards.
- No rigorous walk-forward / Monte-Carlo backtesting framework, no slippage model, no survivorship bias handling.
- SQLite default + no advanced concurrency/transactions for high-volume production.
- No live broker integration (execution gap), no kill switches, limited monitoring.

**What leaders actually use (and you should add/evolve toward):**
- Multi-factor + alternative data (on-chain Glassnode/Dune, options flow, macro, credit, satellite).
- Proper portfolio optimization (risk-parity, HRP, Black-Litterman, vol targeting, Kelly/fractional with drawdown overlay).
- Regime detection + dynamic risk budgeting.
- Full execution stack (CCXT, FIX, smart routing, TWAP/VWAP).
- Institutional data (Polygon, Tiingo, Bloomberg/Refinitiv feeds, paid news).
- Rigorous research platform (vectorized backtester, walk-forward, deflated Sharpe, combinatorial purged CV).
- Real-time risk engine + pre-trade checks + post-trade attribution.
- Human + model ensemble with strict position limits per strategy.

**Recommendations before using real capital:**
1. Run extensive historical backtests + walk-forward on your universe.
2. Forward-test in paper for 3-6+ months with real slippage assumptions.
3. Start with tiny risk (0.1-0.25% per trade) + strict max book risk (3-5%).
4. Add your own portfolio layer on top of signals (never blindly take every signal).
5. Implement circuit breakers (pause on >X% daily loss, vol spike, etc.).
6. Treat every signal as "idea to be vetted", not "trade this now".

**Internal Team Commands (the Secret Formula in action)**
After `pip install -r requirements.txt` (include yfinance + ccxt + fastapi for full power):

- `python main.py portfolio`               → Current book risk/exposure across buckets
- `python main.py allocate`                → Allocator suggestions (Core vs Alpha rebalancing)
- `python main.py risk-report`             → RiskEngine + circuit breaker status
- `python main.py hedge`                   → HedgeManager gold/silver overlay recommendations
- `python main.py backtest-book --days 60 --capital 200000` → Full book simulation (Core + Alpha + hedges)
- `python main.py live-paper-run`          → Advanced paper execution engine

**Minimal / Zero Paid API Keys Mode (Max Free Sources)**
The system is now optimized to run with almost no paid keys:

- **Market data**: yfinance (free, no key) for Gold (GC=F), Silver, Oil (CL=F), Forex (EURUSD=X). CoinGecko (free) for crypto. AlphaVantage only as last resort.
- **News**: Pure public RSS (10+ high-quality free feeds for gold/oil/forex/macro). No NewsAPI needed.
- **Analysis**: Heuristic + Knowledge Base (embedded professional trading/finance expertise) completely replaces LLM when no keys. Strong rule-based direction/confidence/summary using asset knowledge, risk-off detection, patterns.
- **Only "optional paid"**: Telegram bot token (for alerts). If missing or BACKTEST_MODE=true, no messages sent.
- **Execution**: CCXT only if you want live trading (public endpoints for data are free).

Recommended minimal .env for full operation (free mode):
```
DATABASE_PATH=simple_trader.db
BACKTEST_MODE=false
TELEGRAM_BOT_TOKEN=your_token_if_you_want_alerts
# No LLM keys, no AlphaVantage, no NewsAPI needed.
```

With zero keys you still get:
- Free news monitoring
- Knowledge-driven "LLM-like" analysis for gold risk-off, oil supply shocks, etc.
- Full Core/Alpha allocation, hedging, risk engine, UI, service, learning loop.

**VPS / Systemd Deployment (recommended for 24/7)**
1. Clone to `/opt/simple-trader`
2. Create venv, `pip install -r requirements.txt` (yfinance and fastapi/uvicorn for UI)
3. Copy `.env` (can be almost empty for free mode)
4. `sudo cp simple-trader.service /etc/systemd/system/`
5. Edit the .service file (User, WorkingDirectory, paths)
6. `sudo systemctl daemon-reload && sudo systemctl enable --now simple-trader`
7. Logs: `journalctl -u simple-trader -f` and `/var/log/simple-trader/service.log`

Dashboard (internal): http://your-vps-ip:8080 (protect with nginx + auth or firewall/VPN).

**Continuous Learning & Knowledge**
- The system has a rich embedded `knowledge_base.py` with Kelly, risk-parity, asset-specific behaviors (gold as hedge for crypto/oil news events, forex session dynamics, etc.).
- Every decision stores "decision_audit" with knowledge rationale.
- Online learning in SignalScorer + StrategyTuner improves from every trade outcome ("mistakes").
- Use `python main.py record-trade ...` after real or paper results.
- Future: CLI "review-mistakes" to tag bad judgments and force model updates.
- Urgent Telegram alerts for high-opportunity (under-allocated strong Alpha) or risk (approaching breakers, risk-off regime) are sent automatically by the service.

Focus symbols (GOLD/SILVER/OIL/CRYPTO/FOREX) are now first-class with proper data routing and bucket logic. The system tries hard to let Alpha swing while Core (gold) keeps the company alive.

The recent improvements (better crypto candles, vol-adjusted sizing, PortfolioManager guards, safer tuner notes) make it **less dangerous** as a signal generator, but you are still responsible for the rest of the stack. Use at your own risk. Consider this a research/idea-generation prototype for your investment company.

Table of Contents
- [Quickstart](#quickstart)
- [Key Concepts](#key-concepts)
- [Configuration & Environment Variables](#configuration)
- [CLI & Usage](#cli-usage)
- [Database & Migrations](#database-migrations)
- [Telemetry & Observability](#telemetry-observability)
- [Security & Secrets](#security)
- [Testing & Development](#testing-development)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

<a id="quickstart"></a>
## Quickstart
1. Clone the repository:
```bash
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader
```

2. Create a Python virtual environment and install dependencies:
```bash
python -m venv .venv
source .venv/bin/activate       # Linux/Mac
# .venv\\Scripts\\Activate      # Windows

pip install -r requirements.txt
```

3. Copy `.env.example` to `.env` and configure the environment variables listed below.

4. Run a single fetch + processing step:
```bash
# fetch news
python main.py fetch

# create signals from unprocessed news
python main.py process --max-news 100
```

5. Run periodic scanning (production):
```bash
python main.py run --interval 300 --max-news 200
```

<a id="key-concepts"></a>
## Key Concepts
- News Fetching: `NewsFetcher` aggregates RSS and optional news API sources, dedupes entries, and stores raw news.
- LLM Analysis: `LLMPool` queries configured LLM providers and stores each analysis.
- Market Data: `MarketDataClient` collects OHLC data (CoinGecko for crypto; AlphaVantage for forex) and aggregates 2-hour candles.
- Pattern Detection: `PatternDetector` identifies candlestick patterns used to create signals (e.g., engulfing, hammer).
- Signal Manager: `SignalManager` fuses LLM analysis + pattern detection to create signals with entry/stop/target price suggestions.
- Telegram Notifier: Sends formatted message notifications for created/open/closed signals.
- Tuner & Learning: `StrategyTuner` uses recorded trade outcomes to adapt parameters automatically.

<a id="configuration"></a>
## Configuration & Environment Variables
Simple-Trader reads configuration from environment variables (or `.env`). Important variables:
- `DATABASE_PATH`: SQLite file path. Default: `simple_trader.db`.
- `TENANT_ID`: tenant scope for multi-tenant SaaS isolation (letters/numbers/`-`/`_`). Default: `default`.
- `LOG_LEVEL`: e.g., `INFO`, `DEBUG`. Default: `INFO`.
- `ACCOUNT_BALANCE_USD`: Number for position sizing (default tuned conservatively).
- `RISK_PER_TRADE_PCT`: percent of account risk per trade (e.g., `0.01` for 1%).
- `MIN_RISK_REWARD_RATIO`: minimum R:R allowed by the strategy (default `3.0`).
- `MAX_LEVERAGE_CRYPTO`, `MAX_LEVERAGE_FOREX`: leverage caps for position sizing.
- `NEWS_RSS_FEEDS`: CSV list of RSS feed URLs.
- `NEWS_API_KEY`, `CRYPTONEWS_API_KEY`: optional news provider keys.
- `ALPHAVANTAGE_API_KEY`: required for forex intraday OHLC.
- `GROQ_API_KEY`, `CLOUDFLARE_API_KEY`, `GOOGLE_AI_API_KEY`: optional LLM provider keys.
- `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`: for sending alerts via Telegram.
- `BACKTEST_MODE`: `true`/`false` - if true, suppress Telegram notifications.
- `ENABLE_TELEMETRY`: `true`/`false` to serve Prometheus metrics.
- `PROMETHEUS_PORT`: the port Prometheus metrics server will listen on.

A more complete `env` scaffold:
```bash
DATABASE_PATH=simple_trader.db
TENANT_ID=default
LOG_LEVEL=INFO
ACCOUNT_BALANCE_USD=100000
RISK_PER_TRADE_PCT=0.01
MIN_RISK_REWARD_RATIO=3.0
ALPHAVANTAGE_API_KEY=your_alphavantage_key
NEWS_API_KEY=your_newsapi_key
GROQ_API_KEY=your_groq_key
CLOUDFLARE_API_KEY=your_cloudflare_key
GOOGLE_AI_API_KEY=your_google_api_key
TELEGRAM_BOT_TOKEN=botXXXXXXXX:YYYYYYYYY
TELEGRAM_CHAT_ID=-123456789
NEWS_RSS_FEEDS=https://cointelegraph.com/rss,https://www.reuters.com/finance/markets/rss
ENABLE_TELEMETRY=true
PROMETHEUS_PORT=9000
```

<a id="cli-usage"></a>
## CLI & Usage
The CLI command `python main.py` supports the following subcommands:

- Fetch news:
  - `python main.py fetch` — fetch RSS/API news and store them in the DB. `--list` to print configured feeds.

- Process news (create signals from unprocessed news):
  - `python main.py process --max-news 100` — analyze & create signals from news.

- Close expired signals:
  - `python main.py close` — close signals that exceeded the maximum duration (default 24h).

- Monitor & tune:
  - `python main.py monitor --since-seconds 86400` — update tuner stats based on recent trades.

- Run once (fetch → process → close → tune):
  - `python main.py all --max-news 100`

- Run continuously:
  - `python main.py run --interval 300 --max-news 100`

- Record executed trade:
  - `python main.py record-trade --signal-id 123 --executed-price 3.1 --exit-price 3.5 --pnl 120 --outcome win`

- Paper execution simulation:
  - `python main.py paper-exec --lookback-hours 72 --slippage-pct 0.001`

- Tuner suggestions and application:
  - `python main.py suggest --min-win-rate 0.4 --min-avg-rr 3.0 --min-sample-size 10 --apply`

<a id="database-migrations"></a>
## Database & Migrations
- Default local DB is SQLite. For production use, consider migrating to PostgreSQL.
- There is a migrations script: `migrations/001_create_postgres_schema.sql`.
- Use `tools/migrate_sqlite_to_postgres.py` to migrate from SQLite to Postgres if needed:
```bash
python tools/migrate_sqlite_to_postgres.py --sqlite simple_trader.db --pg "postgresql://user:pass@host:5432/dbname"
```

<a id="telemetry-observability"></a>
## Telemetry & Observability
- Enable Prometheus metrics with `ENABLE_TELEMETRY=true`. Use `PROMETHEUS_PORT` to set the port (default `9000`).
- Grafana dashboard sample is included in `grafana/simple_trader_dashboard.json`.

<a id="security"></a>
## Security & Secrets
- Do NOT commit `.env` or any API keys. Use a secure injection mechanism or GitHub secrets in CI.
- Avoid versioning local DB files — use `.gitignore` to exclude `*.db`, `.env` and other local artifacts.
- For SaaS deployments, set a unique `TENANT_ID` per customer/workspace to isolate all runtime data (news, analyses, signals, trades, tuning, and telemetry) at the storage layer.

<a id="testing-development"></a>
## Testing & Development
- Run unit tests (if provided) with pytest:
```bash
pytest
```
- Add debugging by enabling `LOG_LEVEL=DEBUG`.

<a id="troubleshooting"></a>
## Troubleshooting
- LLM endpoints failing: check provider credentials and rate limits, check the DB `llm_usage` for errors.
- Market data not found: ensure symbol maps to a CoinGecko id or confirm AlphaVantage requests/keys for forex.
- Telegram messages missing: check `BACKTEST_MODE`, `TELEGRAM_BOT_TOKEN`, and `TELEGRAM_CHAT_ID`.
- If a secret accidentally gets committed, use `git filter-repo` or `BFG` to remove it and rotate your keys.

<a id="contributing"></a>
## Contributing
Contributions are welcome. Guidelines:
- Fork the repo and create a feature branch.
- Add tests for new features and ensure existing tests pass.
- Follow code style: prefer Black formatting and type hints.
- Open a PR with a clear summary and testing instructions.

<a id="license"></a>
## License
- This repository is licensed under the MIT License (see the full text below).
- If you prefer another license, let the maintainers know and we can update `LICENSE`.

MIT License (simplified)
Copyright (c) 2025 rqzbeh

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, subject to the following conditions:
- The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.

Credits & Acknowledgments
- This project uses CoinGecko for public crypto market data and (optionally) AlphaVantage for forex.
- LLM integrations are intentionally pluggable and can be expanded with provider-specific clients.
- Grafana dashboard is a sample and can be tailored to your monitoring setup.

Contact
If you need help, raise an issue on GitHub: https://github.com/rqzbeh/Simple-Trader/issues

## Public Politician Disclosures (STOCK Act PTRs) + Buffett/Simons ML (added for real compounding edge)
- Free/public source: RSS (opensecrets, reuters/crypto-policy, marketwatch, benzinga) + lightweight public page scans (capitoltrades etc). Matches "disclosure|PTR|STOCK Act|Trump|Pelosi|WLFI|congress".
- High-impact POLITICS signals (provider=politics_disclosure) -> Alpha bucket. Lagged official filings but powerful sentiment catalyst for our 5 assets. Ethics: PUBLIC ONLY, cross-verify, small Alpha size, mandatory Gold hedge.
- Full loop: fetch (news_fetcher + dedicated pol module) -> service 24/7 -> heuristic (boost conf + Simons/Buffett rationale) -> gate (allocator/risk/hedge/knowledge/audit) -> scorer (politics_f, value_f, 12 feats) -> record loss: attribute causes from audit (e.g. insufficient_hedge) -> regret_table + cause_weights persist (delta negative) -> CauseWeightPersister penalty in predict_proba + regret_veto (>=3 bad -> block/discount) -> tuner (cause-weighted suggestions) -> auto_retrain on review --tag + monitor.
- CLIs: ml-status, retrain, review-mistakes --tag "ignored_hedge,low_conf" --signal-id N --lesson "...", suggest --apply.
- Dashboard: Core/Alpha % pie, special disclosures/whales, ML card (weights + recent regrets).
- Gets better: every public disclosure/whale + outcome updates factors (Simons); avoid repeating safety violations (Buffett). Real auditable "learn from mistakes".
See: politician_disclosures.py, scorer.py:CauseWeightPersister, signal_manager (veto+retrain+attribute), db (tables+log), knowledge_base (principles), web_dashboard+service.