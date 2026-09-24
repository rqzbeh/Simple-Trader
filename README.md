# Simple-Trader • Institutional Autonomous Quantitative Trading Terminal

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.24%2B%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2019%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![PyTorch](https://img.shields.io/badge/PyTorch-2.x%20CUDA%20(RTX%202060)-EE4C2C?logo=pytorch)
![Architecture](https://img.shields.io/badge/Architecture-Host%20Nginx%20%7C%20Go%208080%20%7C%20Postgres%20%7C%20Redis-0284c7)
![AI Engine](https://img.shields.io/badge/AI%20Engine-LLM%20Reasoning%20(OpenAI%20Compatible)-8b5cf6)
![Docker](https://img.shields.io/badge/Docker-GHCR%20Prebuilt%20Backend-2496ED?logo=docker)

<p align="center">
  <b>Simple-Trader</b> is an institutional-grade, 24/7 autonomous quantitative trading terminal and Progressive Web App (PWA).
  <br />
  Zero fake data. Zero hardcoded risk bounds. Dynamic .env configuration, authentic live market feeds across 100+ instruments,
  institutional multi-factor indicators, Bayesian Thompson sampling, GPU-trained alpha models, and a strict news-catalyst-first
  two-sided futures signal pipeline where every persisted signal is a live position.
</p>

[Quick Start](#-quick-start) •
[Asset Universe](#-asset-universe-123-instruments) •
[Signal Pipeline](#-signal-pipeline--position-lifecycle) •
[Quantitative Indicators](#-institutional-quantitative-indicators) •
[Live Feeds & SSE](#-multi-source-live-exchange-feed--sse) •
[Macroeconomic Calendar](#-authentic-macroeconomic-calendar--trading-halts) •
[GPU Machine Learning](#-gpu-accelerated-machine-learning) •
[Dynamic Config](#-dynamic-environment-configuration-zero-hardcoding) •
[Reverse Proxy (Nginx)](#-host-managed-reverse-proxy) •
[API Reference](#-api--sse-endpoints)

</div>

---

## 🚀 Quick Start

Simple-Trader runs as an autonomous containerized stack (PostgreSQL 16 + Redis 7 + Pure Go Backend) designed to sit cleanly behind an Nginx instance installed directly on your VPS host.

### 1. Launch with Docker Compose

```bash
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

cp .env.example .env
# Edit .env: AI gateway credentials, admin password, risk bounds

docker compose up -d
```

### 2. Verify System Health

```bash
curl http://localhost:8080/health
# {"status":"healthy","version":"2.0.0-pure-go","model_id":"your-model-id"}
```

### 3. Verify the Asset Universe

```bash
curl http://localhost:8080/api/v1/assets | jq '.assets | length'
# The catalog is served live from the backend. The frontend holds no hardcoded copy.
```

---

## 🌐 Asset Universe (123 Instruments)

The catalog lives in exactly one place — `internal/market/assets.go` — and is served to the dashboard via `GET /api/v1/assets`. Everything (dashboard grid, screener, symbol pickers) renders from this single source of truth.

**CORE (10 commodities)** — wealth preservation / inflation hedges:

| Assets | Exposure Group | Feed | Contract |
| :--- | :--- | :--- | :--- |
| PAXG/USDT, XAU/USDT | GOLD | Binance Spot / Futures | Tokenized physical / perpetual futures |
| XAG/USDT | SILVER | Binance Futures | Perpetual futures |
| COPPER/USDT | COPPER | Binance Futures | Industrial electrification |
| XPT/USDT, XPD/USDT | PLATINUM, PALLADIUM | Binance Futures | Hydrogen catalyst / automotive |
| OIL/USDT, BRENT/USDT | OIL | Yahoo Finance (`CL=F`, `BZ=F`) | WTI + Brent crude benchmarks |
| NG/USDT | NATGAS | Yahoo Finance (`NG=F`) | Natural gas futures |
| ALU/USDT | ALUMINUM | Yahoo Finance (`ALI=F`) | COMEX industrial metal |

**ALPHA (113 cryptocurrencies)** — every non-stablecoin from the CoinMarketCap top 200 that lists on **all four** of Binance, KuCoin, CoinEx, and Kraken spot (verified programmatically against each exchange's public market listing API). Correlated exposure groups prevent duplicate risk on identical underlyings (e.g. a PAXG signal blocks a new XAU signal).

Order sizes derive from Binance `LOT_SIZE` step sizes; price display decimals derive from live price scale. No sizes or decimals are hardcoded.

---

## 📡 Signal Pipeline & Position Lifecycle

The signal engine is **two-sided** (LONG and SHORT) and **news-catalyst-first**: no catalyst, no trade — technicals alone never trigger entries.

```
Live ticks ──▶ News crawler ──▶ Sentiment packet (score, polarity, phrase mix)
                                   │
Screener (liquidity-qualified      ▼
universe, 12-way parallel) ──▶ AI decision (equity-research discipline):
                                   catalyst triage → bull/base/bear paths →
                                   risk gate → ATR-based SL/TP → sizing
                                   │
                                   ▼
                        FuturesTradeSignal (PostgreSQL)
                                   │
                 ┌─────────────────┴─────────────────┐
                 ▼                                   ▼
        ExecutionEngine position            SSE + Telegram broadcast
        (positions endpoint mirrors         (entry + resolution events)
         the signal exactly)
                 │
                 ▼
   Autonomous SL/TP resolution on every tick ──▶ position closed,
   margin released, Thompson Sampling posterior updated
```

Key invariants:

- **Signals = positions.** A persisted ACTIVE signal always has a matching execution-engine position; rehydration restores the pairing on startup from PostgreSQL.
- **ATR-based protective bounds.** Stop loss uses `1.5 × NATR`, take profit `3.0 × NATR` from the live indicator snapshot; AI-suggested percentages and config bounds are validated against the R:R gate (`MIN_RISK_TO_REWARD_RATIO`, default 2.5:1).
- **Sizing integrity.** Position size is derived from the final clamped margin, so the persisted allocation and the live position never disagree.
- **Concurrency guard.** Max concurrent active signals (`MAX_CONCURRENT_SIGNALS`, default 5), enforced in the scan loop after every evaluation, plus correlated-commodity blocking across both the DB and the execution engine.
- **Manual close is immediate.** Closing a signal closes the engine position in the same request — margin is released without waiting for a price tick.

---

## 📊 Institutional Quantitative Indicators

Computed from authentic exchange candles with zero synthetic data:

1. **Garman-Klass Volatility Estimator ($\sigma_{GK}$)**:
   $$\sigma^2_{GK} = 0.5 \ln\left(\frac{H}{L}\right)^2 - (2\ln 2 - 1) \ln\left(\frac{C}{O}\right)^2$$
   $\sim 8\times$ more statistically efficient than close-to-close variance.

2. **Parkinson Volatility Estimator ($\sigma_P$)**:
   $$\sigma^2_P = \frac{\ln(H/L)^2}{4\ln 2}$$
   Captures intraday diffusion while filtering bid-ask bounce.

3. **Kaufman Efficiency Ratio (KER)**:
   $$KER = \frac{|P_t - P_{t-n}|}{\sum_{i=1}^n |P_i - P_{i-1}|}$$
   Fractal signal-to-noise ratio. High KER amplifies trend conviction; low KER dampens it.

4. **Chaikin Money Flow (CMF 20)**:
   $$CLV = \frac{(Close - Low) - (High - Close)}{High - Low}, \quad CMF = \frac{\sum CLV \cdot Volume}{\sum Volume}$$
   Institutional accumulation ($CMF > +0.05$) versus distribution ($CMF < -0.05$).

5. **Level-2 Order Book Imbalance (OBI) & CVD Divergence**:
   Bid/ask depth imbalance plus cumulative volume delta divergence for smart-money absorption and buyer exhaustion.

6. **Volatility Regime Classifier**:
   Rolling $ATR_{14} / SMA(ATR_{14}, 50)$ classifies the market into `LOW_VOL_CONSOLIDATION`, `NORMAL_TRENDING`, `HIGH_VOL_CHOP`, and `VOLATILE_BREAKOUT`, damping or amplifying confluence scores accordingly.

---

## ⚡ Multi-Source Live Exchange Feed & SSE

- **Batched, concurrent polling**:
  - Binance Spot batch: all 114 USDT spot instruments in a single HTTP request.
  - Binance Futures: Gold, Silver, Copper, Platinum, Palladium perpetuals.
  - Yahoo Finance: WTI, Brent, Natural Gas, Aluminum polled concurrently.
- **Server-Sent Events (SSE)**: `GET /api/v1/events`
  - Newly connected dashboards receive a full hydration burst of all cached quotes on connect — no cold-start `$0` prices.
  - Dual serialization accepts both `change24h` and `change_24h` field spellings across frontend and backend.

---

## 📅 Authentic Macroeconomic Calendar & Trading Halts

- **Live institutional feed**: real macro releases from `ECONOMIC_CALENDAR_URL` (default: ForexFactory weekly JSON), refreshed every 30 minutes.
- **Autonomous trade halts**: new entries halt inside $[T_{event} - W, T_{event} + W]$ for high-impact events affecting the asset's base or quote currencies ($W$ = `CALENDAR_HALT_MINUTES`, default 15).

---

## 🧠 GPU-Accelerated Machine Learning

- **Local NVIDIA RTX 2060 execution**: PyTorch with CUDA acceleration.
- **Trainers**:
  - `ml/train_gpu.py` — LSTM sequence model on 1h candles, per-symbol checkpoints.
  - `ml/train_max_acc.py` — Deep Residual MLP + Self-Attention LSTM ensemble with purged walk-forward cross-validation over 5,000+ authentic candles.
  - `ml/train_multi_asset.py` — cross-asset training (BTC, ETH, SOL, BNB) for generalization; multi-asset models reach ~54% out-of-sample directional accuracy, a positive-expectancy edge under 2.5:1 R:R sizing.
- **Target engineering**: volatility-adjusted multi-horizon forward returns, label smoothing, walk-forward splits — no lookahead bias.
- **Adaptive Thompson Sampling**: realized trade outcomes update conjugate Beta posteriors for RSI, MACD, SuperTrend, Microstructure, CMF, and KER to continuously recalibrate confluence weights.

---

## ⚙️ Dynamic Environment Configuration (Zero Hardcoding)

All financial, risk, and architectural parameters are configured exclusively via `.env`. Every risk bound (`MIN_RISK_TO_REWARD_RATIO`, `DEFAULT_LEVERAGE`, `MIN/MAX_STOP_LOSS_PCT`, `MIN/MAX_TAKE_PROFIT_PCT`, `MAX_RISK_PER_TRADE_PCT`, `MAX_TRADE_MARGIN_PCT`, `MAX_CONCURRENT_SIGNALS`, screener thresholds, fees, slippage) is read from the environment at boot — with defensive fallbacks only for unset variables.

```env
# Server & Database
PORT=8080
DATABASE_URL=postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable
REDIS_URL=redis://localhost:6379/0

# AI Engine Gateway (any OpenAI-compatible endpoint)
AI_BASE_URL=https://api.openai.com/v1
AI_API_KEY=your_api_key_here
AI_MODEL_ID=your-model-id
AI_TEMPERATURE=0.2
AI_TIMEOUT_SECONDS=30
AI_REASONING_EFFORT=high

# Risk Management & Multi-Horizon 3-Tier Allocation
INITIAL_CAPITAL=100000.0
CORE_TARGET_PCT=0.45
ALPHA_TARGET_PCT=0.40
MAX_DRAWDOWN_LIMIT_PCT=0.10
MAX_RISK_PER_TRADE_PCT=0.02
MIN_RISK_PER_TRADE_PCT=0.005
KELLY_FRACTION=0.50
MAX_CONCURRENT_SIGNALS=5

# Dynamic Trading & Institutional Execution Bounds
MIN_RISK_TO_REWARD_RATIO=2.5
DEFAULT_LEVERAGE=8
MIN_STOP_LOSS_PCT=0.6
MAX_STOP_LOSS_PCT=2.5
MIN_TAKE_PROFIT_PCT=1.5
MAX_TAKE_PROFIT_PCT=8.0
MAX_TRADE_MARGIN_PCT=0.20
MAKER_FEE_RATE=0.0002
TAKER_FEE_RATE=0.0005
MAX_SLIPPAGE_PCT=0.05
IMPACT_FACTOR=0.05

# Macroeconomic Calendar & Screener Liquidity
CALENDAR_HALT_MINUTES=15
ECONOMIC_CALENDAR_URL=https://nfs.faireconomy.media/ff_calendar_thisweek.json
SCREENER_MIN_24H_VOLUME=50000000.0
SCREENER_MAX_SPREAD_BPS=10.0

# Authentication
ADMIN_PASSWORD=your_secure_admin_password
APP_SECRET=your_32_byte_secret_hex

# Telegram Signals Bot
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
```

Resilience guarantees:

- **Fail-fast database**: connection attempts, pings, migrations, and startup ledger queries are all time-bounded. An unreachable Postgres degrades to in-memory mode; it never hangs startup.
- **No typed-nil interfaces**: a nil `*db.Store` never enters an interface field, so no-DB mode cannot panic the server.

---

## 🛡️ Host-Managed Reverse Proxy (Nginx)

Simple-Trader serves both the REST API and the compiled React PWA directly on port `8080`.
The host Nginx configuration handles SSL termination, gzip compression, and SSE streaming:

```nginx
upstream simple_trader_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name your-domain-or-ip.com;

    # Real-Time SSE Stream (Zero-Buffering)
    location /api/v1/events {
        proxy_pass http://simple_trader_backend/api/v1/events;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
        proxy_read_timeout 86400s;
    }

    # REST API & Static PWA Frontend
    location / {
        proxy_pass http://simple_trader_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 📡 API & SSE Endpoints

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/health` | Service health status and active AI model identifier |
| `GET` | `/api/v1/events` | Real-time SSE stream (ticks, signals, trades, resolutions) |
| `GET` | `/api/v1/assets` | Full live asset catalog with exchange pricing (single source of truth) |
| `GET` | `/api/v1/klines` | Authentic Binance historical candlestick data |
| `GET` | `/api/v1/positions` | Open signal-driven positions with live mark-to-market PnL |
| `GET` | `/api/v1/portfolio/summary` | Mark-to-market equity, drawdown, and 3-tier breakdown |
| `GET` | `/api/v1/calendar` | Live macroeconomic releases and trading halt status |
| `GET` | `/api/v1/market/screener` | Liquidity screening results and qualified universe |
| `POST` | `/api/v1/signals/futures/decide-all` | Scan the qualified universe for catalysts (parallel) |
| `POST` | `/api/v1/signals/futures/decide` | Single-asset AI evaluation |
| `POST` | `/api/v1/signals/futures/{id}/close` | Manual or algorithmic position close |
| `GET` | `/api/v1/weights` | Dynamic indicator weights from Thompson Sampler |
| `GET` | `/api/v1/ml/status` | GPU telemetry, VRAM, and model status |
| `GET` | `/api/v1/ml/runs` | Training run history |
| `POST` | `/api/v1/ml/train` | Trigger a GPU training run |
| `POST` | `/api/v1/backtest/run` | Vectorized backtest and Monte Carlo bootstrap simulation |
| `GET` | `/api/v1/investors` | Unitized investor capital ledger and multi-tenant NAV |
| `GET` | `/api/v1/learning/dataset.jsonl` | Closed-signal fine-tuning dataset export |

---

## 🧪 Testing & Verification Suite

```bash
# 1. All Go unit tests (hermetic — no network)
go test ./...

# 2. Race detection
go test -race ./...

# 3. Live feed smoke test against real exchange APIs (opt-in)
SIMPLE_TRADER_LIVE_TEST=1 go test ./internal/market/ -run TestLiveFeedSmokeAllAssets -v

# 4. Frontend TypeScript and React unit tests
cd web && bun test

# 5. Build production PWA assets
cd web && bun run build

# 6. Train the alpha model on local GPU
python3 ml/train_multi_asset.py --epochs 200
```

CI (`ci-cd.yml`) runs backend tests, static verification, frontend tests, the PWA build, and publishes multi-arch images to GHCR on green main-branch builds.

---

## 📄 License

MIT License. Designed and engineered for high-performance quantitative trading operations. Educational research tooling — not financial advice.
