# Technical Specification: Simple-Trader Full Architecture Rewrite (Go + React PWA with Bun + PostgreSQL + Redis)

**Date**: 2026-09-20  
**Author**: Antigravity / Pair Programming Agent  
**Repository**: `rqzbeh/Simple-Trader`  
**Status**: Ready for Implementation  

---

## 1. Executive Summary & Goals

Simple-Trader is being completely redesigned and rewritten from its legacy Python prototype into an enterprise-grade quantitative trading platform.

### Core Deliverables
1. **Backend Rewrite**: 100% Go (`go 1.24`), delivering microsecond indicator calculations, low memory footprint, thread-safe concurrent routines, and robust HTTP/SSE/WebSocket servers.
2. **Purge Legacy Iranian Assets**: Completely eliminate all legacy Iranian market modules (`iran.py`, TSE/IFB, Codal, Eghtesad, Islamic treasury bonds). Focus exclusively on liquid global markets:
   - **Core Book**: Gold (`XAU/USD`), Silver (`XAG/USD`) for capital preservation and macro hedging.
   - **Alpha Book**: High-liquidity Crypto (`BTC/USD`, `ETH/USD`, `SOL/USD`, `BNB/USD`), Major Forex (`EUR/USD`, `GBP/USD`, `USD/JPY`), and Commodities (`WTI/USD`).
3. **High-Performance Caching Layer (Redis 7)**:
   - Sub-millisecond read cache for live ticker prices, latest OHLCV candles, and order books.
   - Fast session caching, pub/sub for real-time market ticks and trading signals to connected web clients.
   - Write-through / write-behind caching before PostgreSQL persistence to maximize database throughput.
4. **Unified OpenAI-Compatible AI Engine**:
   - Single standard client supporting any OpenAI-compatible provider (OpenAI, Groq, vLLM, Ollama, OpenRouter, or local proxies such as OmniRoute).
   - Dynamic model configuration (`base_url`, `api_key`, `model_id`, temperature, timeout).
   - Robust structured JSON output parsing (`response_format: {"type": "json_object"}`).
5. **Adaptive Trade Learning & Dynamic Indicator Weight Manipulation**:
   - Post-trade outcome analyzer with root-cause attribution (loss regret penalties vs. profit bonuses).
   - Dynamic Indicator Weight Matrix persisted in PostgreSQL & cached in Redis across regimes (Bull, Bear, Volatile, Ranging).
   - In-context adaptive prompt injection feeding learned weights and failure patterns directly into the LLM.
   - Continuous JSONL dataset generator for model fine-tuning (`/v1/fine_tuning` ready).
6. **Modern Technical Indicator Suite**: Pure Go implementations of RSI, MACD, Bollinger Bands, ATR, SuperTrend, EMA (9/21/50/200), VWAP, Stochastic RSI, and Confluence Scoring.
7. **Next-Gen Global News & Signal Aggregator**: Real-time RSS & JSON feeds (CoinDesk, FXStreet, Reuters, CryptoPanic, CoinGecko, Whale Alerts).
8. **React + TypeScript PWA Powered by Bun**:
   - Fast, modern runtime using **Bun** (`bun 1.3.x`) for package management and bundling.
   - **Progressive Web App (PWA)**: Offline service worker, web manifest, installable on desktop and mobile, responsive layout.
   - Modern Vite + React 18 + TailwindCSS + Lucide Icons + TradingView Lightweight Charts.
   - Dark mode / Light mode toggle with persistent local preference.
   - Real-time SSE / WebSocket connection for live prices, signals, and trades.
   - AI dynamic weight manipulation visualizer & fine-tune dataset management.
9. **Production Deployment**: Docker Compose (`postgres` + `redis` + `backend` + `frontend-pwa`), environment templates, database migration engine, and health checks.

---

## 2. System Architecture Diagram

