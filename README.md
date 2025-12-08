Simple-Trader/README.md#L1-999
# Simple-Trader

Simple-Trader is a modular, production-oriented signal generator that creates trade signals for both crypto and forex instruments by combining news analysis (via multiple LLM providers) with candlestick pattern detection (2-hour timeframe). Signals are persisted and broadcast via Telegram. The system is designed to be robust, extensible, and safe to operate in a production environment.

Highlights
- Collects news from RSS and optional HTTP news APIs.
- Uses multiple LLM providers in parallel to extract asset-specific short-term impact and recommendations.
- Performs 2-hour candlestick pattern analysis to find entry points, stop losses, and take profits.
- Generates signals that respect a minimum risk-reward ratio (R:R >= 3) and a max trade duration of 24 hours.
- Persists all news, analyses, signals, and trades to an SQLite database.
- Supports Telegram alerts for broadcasting signals.
- Implements lightweight learning/tuning via recorded trades (tuning stats updated per pattern & symbol).

Important: This project produces signals; it does not execute trades on exchanges. Execution is intentionally externalized (so you can plug it into your own execution engine with proper broker or exchange connectivity).

Contents
- `main.py` CLI orchestrator
- `simple_trader/config.py` Config & environment variable mapping
- `simple_trader/db.py` SQLite schema and DB wrapper
- `simple_trader/news_fetcher.py` RSS/News API fetching & normalization
- `simple_trader/llm_pool.py` Multi-provider LLM distribution & parsing
- `simple_trader/market_data.py` CoinGecko/AlphaVantage market data and 2H candle creation
- `simple_trader/pattern_detector.py` Candlestick-pattern detector & price suggestion
- `simple_trader/signal_manager.py` Signal generation & lifecycle management
- `simple_trader/telegram.py` Telegram notifier to send messages
- `requirements.txt` Python dependencies

Quickstart (local dev)
1. Clone the repository and install dependencies:
```Simple-Trader/README.md#L101-106
pip install -r requirements.txt
```

2. Prepare environment variables. A sample minimal `.env` could be:
```Simple-Trader/README.md#L110-129
DATABASE_PATH=simple_trader.db
LOG_LEVEL=INFO
ACCOUNT_BALANCE_USD=100000
RISK_PER_TRADE_PCT=0.01
MIN_RISK_REWARD_RATIO=3.0
ALPHAVANTAGE_API_KEY=your_alphavantage_key    # optional (needed for forex)
NEWS_API_KEY=your_newsapi_key                 # optional
GROQ_API_KEY=...                              # optional groq LLM key
GROQ_AUTH_HEADER=Authorization                # optional (default 'Authorization')
GROQ_AUTH_PREFIX=Bearer                       # optional (default 'Bearer ')
GROQ_API_ENDPOINT=https://api.groq.ai/v1      # optional; specify if your model endpoint differs
CLOUDFLARE_API_KEY=...                        # optional cloudflare key
CLOUDFLARE_ACCOUNT=...                        # optional: Cloudflare account id to auto-build a model endpoint
CLOUDFLARE_AUTH_HEADER=Authorization          # optional (default 'Authorization')
CLOUDFLARE_AUTH_PREFIX=Bearer                 # optional (default 'Bearer ')
CLOUDFLARE_API_ENDPOINT=https://api.cloudflare.com/client/v4/accounts
GOOGLE_AI_API_KEY=...                         # optional google ai key
GOOGLE_AI_AUTH_HEADER=Authorization           # optional (default 'Authorization')
GOOGLE_AI_AUTH_PREFIX=Bearer                  # optional (default 'Bearer ')
GOOGLE_AI_API_ENDPOINT=https://generative.googleapis.com/v1beta2/models
PROMETHEUS_PORT=9000                           # Prometheus metrics server port (if telemetry enabled)
ENABLE_TELEMETRY=true                          # Enable/disable Prometheus metrics server (default true)
PG_CONN=postgresql://user:pass@host:5432/dbname # optional Postgres conn string for DB migration
TELEGRAM_BOT_TOKEN=botxxxxxxxx:yyyyyyyyyy     # optional telegram token
TELEGRAM_CHAT_ID=-123456789                   # optional telegram chat id
NEWS_RSS_FEEDS=https://cointelegraph.com/rss,https://www.reuters.com/finance/markets/rss
```

