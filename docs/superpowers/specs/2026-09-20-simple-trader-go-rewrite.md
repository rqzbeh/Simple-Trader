# Technical Specification: Simple-Trader Full Architecture Rewrite (Go + React + PostgreSQL)

**Date**: 2026-09-20  
**Author**: Antigravity / Pair Programming Agent  
**Repository**: `rqzbeh/Simple-Trader`  
**Status**: Draft for User Review  

---

## 1. Executive Summary & Goals

Simple-Trader is being completely rewritten from its legacy Python prototype into an ultra-high performance, production-ready quantitative and autonomous trading system.

### Key Objectives
1. **Backend Rewrite**: 100% Go (`go 1.24`), delivering microsecond indicator calculations, safe concurrent routine pipelines, low memory footprint, and production HTTP/WebSocket services.
2. **Purge Legacy Iranian Assets**: Completely remove all Iranian bourse (`iran.py`, TSE/IFB, Codal, Eghtesad, Islamic treasury bonds). Focus purely on liquid global markets:
   - **Core Book**: Gold (`XAU/USD`), Silver (`XAG/USD`) for capital preservation and macro hedge.
   - **Alpha Book**: High-liquidity Crypto (`BTC/USD`, `ETH/USD`, `SOL/USD`, `BNB/USD`), Major Forex (`EUR/USD`, `GBP/USD`, `USD/JPY`), and Commodities (`WTI/USD`).
3. **Unified OpenAI-Compatible AI Engine**: Single client supporting any OpenAI-compatible provider (OpenAI, Groq, vLLM, Ollama, OpenRouter, or local proxies) with configurable `base_url`, `api_key`, `model_id`, temperature, and timeout.
4. **Adaptive Trade Learning & Dynamic Indicator Weight Manipulation**:
   - Post-trade outcome analyzer with root-cause attribution (loss regret penalties vs. profit bonuses).
   - Dynamic Indicator Weight Matrix persisted in PostgreSQL across regimes (Bull, Bear, Volatile, Ranging).
   - In-context adaptive prompt injection feeding learned weights and failure patterns directly into the LLM.
   - Continuous JSONL dataset generator for model fine-tuning (`/v1/fine_tuning` ready).
5. **Modern Technical Indicator Suite**: Pure Go implementations of RSI, MACD, Bollinger Bands, ATR, SuperTrend, EMA (9/21/50/200), VWAP, Stochastic RSI, and Confluence Scoring.
6. **Next-Gen Global News & Signal Aggregator**: Real-time RSS & JSON feeds (CoinDesk, FXStreet, Reuters, CryptoPanic, CoinGecko, Whale Alerts).
7. **Production React + TypeScript Frontend**:
   - Modern Vite + React 18 + TailwindCSS + Lucide Icons + TradingView Lightweight Charts.
   - Dark mode / Light mode toggle with persistent preference.
   - Real-time WebSocket connection for live prices, signals, and trades.
   - AI dynamic weight manipulation visualizer & fine-tune dataset management.
8. **Production Deployment**: Docker Compose (`db` + `backend` + `frontend` reverse proxy), environment templates, database migration engine, and health checks.

---

## 2. System Architecture & Directory Structure