```
+-----------------------------------------------------------------------------------------+
|                  React + TypeScript Progressive Web App (PWA)                           |
|                  - Built & Managed with Bun 1.3+ (Vite + Workbox PWA)                  |
|                  - Installable on Desktop/Mobile, Offline Service Worker                |
|                  - Dark / Light Mode with Instant Theme Switching                       |
|                  - TradingView Lightweight Charts + Real-time SSE / WebSockets          |
+--------------------------------------------+--------------------------------------------+
                                             | HTTP / SSE / WS
                                             v
+-----------------------------------------------------------------------------------------+
|                                Go Trading Backend (1.24)                                |
|                                                                                         |
|  +--------------------+   +---------------------+   +--------------------------------+  |
|  | Market Data Engine |   | Indicator Engine    |   | Unified AI Reasoning           |  |
|  | - Binance & Yahoo  |   | - RSI, MACD, BB,    |   | - OpenAI-compatible            |  |
|  | - CoinGecko API    |   |   SuperTrend, ATR,  |   |   client (Any Model ID)        |  |
|  | - Global News RSS  |   |   VWAP, EMA Ribbon  |   | - In-Context Learned Weights   |  |
|  +---------+----------+   +----------+----------+   +---------------+----------------+  |
|            |                         |                              |                   |
|            +-------------------------+---------------+--------------+                   |
|                                                      v                                  |
|                                     +--------------------------------+                  |
|                                     | Adaptive Learning & Weights    |                  |
|                                     | - Trade Regret Minimization    |                  |
|                                     | - Dynamic Weight Manipulation  |                  |
|                                     | - JSONL Fine-Tune Exporter     |                  |
|                                     +----------------+---------------+                  |
+------------------------------------------------------+----------------------------------+
                             |                         |
              Cache / PubSub |                         | Storage / Audits
                             v                         v
                   +-------------------+     +--------------------+
                   |      Redis 7      |     |   PostgreSQL 16    |
                   | - Price Tickers   |     | - Persistent State |
                   | - Active Candles  |     | - Trade History    |
                   | - PubSub Events   |     | - Audit Trails     |
                   | - Fast Weights    |     | - Finetune Dataset |
                   +-------------------+     +--------------------+
```

---

## 3. Directory Layout

```
Simple-Trader/
├── docker-compose.yml              # Production compose: Postgres 16 + Redis 7 + Backend + Frontend
├── Dockerfile.backend              # Multi-stage Go build
├── Dockerfile.frontend             # Multi-stage Bun build (Vite PWA static bundle)
├── Makefile                        # Dev/build commands using Bun and Go
├── .env.example                    # Exhaustive configuration template
├── go.mod / go.sum                 # Go dependencies
├── migrations/                     # PostgreSQL database migrations
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── cmd/
│   └── server/
│       └── main.go                 # App entrypoint & CLI commands
├── internal/
│   ├── config/                     # Configuration loading from environment
│   ├── db/                         # PostgreSQL connection pool (pgx), models, repos
│   ├── cache/                      # Redis client wrapper, key patterns, pub/sub
│   ├── indicators/                 # Pure Go technical indicators
│   ├── market/                     # Market data ingestion (Binance, CoinGecko, Yahoo Finance)
│   ├── news/                       # Global financial news fetchers & deduplication
│   ├── ai/                         # Unified OpenAI-compatible client & prompt builder
│   ├── learning/                   # Regret tracking, dynamic weight matrix, fine-tuning exporter
│   ├── risk/                       # Core/Alpha allocator, position sizing, circuit breakers
│   ├── trader/                     # Signal generator, paper/live execution engine
│   └── api/                        # REST router, SSE real-time streaming, WebSocket handler
└── web/                            # React + TypeScript PWA (Bun)
    ├── package.json                # Dependencies managed by Bun
    ├── bun.lockb                   # Bun lockfile
    ├── vite.config.ts              # Vite config with vite-plugin-pwa
    ├── tailwind.config.js          # Tailwind styling with dark/light themes
    ├── public/
    │   ├── favicon.svg
    │   ├── pwa-192x192.png
    │   ├── pwa-512x512.png
    │   └── manifest.webmanifest    # PWA web manifest
    └── src/
        ├── components/             # Reusable UI components
        ├── context/                # ThemeContext, WebSocketContext
        ├── pages/
        │   ├── Dashboard.tsx       # Portfolio overview, PnL, Core vs Alpha
        │   ├── ChartView.tsx       # TradingView Lightweight Charts + overlays
        │   ├── SignalsView.tsx     # Live AI signals with indicator breakdowns
        │   ├── TradeHistory.tsx    # Detailed trade logs & post-mortem audits
        │   ├── AIWeightsView.tsx   # Dynamic indicator weights & fine-tuning
        │   └── SettingsView.tsx    # OpenAI endpoint & model-id config
        └── services/               # API client and real-time event listeners
```

---

## 4. Redis Caching & PubSub Strategy