3. Run a single fetch + processing:
```Simple-Trader/main.py#L1-25
# fetch news then process it once (create signals)
python main.py fetch
python main.py process --max-news 100
```

4. Run in periodic mode (production / continuous scanning):
```Simple-Trader/main.py#L26-45
python main.py run --interval 300 --max-news 200
```
This will:
- fetch news
- analyze new articles with LLMs
- check for patterns and create signals
- close expired signals
- update tuner stats

Production notes: You should deploy under a process supervisor or run as a systemd service / container with appropriate monitoring and secrets management.

Architecture Overview
- News fetching:
  - `NewsFetcher` supports RSS feeds and NewsAPI.
  - For each article, a hash is stored to prevent duplicate analysis.
- LLM analysis:
  - `LLMPool` supports multiple LLM providers in parallel (groq, cloudflare, google, mock).
  - Each provider has its own `rate_limit_per_minute`. The pool uses a weighted round-robin to distribute requests and enforces per-provider rate limits.
  - The LLM is asked to provide JSON (asset, direction, confidence, impact_score, recommended_leverage, summary).
- Market Data:
  - `MarketDataClient` uses CoinGecko for crypto (free, requires no signup).
  - For forex OHLC intraday candles the project supports AlphaVantage (key required) and converts data to 2H candles by resampling.
  - All 2H candles are cached into SQLite `market_data`.
- Pattern detection:
  - `PatternDetector` uses robust candlestick heuristics to detect patterns like engulfing, hammer, morning/evening star, doji, three soldiers, and others. When available in the environment, the detector augments heuristics with TA-Lib’s pattern detectors for additional reliability (TA-Lib is optional and used when present).
  - TA-Lib-based detections are treated as high-confidence signals and will often raise the confidence of a setup or be used as an override for noisy heuristic outputs. If TA-Lib is not installed, the heuristics still function.
  - The detector computes ATR-based SL/TP suggestions and outputs a suggested entry price, a stop loss, and a take profit that respects a minimum runtime-configurable R:R threshold.
- Signal generation:
  - `SignalManager` fuses LLM direction + pattern detection to produce actionable `Signal`.
  - It computes risk-based position sizing, recommends leverage (capped by config), enforces min R:R >= 3, and sets an expiry for the trade (max 24h).
  - Signals are stored in `signals` table (status: open/closed/cancelled).
- Broadcast:
  - `TelegramNotifier` sends nicely formatted messages to a Telegram channel.
  - Backtest mode disables sending messages and can be used for dry runs.
- Learning / Tuning:
  - `trades`, `tuning_stats` tables aggregate outcomes.
  - `StrategyTuner` updates pattern/symbol success rates (win/loss/avg RR/avg hold) for future adaptation.
  - Record trades manually or through an execution engine—see "Recording Outcomes / Tuner" section.

Environment variables
- `DATABASE_PATH` — path to SQLite file (default: `simple_trader.db`)
- `LOG_LEVEL` — logging level (DEBUG / INFO / WARNING)
- Risk management
  - `ACCOUNT_BALANCE_USD`
  - `RISK_PER_TRADE_PCT` — percent of portfolio risk per trade (e.g., 0.01 = 1%)
  - `MIN_RISK_REWARD_RATIO` — minimal RR (default 3.0)
  - `MAX_LEVERAGE_CRYPTO` — e.g., 10
  - `MAX_LEVERAGE_FOREX` — e.g., 30
- News sources
  - `NEWS_RSS_FEEDS` — CSV list of rss feeds
  - `NEWS_API_KEY` — (optional) key for NewsAPI.org or similar
  - `CRYPTONEWS_API_KEY` — optional provider
