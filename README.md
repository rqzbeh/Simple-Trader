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