| Key Pattern | Data Structure | TTL | Purpose |
|---|---|---|---|
| `ticker:{symbol}` | String (JSON) | 5s | Real-time live price, 24h change, high/low |
| `candles:{symbol}:{timeframe}` | List / ZSet | 1h | In-memory latest OHLCV candles for instant indicator computation |
| `weights:{symbol}:{regime}` | Hash | 24h | Dynamic indicator weight matrix for microsecond signal scoring |
| `cache:news:hash:{hash}` | String (Flag) | 7d | Deduplication cache to prevent re-analyzing identical news |
| `pubsub:market_ticks` | Channel | - | Streams live market price updates to SSE / WebSocket clients |
| `pubsub:signals` | Channel | - | Broadcasts generated trading signals immediately to active UIs |

---

## 5. PostgreSQL Database Schema

```sql
-- 1. Market Data Candles (Hyper-fast OHLCV storage)
CREATE TABLE IF NOT EXISTS candles (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    timeframe VARCHAR(10) NOT NULL,
    open_time TIMESTAMPTZ NOT NULL,
    open NUMERIC(20, 8) NOT NULL,
    high NUMERIC(20, 8) NOT NULL,
    low NUMERIC(20, 8) NOT NULL,
    close NUMERIC(20, 8) NOT NULL,
    volume NUMERIC(24, 8) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_candle UNIQUE(symbol, timeframe, open_time)
);
CREATE INDEX idx_candles_symbol_time ON candles(symbol, timeframe, open_time DESC);

-- 2. News Items (Global RSS / API feeds)
CREATE TABLE IF NOT EXISTS news_items (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(64) NOT NULL,
    headline TEXT NOT NULL,
    summary TEXT,
    url TEXT,
    published_at TIMESTAMPTZ NOT NULL,
    asset_tag VARCHAR(32),
    sentiment_score REAL DEFAULT 0.0,
    hash VARCHAR(64) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_news_published ON news_items(published_at DESC);

-- 3. Dynamic Indicator Weights (Learned & Adjusted per Asset & Market Regime)
CREATE TABLE IF NOT EXISTS indicator_weights (
    id SERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    regime VARCHAR(32) NOT NULL DEFAULT 'NORMAL', -- 'BULL', 'BEAR', 'VOLATILE', 'RANGING'
    indicator_name VARCHAR(64) NOT NULL,
    weight DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    win_count INT NOT NULL DEFAULT 0,
    loss_count INT NOT NULL DEFAULT 0,
    cumulative_pnl NUMERIC(16, 4) NOT NULL DEFAULT 0.0,
    last_updated TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_weight UNIQUE(symbol, regime, indicator_name)
);

-- 4. Trading Signals (AI + Confluence Generated)
CREATE TABLE IF NOT EXISTS signals (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL,
    side VARCHAR(8) NOT NULL,                  -- 'BUY' or 'SELL'
    bucket VARCHAR(16) NOT NULL,               -- 'CORE' or 'ALPHA'
    entry_price NUMERIC(20, 8) NOT NULL,
    stop_loss NUMERIC(20, 8) NOT NULL,
    take_profit NUMERIC(20, 8) NOT NULL,
    confidence REAL NOT NULL,
    confluence_score REAL NOT NULL,
    ai_reasoning TEXT,
    indicator_snapshot JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 5. Trades & Positions (Executions, Live PnL, Close Audit)
CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    signal_id BIGINT REFERENCES signals(id),
    symbol VARCHAR(32) NOT NULL,
    side VARCHAR(8) NOT NULL,
    bucket VARCHAR(16) NOT NULL,
    position_size NUMERIC(20, 8) NOT NULL,
    entry_price NUMERIC(20, 8) NOT NULL,
    entry_time TIMESTAMPTZ NOT NULL,
    exit_price NUMERIC(20, 8),
    exit_time TIMESTAMPTZ,
    stop_loss NUMERIC(20, 8) NOT NULL,
    take_profit NUMERIC(20, 8) NOT NULL,
    realized_pnl NUMERIC(16, 4) DEFAULT 0.0,
    return_pct REAL DEFAULT 0.0,
    exit_reason VARCHAR(64),
    root_cause VARCHAR(128),
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 6. Fine-Tuning Dataset (Exportable JSONL pairs)
CREATE TABLE IF NOT EXISTS fine_tune_records (
    id BIGSERIAL PRIMARY KEY,
    trade_id BIGINT REFERENCES trades(id),
    symbol VARCHAR(32) NOT NULL,
    prompt_system TEXT NOT NULL,
    prompt_user TEXT NOT NULL,
    assistant_response TEXT NOT NULL,
    trade_outcome VARCHAR(16) NOT NULL,        -- 'WIN' or 'LOSS'
    exported BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 6. React + TypeScript PWA (Bun) Specification

### 6.1 Progressive Web App Capabilities
* Configured using `vite-plugin-pwa` with Workbox service worker caching strategy.
* Offline caching of application shell, fonts, and icons.
* Web App Manifest enabling standalone installation on iOS, Android, macOS, and Windows.
* Status bar styling and theme color matching dark/light mode.

### 6.2 Key Views
1. **Dashboard View**:
   - Total Portfolio Value, Realized / Unrealized MTM PnL, Win Rate, Profit Factor, Drawdown.
   - Core Bucket (Gold/Silver) vs. Alpha Bucket (Crypto/Forex/Oil) dynamic allocation dial.
   - Recent signal feed with AI conviction score.
2. **Chart & Technical View**:
   - TradingView Lightweight Charts with candlestick data and interactive crosshair.
   - Indicator overlays: Bollinger Bands, EMA ribbon (20, 50, 200), SuperTrend, and volume histogram.
   - Timeframe switcher (1m, 5m, 15m, 1h, 4h, 1d).
3. **AI Learning & Weight Matrix Console**:
   - Visual matrix of dynamic indicator weights per asset and market regime.
   - Real-time display of regret attribution and learning adjustments.
   - One-click fine-tuning dataset export (`.jsonl`) for training custom LLMs.
4. **Trade Execution & History**:
   - Open positions with live unrealized P&L and emergency market-close button.
   - Historical trade ledger with entry/exit timestamps, fees, realized P&L, and AI post-mortem tags.
5. **System Settings**:
   - Unified OpenAI-compatible endpoint URL, API key, and model-id (`gpt-4o`, `claude-3-5-sonnet`, `deepseek-chat`, `meta-llama/llama-3.3-70b`, local Ollama/vLLM).
   - Test connection button with latency reporting.

---

## 7. Implementation Plan

1. **Step 1: Clean Up Legacy Python & Iranian Modules**
   - Remove obsolete files (`iran.py`, legacy python scripts, old sqlite migrations).
   - Initialize clean Go project (`go.mod`, directory skeleton).
2. **Step 2: Database & Redis Infrastructure**
   - Implement PostgreSQL schema migrations and `pgx` connection pool.
   - Implement Redis client wrapper with pub/sub and key caching utilities.
3. **Step 3: High-Performance Indicator Engine (Go)**
   - Implement pure Go mathematical routines: RSI, MACD, Bollinger Bands, ATR, SuperTrend, EMA, VWAP, Confluence.
   - Write unit tests validating indicator accuracy against standard TA benchmarks.
4. **Step 4: Market Data & Global News Fetchers**
   - Implement live market fetchers (Binance public API, CoinGecko, Yahoo Finance).
   - Implement global financial RSS/API feed scraper with content deduplication.
5. **Step 5: Unified OpenAI-Compatible AI Client & Learning Engine**
   - Implement flexible HTTP chat completions client with model ID selection.
   - Implement trade outcome feedback loop: regret minimization, dynamic weight adjustment, and JSONL fine-tuning dataset generation.
6. **Step 6: Trading Execution & Risk Engine**
   - Implement Core/Alpha portfolio allocator, position sizing, circuit breakers, and paper execution engine.
7. **Step 7: REST API & Real-time SSE / WebSocket Service**
   - Implement Go HTTP API router, SSE price/trade broadcasters, and endpoints.
8. **Step 8: React + TypeScript PWA Frontend (Bun)**
   - Initialize Vite + React 18 + TypeScript + Tailwind CSS using Bun.
   - Configure PWA manifest, service worker, and dark/light mode toggle.
   - Implement Dashboard, TradingView charts, Signals, AI Weight Matrix, and Settings views.
9. **Step 9: Dockerization & Production Verification**
   - Create multi-stage `Dockerfile.backend` (Go) and `Dockerfile.frontend` (Bun).
   - Create `docker-compose.yml` orchestrating PostgreSQL, Redis, Backend, and Frontend.
   - End-to-end integration testing and verification.