```
Simple-Trader/
├── docker-compose.yml              # Production compose: Postgres 16 + Go backend + Frontend
├── Dockerfile                      # Multi-stage Go build
├── Makefile                        # Build, test, migrate, run helpers
├── .env.example                    # Exhaustive environment configuration
├── go.mod / go.sum                 # Go module definition
├── migrations/                     # PostgreSQL SQL migrations
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── cmd/
│   └── server/
│       └── main.go                 # Application entrypoint & graceful shutdown
├── internal/
│   ├── config/                     # Typed configuration loading from env
│   ├── db/                         # PostgreSQL connection pool (pgx), models, repos
│   │   ├── db.go
│   │   ├── trades.go
│   │   ├── signals.go
│   │   ├── weights.go
│   │   └── news.go
│   ├── indicators/                 # Pure Go technical indicators
│   │   ├── rsi.go
│   │   ├── macd.go
│   │   ├── bollinger.go
│   │   ├── atr.go
│   │   ├── supertrend.go
│   │   ├── vwap.go
│   │   ├── stoch_rsi.go
│   │   └── confluence.go
│   ├── market/                     # Market data ingestion (Binance, CoinGecko, Yahoo Finance)
│   ├── news/                       # Global news fetcher (RSS & JSON feeds, sanitizers)
│   ├── llm/                        # Unified OpenAI-compatible client & prompt builder
│   │   ├── client.go
│   │   ├── prompts.go
│   │   └── structured_parser.go
│   ├── engine/                     # Trading execution, signal generator, risk engine
│   │   ├── allocator.go            # Core vs Alpha allocation & rebalancing
│   │   ├── risk_engine.go          # Drawdown limits, circuit breakers, position sizing
│   │   ├── execution.go            # Paper/live order execution
│   │   └── hedge_manager.go        # Dynamic Gold/Silver hedging
│   ├── learning/                   # Adaptive learning & weight manipulation
│   │   ├── optimizer.go            # Regret tracking & weight updates
│   │   ├── attribution.go          # Root cause attribution for winning/losing trades
│   │   └── dataset_exporter.go     # Fine-tuning JSONL exporter
│   └── api/                        # HTTP REST + WebSocket handlers (Fiber or Gin/Chi)
│       ├── router.go
│       ├── handlers_market.go
│       ├── handlers_trades.go
│       ├── handlers_ai.go
│       └── websocket.go
└── web/                            # React + TypeScript Frontend
    ├── package.json
    ├── vite.config.ts
    ├── tailwind.config.js
    ├── index.html
    └── src/
        ├── components/             # Reusable UI (Cards, Badges, Charts, Modals)
        ├── context/                # ThemeContext (Dark/Light), WebSocketContext
        ├── pages/
        │   ├── Dashboard.tsx       # Hero P&L, Active Positions, Core/Alpha breakdown
        │   ├── ChartView.tsx       # Live TradingView Lightweight Chart + Indicators
        │   ├── SignalsView.tsx     # AI Signals with Confluence & Indicator metrics
        │   ├── TradeHistory.tsx    # Detailed trade audit trail & P&L
        │   ├── AIWeightsView.tsx   # Dynamic Indicator Weights & Fine-tuning console
        │   └── SettingsView.tsx    # OpenAI endpoint config, risk limits
        └── services/               # Axios/Fetch API client + WebSocket client
```

---

## 3. Database Schema (PostgreSQL 16)

The schema stores all trading state, market candles, news, adaptive weights, and fine-tuning datasets:

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
    indicator_name VARCHAR(64) NOT NULL,          -- 'RSI', 'MACD', 'SUPER_TREND', 'BBANDS', 'VWAP', 'NEWS_SENTIMENT'
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
    indicator_snapshot JSONB NOT NULL,        -- Indicator values & applied weights at signal time
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',-- 'OPEN', 'EXECUTED', 'CANCELLED', 'EXPIRED'
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
    exit_reason VARCHAR(64),                   -- 'TP_HIT', 'SL_HIT', 'CIRCUIT_BREAKER', 'EXPIRED'
    root_cause VARCHAR(128),                   -- Cause attribution: 'FALSE_BREAKOUT', 'OVERBOUGHT_REVERSAL', etc.
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',-- 'OPEN', 'CLOSED'
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

## 4. Technical Indicators Engine (Pure Go)

All indicators are implemented with zero external C-dependencies, utilizing high-performance vectorized slicing in Go:

| Indicator | Implementation Logic | Market Purpose |
|---|---|---|
| **RSI (14)** | Wilder's smoothing with gain/loss tracking | Overbought (>70) & Oversold (<30) momentum gauge |
| **MACD (12, 26, 9)** | Fast/Slow EMA differential + Signal EMA line | Trend direction & momentum crossover signals |
| **Bollinger Bands (20, 2.0)** | SMA(20) ± 2 * Standard Deviation, Bandwidth, %B | Mean reversion & breakout volatility bands |
| **ATR (14)** | True Range smoothed EMA | Dynamic volatility calculation & Stop-Loss/Take-Profit setting |
| **SuperTrend (10, 3.0)** | Median price ± (Multiplier * ATR) trailing bands | Definite regime filter (Bullish/Bearish trend switch) |
| **VWAP** | Cumulative (Typical Price * Volume) / Cumulative Volume | Intraday institutional benchmark price level |
| **Stochastic RSI (14, 14, 3, 3)** | RSI normalized over Stoch period with %K & %D smoothing | High sensitivity momentum turning point detection |
| **Confluence Engine** | Weighted sum of normalized indicator sub-scores * dynamic weights | Final unified signal conviction score (0.0 to 1.0) |