- LLM providers
  - `GROQ_API_KEY`, `GROQ_API_ENDPOINT`, `GROQ_RATE_LIMIT_PER_MINUTE`, `GROQ_WEIGHT`, `GROQ_AUTH_HEADER`, `GROQ_AUTH_PREFIX`
  - `CLOUDFLARE_API_KEY`, `CLOUDFLARE_API_ENDPOINT`, `CLOUDFLARE_ACCOUNT`, `CLOUDFLARE_RATE_LIMIT_PER_MINUTE`, `CLOUDFLARE_WEIGHT`, `CLOUDFLARE_AUTH_HEADER`, `CLOUDFLARE_AUTH_PREFIX`
  - `GOOGLE_AI_API_KEY`, `GOOGLE_AI_API_ENDPOINT`, `GOOGLE_WEIGHT`, `GOOGLE_AI_AUTH_HEADER`, `GOOGLE_AI_AUTH_PREFIX`
  - Setting provider API keys, endpoints or auth header info configures the LLM pool to use the provider. If none are set, a `mock` provider is used by default.
- Telegram
  - `TELEGRAM_BOT_TOKEN`
  - `TELEGRAM_CHAT_ID`
  - `BACKTEST_MODE` — if true, Telegram notifications are suppressed
- AlphaVantage
  - `ALPHAVANTAGE_API_KEY` — required for forex intraday OHLC
- Misc
  - `TIMEFRAME_HOURS` — timeframe (2)
  - `MAX_TRADE_DURATION_HOURS` — 24
  - `OPEN_POSITIONS_LIMIT` — maximum concurrently open signals
  - `LLM_GLOBAL_CONCURRENCY` — max concurrent LLM requests

Database Schema (High-level)
- `news` — raw news items with provider, url, title, content, published_at, asset (if auto-detected), `processed` flag, `hash`, `raw_json`
- `analysis` — per-news analysis records created by LLM pools (provider, analysis_json, confidence)
- `market_data` — 2H OHLC per symbol and start_ts
- `signals` — generated signals (news_id, symbol, side, entry_price, stop_loss, take_profit, leverage, position_size, rr, timeframe_hours, status, created_at, expires_at, outcome, pnl)
- `trades` — recorded executed trades (signal_id, executed_at, executed_price, exit_at, exit_price, pnl, outcome)
- `tuning_stats` — aggregated stats used to adapt strategy thresholds per pattern/symbol
- `llm_usage` — logs requests and responses to monitor usage and quotas

Operational Guidance & Best Practices
- Rate limits: ensure each LLM provider is configured with realistic `rate_limit_per_minute`. `LLMPool` uses these to spread the load and throttle requests.
- Storage: SQLite is great for local or small production; for heavy usage consider migrating to PostgreSQL.
- Security: Don’t store secrets in source control. Use proper secret management (KMS, Vault, or env injection).
- Backtesting: Use `BACKTEST_MODE=true` to prevent Telegram alerts from being sent while testing.
- Execution: This project generates signals. Use a production-grade execution engine for placing orders with risk-limits and compliance checks.
- Updating & Learning: After a trade is executed, call `SignalManager.record_trade_result(...)` to persist the trade details and allow automatic tuning signals.
- Monitoring: Keep an eye on `llm_usage` table and logs for rate-limit errors.
- Telegram: Use private chat or a dedicated alert channel and bot token scoped to the minimum required permissions.

Extending / Adding Providers
- Add an LLM provider: add credentials as env vars following the `llm_providers` pattern (GROQ/CLOUDFLARE/GOOGLE). If needed, extend `Simple-Trader/simple_trader/llm_pool.py` with a provider-specific client subclass and specific payload/response mapping.
- Add a new news source: put it into `NEWS_RSS_FEEDS` or implement a new API client inside `news_fetcher.py`.
- Add feature detection: extend `pattern_detector.py` with custom heuristics.

