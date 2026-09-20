# Simple-Trader v2.0 • Autonomous Pure Go & AI Quantitative Engine

A high-performance, autonomous 24/7 algorithmic trading engine and Progressive Web App (PWA) rewritten from the ground up in **Pure Go 1.24** and **React 18 + TypeScript + Bun**.

Designed for production deployment with unified OpenAI-compatible LLM orchestration, post-trade outcome regret minimization, dynamic indicator weight calibration, and a strict 60% Core (Gold/Silver) vs 40% Alpha (Crypto/Forex/Oil) capital allocation model.

---

## 🏛️ System Architecture

```
                                  ┌────────────────────────────────────────┐
                                  │      React 18 + TypeScript Bun PWA     │
                                  │  (TradingView Charts, SSE, Dark Mode)  │
                                  └───────────────────▲────────────────────┘
                                                      │
                                           Nginx Alpine Reverse Proxy
                                         (/api/ & SSE /api/v1/events)
                                                      │
                                  ┌───────────────────▼────────────────────┐
                                  │      Simple-Trader Engine (Go 1.24)    │
                                  │   (Zero-CGO, Chi Mux, Goroutine Pool)  │
                                  └────────▲───────────────▲───────────────┘
                                           │               │
                 ┌─────────────────────────┴────┐     ┌────┴────────────────────────┐
                 ▼                              ▼     ▼                             ▼
   ┌───────────────────────────┐ ┌───────────────────────────┐ ┌───────────────────────────┐
   │  PostgreSQL 16 Datastore  │ │   Redis 7 Cache & PubSub  │ │  Unified OpenAI-Compatible  │
   │ (Trades, Signals, Weights)│ │ (Live Tickers, Snapshots) │ │ (OpenAI, Groq, Ollama, vLLM)│
   └───────────────────────────┘ └───────────────────────────┘ └───────────────────────────┘
```

---

## ✨ Key Highlights & Features