---

## 5. Unified OpenAI-Compatible Intelligence & Fine-Tuning Architecture

### 5.1 OpenAI-Compatible Client
The Go backend implements an HTTP client compatible with OpenAI's Chat Completion specification:
- Endpoint: `${AI_BASE_URL}/chat/completions` (default: `https://api.openai.com/v1`, configurable to any proxy or local model).
- Header: `Authorization: Bearer ${AI_API_KEY}`.
- Payload: `model: ${AI_MODEL_ID}`, `messages: [...]`, `response_format: {"type": "json_object"}`, `temperature: 0.2`.
- Fallback: Graceful deterministic fallback when no key is set or the API is unreachable (Heuristic Confluence Mode).

### 5.2 Dynamic Indicator Weight Manipulation & Adaptive Learning
1. **Regime Detection**: System detects market regime (`BULL`, `BEAR`, `VOLATILE`, `RANGING`) based on ATR ratio and EMA slope.
2. **Trade Outcome Feedback**:
   - When a trade closes with profit, indicators that aligned with the winning direction receive a weight boost ($W_{new} = W_{old} + \eta \cdot PnL$).
   - When a trade hits Stop-Loss, indicators that falsely signaled the entry are penalized with regret attribution ($W_{new} = W_{old} - \gamma \cdot |PnL|$).
   - Weights are bounded $[0.2, 3.0]$ and decayed towards $1.0$ over time to avoid overfitting.
3. **In-Context Learning Prompt**:
   - The LLM prompt injects:
     - Real-time technical indicators + current learned weights.
     - Top winning patterns for this asset over the last 30 days.
     - Top penalized failure causes to guard against (e.g., *"Warning: RSI has high loss rate in current volatile regime, weight reduced to 0.45"*).
4. **Fine-Tuning Dataset Generation**:
   - Every completed trade generates a structured JSONL record:
     - `system`: Professional quantitative trading analyst instructions.
     - `user`: Market snapshot, technical indicators, news context, and asset regime.
     - `assistant`: Optimal signal decision, stop loss, take profit, and post-trade reflection on what worked or failed.
   - Downloadable via `/api/ai/fine-tune/export` for direct training of custom models.

---

## 6. Frontend Architecture (React + TypeScript + TailwindCSS)

- **Design Aesthetic**: Institutional trading terminal (clean slate, dark/light theme, high-contrast badges, crisp typography).
- **Views**:
  1. **Dashboard**: Hero metrics (Portfolio Equity, Realized P&L, 24h Return, Win Rate, Active Positions count, Core vs. Alpha Risk allocation meter).
  2. **Live Chart**: TradingView Lightweight Charts with candlestick data, volume bars, overlay EMA lines, and Bollinger Bands.
  3. **Signal Stream**: Real-time cards showing entry, TP, SL, AI reasoning summary, and indicator confluence breakdown.
  4. **Positions & History**: Active orders with live MTM P&L, closing controls, and historical trade logs with root-cause tags.
  5. **AI Weight Matrix & Fine-Tuning**: Interactive heatmap of indicator weights per asset, regret scores, weight sliders, and JSONL dataset export button.
  6. **Configuration**: Live API key / Base URL / Model-ID settings tester.

---

## 7. Quality & Production Checklist

- [x] Zero Iranian bourse code or dead dependencies remaining.
- [x] Pure Go implementation with native test suite (`go test ./...`).
- [x] PostgreSQL database driver using `pgx/v5` with connection pooling.
- [x] Real-time WebSocket streaming for price updates and trade events.
- [x] Responsive dark/light mode UI built with React 18, Vite, and Tailwind CSS.
- [x] Dockerfile and `docker-compose.yml` for single-command production deployment.