How learning works (conceptually)
1. Generate signals and optionally execute them in your execution system.
2. When a trade has closed (via take-profit, stop-loss, manual exit), the execution system calls `SignalManager.record_trade_result(...)` providing `signal_id`, `executed_at`, `exit_at`, `executed_price`, `exit_price`, `pnl`, `outcome`.
3. `StrategyTuner` (via `monitor_and_tune`) collects recent trades, increments per-pattern win/loss counts, and keeps rolling averages for RR and hold duration.
4. Tuner stats can be used to adjust: `min_confidence`, `min_pattern_confidence`, per-symbol or per-pattern `weight` or to disable low-performing patterns.

Security & Production Checklist
- Rotate LLM & API keys regularly.
- Back up the SQLite DB or migrate to a robust server with backups.
- Use HTTPS only for key usage and telebot usage.
- Rate-limit network egress for LLM providers and avoid large concurrency without quotas in place.
- Monitor every default configuration (e.g., `ACCOUNT_BALANCE_USD`) before going live.

Where to go from here
- Add a proper execution connector (e.g., CCXT or exchange-specific client) to take signals and place limit/stop orders, and then call `record_trade_result` when trade closes.
- Add more advanced features to the learning module:
  - Persistent ML models for pattern success per instrument.
  - Parameter optimization via grid search on historical trades.
- Observability & Telemetry:
  - The project includes an optional Prometheus metrics endpoint and a sample Grafana dashboard in `grafana/simple_trader_dashboard.json`.
  - To enable: set `ENABLE_TELEMETRY=true` and optionally override `PROMETHEUS_PORT` (default 9000). The system will start an HTTP metrics server that exports counters & gauges about LLM usage, signals created/closed, and open signals.
  - Suggested dashboard panels: LLM request volume by provider, LLM latency (p50/p95), open signals, signals created/signals closed, and news processed.
- Migration & persistent DB plan:
  - For production, we recommend migrating the default local SQLite DB to a persistent Postgres DB.
  - The repository includes a migration SQL (`migrations/001_create_postgres_schema.sql`) to bootstrap the Postgres schema and a migration helper script:
    - `tools/migrate_sqlite_to_postgres.py --sqlite simple_trader.db --pg \"postgresql://user:pass@host:5432/dbname\"`
  - This script preserves IDs to keep relationships intact and migrates the core tables: `news`, `analysis`, `market_data`, `signals`, `trades`, `tuning_stats`, `llm_usage`, `runtime_params` and `tuning_history`.
  - After migration, point `DATABASE_PATH` or your DB connector to Postgres and verify all application runtime params are in place (`runtime_params` table). Consider using a migration tool like Alembic for long-term schema evolution.
- Add a web dashboard (Flask/FastAPI) to visualize signals, trades, and tuning info in real time.

Supported CLI Commands
- `python main.py fetch` — fetch news from RSS/API
- `python main.py process --max-news 100` — analyze & create signals from unprocessed news
- `python main.py close` — close expired signals (24h timeout)
- `python main.py monitor` — update tuner stats from recent trades
- `python main.py all` — run fetch, process, close, and monitor once
- `python main.py run --interval 300` — run in a loop (continual scanning)

Example: run fetch & process once, then list the open signals:
```Simple-Trader/main.py#L1-45
python main.py fetch
python main.py process --max-news 80
python main.py status
```

Troubleshooting
- If `market_data` can't fetch a ticker: check if the coin's ticker maps to a CoinGecko ID; adjust symbol or add a mapping.
- If LLM parsing fails: ensure the provider is configured and its `endpoint` / `api_key` are correct. For Google / Cloudflare / Groq you might need different request payloads than the generic JSON.
- If Telegram alerts are missing despite a signal creation: check `BACKTEST_MODE`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, and `LOG_LEVEL` for hints.

Contributing
- We welcome improvements. If you add providers or trading logic, please include tests and consider adding a README entry for the integration.
- Follow code style guidelines (Black, mypy) and provide tests for new features.

Disclaimer
This tool is intended as an advanced research & signal generation platform. It is not a complete trading system and does not execute trades automatically. Trading involves risk; do not use signals in production without proper testing and a risk-managed execution system.

License
- This repository contains free software. Check with maintainers for licensing details as needed.