1. **Pure Go 1.24 Architecture (Zero Python, Zero CGO)**:
   - High-throughput mathematical implementations of technical indicators: **RSI** (Wilder's smoothing), **MACD** (EMA-12, EMA-26, Signal-9), **Bollinger Bands** (SMA-20, ±2.0 StdDev), **SuperTrend** (ATR-10, Multiplier 3.0), **VWAP**, and **Confluence Scoring**.
   - Thread-safe event bus and Server-Sent Events (SSE) broadcaster streaming real-time prices, signals, and portfolio metrics directly to connected web clients.

2. **Liquid Global Asset Universe (Purged Legacy Assets)**:
   - Completely purged of local illiquid exchanges and non-standard assets.
   - **Core Allocation (60% Target)**: Capital preservation commodities — Gold (`XAU/USD`), Silver (`XAG/USD`).
   - **Alpha Allocation (40% Target)**: High-growth momentum assets — Crypto (`BTC/USD`, `ETH/USD`, `SOL/USD`), Forex (`EUR/USD`), Commodities (`WTI/USD`).

3. **Unified OpenAI-Compatible AI Engine**:
   - Supports any standard `/v1/chat/completions` provider via simple environment configuration: **OpenAI**, **Groq**, **vLLM**, **Ollama**, or **OpenRouter**.
   - Dynamic prompt injection feeding live price action, trend regimes, and calibrated weights.
   - Resilient heuristic fallback ensures continuous operation even when API keys are absent or endpoints time out.

4. **Continuous Adaptive Learning & Dynamic Indicator Weighting**:
   - Post-trade outcome analyzer attributes trade performance and dynamically adjusts indicator weights bounded within $[0.20, 3.00\text{x}]$.
   - Winning strategies earn reinforcement boosts; false signals trigger regret penalties decaying back toward baseline $1.0\text{x}$.
   - Exports formatted ChatML JSONL training pairs (`/api/v1/learning/dataset.jsonl`) for offline model fine-tuning and distillation.

5. **Institutional Risk Controls & Capital Allocation**:
   - Strict portfolio peak-to-trough drawdown circuit breaker (halts trading if drawdown exceeds 10%).
   - Fixed fractional position sizing (default 2% risk per trade).
   - Core/Alpha rebalancing engine enforcing capital limits across all market conditions.

6. **Progressive Web App (PWA) with Bun & Nginx**:
   - Built with Bun 1.2 and Vite PWA plugin (`vite-plugin-pwa`) featuring service worker offline precaching (`sw.js`) and web app manifest.
   - Interactive TradingView Lightweight Charts, real-time indicator heatmaps, and seamless Dark/Light theme switching.

---

## 🚀 Quick Start with Docker Compose

Deploy the entire production stack (Postgres 16, Redis 7, Go Engine, React PWA) in a single command:

```bash
# Clone the repository
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

# Configure environment variables (optional overrides)
cp .env.example .env

# Launch all 4 services in detached mode
docker compose up -d
```

Once started, access the interfaces:
- **PWA Web App & Trading Terminal**: [http://localhost](http://localhost) (or port 3000)
- **Backend API & SSE Stream**: [http://localhost:8080](http://localhost:8080)
- **Health Check**: [http://localhost:8080/health](http://localhost:8080/health)
- **PostgreSQL 16 Database**: `localhost:5432` (`trader`/`trader_secret`)
- **Redis 7 Cache**: `localhost:6379`

To view logs:
```bash
docker compose logs -f
```

---

## 🛠️ Local Development Setup

### Prerequisites
- **Go**: 1.24+
- **Bun**: 1.2+
- **PostgreSQL**: 16+ (optional for in-memory dev mode)
- **Redis**: 7+ (optional for in-memory dev mode)

### 1. Run the Pure Go Backend

```bash
# Verify Go tests across all internal packages
go test -v ./internal/...

# Run the backend server (falls back gracefully to in-memory if DB/Redis are absent)
go run ./cmd/trader/main.go
```

The Go backend automatically starts on `:8080`.

### 2. Run the React + Bun Frontend

```bash
cd web

# Install dependencies with Bun
bun install

# Run Bun unit test suite
bun test

# Start Vite dev server
bun run dev
```

The frontend will run on [http://localhost:5173](http://localhost:5173) with hot module reloading.

### 3. Build Production PWA Assets

```bash
cd web
bun run build
```

This compiles static assets, generates the PWA service worker (`sw.js`), and produces distribution files in `web/dist/`.

---

## ⚙️ Configuration Reference

All settings can be specified via environment variables or a `.env` file:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP and SSE server port |
| `DATABASE_URL` | `postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable` | PostgreSQL 16 connection string |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis 7 in-memory cache and Pub/Sub URL |
| `AI_BASE_URL` | `https://api.openai.com/v1` | OpenAI-compatible API base endpoint |
| `AI_API_KEY` | `""` | API key (optional; system uses heuristic fallback if empty) |
| `AI_MODEL_ID` | `gpt-4o-mini` | LLM model identifier (e.g. `gpt-4o`, `llama-3.3-70b-versatile`) |
| `AI_TEMPERATURE` | `0.2` | Sampling temperature for AI decisions |
| `INITIAL_CAPITAL` | `100000.0` | Initial portfolio cash balance |
| `CORE_TARGET_PCT` | `0.60` | Target allocation for Gold & Silver (60%) |
| `ALPHA_TARGET_PCT` | `0.40` | Target allocation for Crypto, Forex & Oil (40%) |
| `MAX_DRAWDOWN_LIMIT_PCT` | `0.10` | Portfolio maximum drawdown circuit breaker threshold (10%) |
| `MAX_RISK_PER_TRADE_PCT` | `0.02` | Risk per trade limit (2%) |

---

## 📡 REST API & SSE Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Service health status, engine version, and active AI model |
| `GET` | `/api/v1/assets` | Supported asset universe (Core vs Alpha metadata) |
| `GET` | `/api/v1/weights` | Current post-trade calibrated indicator weights |
| `GET` | `/api/v1/learning/dataset.jsonl` | Continuous ChatML JSONL dataset export for fine-tuning |
| `GET` | `/api/v1/events` | Server-Sent Events (SSE) stream (`tick`, `signal`, `trade`, `halt`) |

---

## 🧪 Testing & Verification

The codebase adheres to strict Spec-Driven Development (SDD) standards with unit and integration test coverage:

```bash
# Run all Go test suites
go test -v ./internal/...

# Run Bun frontend tests
cd web && bun test

# Verify PWA build bundle
cd web && bun run build
```

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
