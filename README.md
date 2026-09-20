# Simple-Trader v2.0 • Autonomous AI Quantitative Trading Engine

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.24%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2018%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![Architecture](https://img.shields.io/badge/Architecture-PWA%20%7C%20Redis%20%7C%20Postgres-0284c7)
![CI/CD](https://github.com/rqzbeh/Simple-Trader/actions/workflows/ci-cd.yml/badge.svg)
![Docker Images](https://img.shields.io/badge/GHCR-Prebuilt%20Images-2496ED?logo=docker)

**Simple-Trader** is an institutional-grade, 24/7 autonomous quantitative trading engine and Progressive Web App (PWA) rewritten completely from scratch in **Pure Go 1.24** and **React 18 + TypeScript (Bun)**.

Featuring unified OpenAI-compatible LLM decision-making, post-trade regret minimization, dynamic indicator weight calibration, and a strict Core (60% Gold/Silver) vs Alpha (40% Crypto/Forex/Oil) institutional allocation model.

[Quick Start](#-instant-deployment-with-prebuilt-images) •
[Architecture](#-system-architecture) •
[Features](#-core-features) •
[Local Development](#-local-development) •
[Configuration](#-environment-configuration) •
[API Reference](#-api--sse-endpoints)

</div>

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

## 🚀 Instant Deployment with Prebuilt Images

Prebuilt, multi-architecture production container images are automatically built and published on every commit via **GitHub Actions** to the **GitHub Container Registry (GHCR)**:
- `ghcr.io/rqzbeh/simple-trader-backend:latest`
- `ghcr.io/rqzbeh/simple-trader-frontend:latest`

### Option 1: Run with Prebuilt Images (No Local Building Required)

```bash
# 1. Download the prebuilt compose file
curl -sSL https://raw.githubusercontent.com/rqzbeh/Simple-Trader/main/docker-compose.prebuilt.yml -o docker-compose.yml

# 2. (Optional) Configure environment secrets
curl -sSL https://raw.githubusercontent.com/rqzbeh/Simple-Trader/main/.env.example -o .env

# 3. Pull images and launch all services
docker compose up -d
```

### Option 2: Build Locally from Source

```bash
# 1. Clone the repository
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

# 2. Build and run locally with compose
docker compose up --build -d
```

### Accessing the System
- **PWA Web Terminal**: [http://localhost](http://localhost) (or port `3000`)
- **Backend API & SSE**: [http://localhost:8080](http://localhost:8080)
- **Health Check**: [http://localhost:8080/health](http://localhost:8080/health)
- **PostgreSQL 16**: `localhost:5432` (`trader`/`trader_secret`)
- **Redis 7**: `localhost:6379`

---

## ✨ Core Features

### 1. Pure Go 1.24 Mathematical Indicator Engine
- **Zero CGO, Zero Python**: Built for raw execution speed and sub-millisecond calculation loops.
- **Full Indicator Suite**:
  - **RSI**: Relative Strength Index with Wilder's exponential smoothing.
  - **MACD**: Moving Average Convergence Divergence (EMA-12, EMA-26, Signal-9).
  - **Bollinger Bands**: 20-period SMA with upper and lower bands ($\pm 2.0\sigma$).
  - **SuperTrend**: True Range with 10-period ATR and 3.0 multiplier.
  - **ATR**: 14-period Average True Range with Wilder smoothing for volatility-adjusted stops.
  - **VWAP**: Real-time volume-weighted average benchmark.
  - **Confluence Scoring**: Normalized composite score ($[0.0, 1.0]$) synthesizing momentum, volatility, and trend regime.

### 2. Purged Legacy Assets & Exclusively Liquid Global Universe
- Completely eliminated local illiquid exchanges, scrapers, Codal disclosures, and Islamic Treasury Bonds.
- **Core Bucket (60% Target)**: Capital preservation commodities — Gold (`XAU/USD`), Silver (`XAG/USD`).
- **Alpha Bucket (40% Target)**: High-conviction momentum assets — Crypto (`BTC/USD`, `ETH/USD`, `SOL/USD`), Forex (`EUR/USD`), Commodities (`WTI/USD`).

### 3. Unified OpenAI-Compatible AI Decision Engine
- One unified configuration for all standard `/v1/chat/completions` LLM providers:
  - **OpenAI** (`gpt-4o`, `gpt-4o-mini`, `o3-mini`)
  - **Groq** (`llama-3.3-70b-versatile`)
  - **vLLM / Ollama** (Self-hosted local models)
  - **OpenRouter** (DeepSeek, Claude, Mistral)
- Injects real-time asset market state, dynamic indicator weights, and volatility regime into prompt schemas.
- Heuristic fallback ensures 100% operational continuity even when LLM endpoints are unreachable or unconfigured.

### 4. Continuous Adaptive Learning & Dynamic Indicator Weights
- **Post-Trade Regret Minimization**: Automated analyzer attributes winning and losing trades to indicator performance.
- **Dynamic Calibration**: Calibrates indicator weights in real time between $[0.20, 3.00\text{x}]$, decaying back to baseline $1.0\text{x}$.
- **ChatML Fine-Tuning Dataset Exporter**: Continuously generates JSONL training pairs accessible at `/api/v1/learning/dataset.jsonl` or downloadable with one click from the UI for model fine-tuning and distillation.

### 5. Institutional Risk Architecture
- **Drawdown Circuit Breaker**: Halts automated order execution if portfolio equity draws down more than 10% from peak.
- **Fixed Fractional Sizing**: Limits single-trade exposure to 2% of portfolio equity.
- **Real-Time Rebalancing**: Dynamically enforces Core (60%) vs Alpha (40%) boundary limits.

### 6. React 18 + TypeScript Bun PWA
- Developed with **Bun** for rapid builds and zero Node.js footprint.
- **Progressive Web App (PWA)**: Complete offline precaching via `vite-plugin-pwa` (`sw.js`, `manifest.webmanifest`).
- **Trading Experience**: Interactive TradingView Lightweight Charts, real-time indicator heatmaps, order logs, and instant Dark/Light theme switching.

---

## 🛠️ Local Development

### Prerequisites
- **Go**: 1.24+
- **Bun**: 1.2+
- **PostgreSQL**: 16+ *(optional: Go engine features automatic in-memory fallback)*
- **Redis**: 7+ *(optional: Go engine features automatic in-memory fallback)*

### Running the Go Backend
```bash
# Run unit & integration test suites
go test -v ./internal/...

# Start backend server
go run ./cmd/trader/main.go
```

### Running the Bun Frontend
```bash
cd web

# Install dependencies
bun install

# Run frontend tests
bun test

# Launch Vite dev server
bun run dev

# Compile production PWA bundle
bun run build
```

---

## ⚙️ Environment Configuration

Configuration is loaded from environment variables or a `.env` file:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP and SSE server port |
| `DATABASE_URL` | `postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable` | PostgreSQL 16 connection URL |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis 7 cache and Pub/Sub connection URL |
| `AI_BASE_URL` | `https://api.openai.com/v1` | OpenAI-compatible endpoint URL |
| `AI_API_KEY` | `""` | API key for LLM provider (optional: uses heuristic if unset) |
| `AI_MODEL_ID` | `gpt-4o-mini` | Model name (e.g., `gpt-4o`, `llama-3.3-70b-versatile`) |
| `AI_TEMPERATURE` | `0.2` | Temperature for AI decision generation |
| `INITIAL_CAPITAL` | `100000.0` | Initial cash balance |
| `CORE_TARGET_PCT` | `0.60` | Target allocation for Gold/Silver (60%) |
| `ALPHA_TARGET_PCT` | `0.40` | Target allocation for Crypto/Forex/Oil (40%) |
| `MAX_DRAWDOWN_LIMIT_PCT` | `0.10` | Peak-to-trough circuit breaker limit (10%) |
| `MAX_RISK_PER_TRADE_PCT` | `0.02` | Risk per trade limit (2%) |

---

## 📡 API & SSE Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Health status, version, and active model ID |
| `GET` | `/api/v1/assets` | Active asset universe and metadata |
| `GET` | `/api/v1/weights` | Live calibrated indicator weights |
| `GET` | `/api/v1/learning/dataset.jsonl` | Continuous ChatML JSONL training dataset export |
| `GET` | `/api/v1/events` | Server-Sent Events stream (`tick`, `signal`, `trade`, `halt`) |

---

## 🔄 Automated CI/CD & Image Pipeline

GitHub Actions automatically runs on every push to `main`:
1. **Validation**: Runs Go unit tests, Bun test suite, and static compilation checks.
2. **Container Build**: Compiles multi-stage `Dockerfile.backend` and `Dockerfile.frontend`.
3. **Registry Publishing**: Automatically pushes versioned and `latest` tagged images to GitHub Container Registry (`ghcr.io`).

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
