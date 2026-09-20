# Simple-Trader • Autonomous Quantitative Trading Engine

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.24%2B%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2018%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![Architecture](https://img.shields.io/badge/Architecture-PWA%20%7C%20Redis%20%7C%20Postgres-0284c7)
![CI/CD](https://github.com/rqzbeh/Simple-Trader/actions/workflows/ci-cd.yml/badge.svg)
![Docker Images](https://img.shields.io/badge/GHCR-Prebuilt%20Images-2496ED?logo=docker)

<p align="center">
  <b>Simple-Trader</b> is an institutional-grade, 24/7 autonomous quantitative trading platform and Progressive Web App (PWA).
  <br />
  Engineered with high-throughput Go execution, unified OpenAI-compatible AI intelligence, dynamic indicator weight calibration, and strict Core (60%) vs Alpha (40%) institutional capital risk management.
</p>

[Quick Start](#-quick-start-with-prebuilt-images) •
[Dashboard](#-terminal-dashboard) •
[Architecture](#-system-architecture) •
[Core Features](#-core-features) •
[Container Architecture](#-container-architecture--image-separation) •
[Local Development](#-local-development) •
[Configuration](#-configuration-reference) •
[API & SSE](#-api--sse-endpoints)

<br />

[![Simple-Trader Terminal Preview](docs/assets/dashboard-preview.svg)](https://github.com/rqzbeh/Simple-Trader)

</div>

---

## 🚀 Quick Start with Prebuilt Images

Prebuilt, production-ready container images are automatically published to the **GitHub Container Registry (GHCR)** on every validated commit to `main`:
- `ghcr.io/rqzbeh/simple-trader-backend:latest`
- `ghcr.io/rqzbeh/simple-trader-frontend:latest`

### Deploy in 30 Seconds

```bash
# 1. Download the prebuilt docker-compose definition
curl -sSL https://raw.githubusercontent.com/rqzbeh/Simple-Trader/main/docker-compose.prebuilt.yml -o docker-compose.yml

# 2. (Optional) Download example configuration
curl -sSL https://raw.githubusercontent.com/rqzbeh/Simple-Trader/main/.env.example -o .env

# 3. Pull images and start the full stack
docker compose up -d
```

### Access Ports & Services
| Component | URL / Port | Credentials / Purpose |
|---|---|---|
| **Web Dashboard (PWA)** | [http://localhost](http://localhost) (or `:3000`) | React 18 + Bun PWA Trading Terminal |
| **Backend REST & SSE** | [http://localhost:8080](http://localhost:8080) | Pure Go API & Live SSE Event Stream |
| **Health Check** | [http://localhost:8080/health](http://localhost:8080/health) | Live system readiness & model metrics |
| **PostgreSQL 16** | `localhost:5432` | Relational datastore (`trader` / `trader_secret`) |
| **Redis 7** | `localhost:6379` | High-speed cache and tick Pub/Sub broker |

---

## 🖥️ Terminal Dashboard

The web interface is a Progressive Web App (PWA) built with React 18, TypeScript, and Bun, designed for fast decision-making and continuous monitoring:

- **TradingView Lightweight Charts**: Smooth 60fps candlestick rendering with volume overlays and dynamic trendlines.
- **Dynamic AI Indicator Weight Matrix**: Real-time visualization of machine learning weight multipliers with inline fine-tuning dataset export (`.jsonl`).
- **Live SSE Event Stream**: Zero-polling, real-time push updates for market ticks, order executions, and AI signals.
- **Adaptive Theme System**: Clean dark/light theme switching with persistence.
- **Full PWA Offline Support**: Service worker precaching (`sw.js`) and installable desktop/mobile experience.

---

## 🏛️ System Architecture

<div align="center">

[![System Architecture](docs/assets/system-architecture.svg)](docs/assets/system-architecture.svg)

</div>

### Architectural Highlights
- **Pure Go Execution Engine**: Zero CGO dependencies for deterministic, sub-millisecond calculation loops and static binary compilation (`CGO_ENABLED=0`).
- **Dual-Tier Storage Strategy**:
  - **Redis 7**: Sub-millisecond tick cache, real-time indicator state snapshots, and high-speed Pub/Sub messaging.
  - **PostgreSQL 16**: Relational storage for historical candles, audit-grade trade logs, signals, and dynamic weight histories.
  - *Graceful Fallback*: The engine includes a thread-safe in-memory cache and state manager if Redis or Postgres are temporarily unavailable.
- **Unified AI Inference**: Standardized OpenAI-compatible client connecting to any LLM provider (OpenAI, Groq, vLLM, Ollama, OpenRouter) with automatic heuristic fallback.

---

## ✨ Core Features

### 1. High-Precision Quantitative Indicator Engine
Engineered from the ground up in Go with sub-millisecond compute loops:
- **RSI (Wilder's Smoothing)**: 14-period Relative Strength Index with smoothed exponential loss/gain tracking.
- **MACD (12/26/9)**: Dual exponential moving averages with signal divergence and histogram momentum.
- **Bollinger Bands ($\pm 2\sigma$)**: 20-period simple moving average with standard deviation envelope bounds.
- **SuperTrend (10, 3.0)**: Directional volatility trend tracking powered by Average True Range (ATR).
- **VWAP**: Real-time Volume Weighted Average Price benchmark calculation.
- **Confluence Scoring**: Normalized composite score ($[0.0, 1.0]$) aggregating trend, momentum, and volatility.

### 2. Institutional Global Asset Universe
Focused strictly on liquid global instruments across two disciplined buckets:
- **Core Bucket (60% Target Allocation)**: Low-volatility capital preservation commodities:
  - Gold Spot (`XAU/USD`)
  - Silver Spot (`XAG/USD`)
- **Alpha Bucket (40% Target Allocation)**: High-conviction momentum assets:
  - Major Crypto: Bitcoin (`BTC/USD`), Ethereum (`ETH/USD`), Solana (`SOL/USD`)
  - Forex & Energy: Euro (`EUR/USD`), WTI Crude Oil (`WTI/USD`)

### 3. Adaptive In-Context Learning & Regret Minimization
- **Post-Trade Attribution**: After each closed trade, the learning engine evaluates whether indicator signals correctly anticipated price movements.
- **Dynamic Weight Multipliers**: Adjusts indicator weights between $[0.20\text{x}, 3.00\text{x}]$ (rewarding accurate indicators and penalizing false signals) with decay toward neutral baseline ($1.0\text{x}$).
- **Continuous ChatML Fine-Tuning Export**: Generates validated prompt-completion pairs formatted for model distillation and fine-tuning:
  ```bash
  curl -s http://localhost:8080/api/v1/learning/dataset.jsonl -o fine_tune_dataset.jsonl
  ```

### 4. Institutional Risk Controls
- **10% Max Drawdown Circuit Breaker**: Continuously tracks peak-to-trough portfolio equity and halts all order execution if drawdown exceeds 10%.
- **Fixed Fractional Sizing**: Hard-caps risk per individual trade to 2% of total portfolio equity.
- **Dynamic Bucket Rebalancing**: Automatically adjusts position size to preserve the 60% Core / 40% Alpha balance.

---

## 📦 Container Architecture & Image Separation

Simple-Trader publishes two focused, lightweight container images:
1. `ghcr.io/rqzbeh/simple-trader-backend`: Static, stripped Go binary running on Alpine Linux (`~25 MB`).
2. `ghcr.io/rqzbeh/simple-trader-frontend`: Static PWA bundle served by Nginx Alpine with gzip/brotli compression and asset caching (`~20 MB`).

### Why are Frontend and Backend Separated?
- **Independent Scaling & Resource Allocation**: The frontend is purely static files served by Nginx with near-zero CPU and memory footprint, easily cached via CDNs. The backend handles continuous market data streams, goroutine pools, and database connection pooling.
- **Zero-Downtime Rolling Updates**: Frontend UI enhancements or styling updates can be deployed instantly without interrupting active trading goroutines, position monitors, or open orders.
- **Security & Attack Surface Reduction**: The frontend container contains no database credentials, API secrets, or Go toolchains. It only acts as an HTTP/SSE reverse proxy.
- **Multi-Environment Flexibility**: In production clusters (e.g. Kubernetes, Nomad), the backend can run in private VPC subnets with direct access to Postgres/Redis, while the frontend sits in a public DMZ.

> **Note on Unified Single-Image Deployments**: If your deployment architecture mandates a single all-in-one container, you can embed the compiled React bundle directly into the Go binary using Go's standard `//go:embed` directive, packaging the entire application into a single self-hosting binary.

---

## 🛠️ Local Development

### Prerequisites
- **Go**: 1.24 or higher
- **Bun**: 1.2 or higher
- **Docker & Docker Compose**: (optional for containerized databases)

### 1. Running the Go Backend
```bash
# Run unit tests across all packages
go test -v ./internal/...

# Build and start the backend service
go run ./cmd/trader/main.go
```

### 2. Running the Bun Frontend
```bash
cd web

# Install dependencies
bun install

# Run frontend tests
bun test

# Launch Vite development server with Hot Module Replacement (HMR)
bun run dev

# Compile production PWA bundle
bun run build
```

---

## ⚙️ Configuration Reference

All settings can be configured via environment variables or a `.env` file:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Backend HTTP API & SSE server port |
| `DATABASE_URL` | `postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable` | PostgreSQL 16 connection URL |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis 7 cache and Pub/Sub connection URL |
| `AI_BASE_URL` | `https://api.openai.com/v1` | OpenAI-compatible endpoint URL (OpenAI, Groq, Ollama, etc.) |
| `AI_API_KEY` | `""` | API key for LLM provider (optional; fallback heuristic active if empty) |
| `AI_MODEL_ID` | `gpt-4o-mini` | Target LLM model name |
| `AI_TEMPERATURE` | `0.2` | Sampling temperature for trading decisions |
| `INITIAL_CAPITAL` | `100000.0` | Initial simulated portfolio equity |
| `CORE_TARGET_PCT` | `0.60` | Target allocation for Gold & Silver (60%) |
| `ALPHA_TARGET_PCT` | `0.40` | Target allocation for Crypto, Forex & Oil (40%) |
| `MAX_DRAWDOWN_LIMIT_PCT` | `0.10` | Peak-to-trough circuit breaker limit (10%) |
| `MAX_RISK_PER_TRADE_PCT` | `0.02` | Maximum risk per position (2%) |

---

## 📡 API & SSE Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Service health status, database ping, and active AI model |
| `GET` | `/api/v1/assets` | Active trading universe with latest quotes and bucket metadata |
| `GET` | `/api/v1/weights` | Live adaptive indicator weights and calibration state |
| `GET` | `/api/v1/learning/dataset.jsonl` | Downloadable ChatML JSONL dataset for model fine-tuning |
| `GET` | `/api/v1/events` | High-frequency Server-Sent Events stream (`tick`, `signal`, `trade`, `halt`) |

---

## 🔄 Automated CI/CD & Image Pipeline

The repository includes a GitHub Actions workflow (`.github/workflows/ci-cd.yml`) implementing a **strict green-build gate**:
1. **Automated Verification**:
   - Compiles Go binary with zero CGO (`CGO_ENABLED=0`).
   - Executes Go backend test suites (`go test -v ./internal/...`).
   - Installs Bun dependencies and runs frontend unit tests (`bun test`).
   - Compiles production PWA build (`bun run build`).
2. **Gated Image Publication**:
   - Images are **only** built and pushed if every test passes with a 100% green status.
   - Clean, standard tags are automatically applied: `latest`, `main`, and release semver tags (`v*`).
   - Images are published directly to GitHub Container Registry (`ghcr.io`).

---

## 📄 License

Licensed under the [MIT License](LICENSE).
