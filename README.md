# Simple-Trader • Institutional Autonomous Quantitative Trading Terminal

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.24%2B%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2019%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![PyTorch](https://img.shields.io/badge/PyTorch-2.4%20CUDA%2012.4%20(RTX%202060)-EE4C2C?logo=pytorch)
![Architecture](https://img.shields.io/badge/Architecture-Host%20Nginx%20%7C%20Go%208080%20%7C%20Postgres%20%7C%20Redis-0284c7)
![AI Engine](https://img.shields.io/badge/AI%20Engine-LLM%20Reasoning%20(OpenAI%20Compatible)-8b5cf6)
![Docker](https://img.shields.io/badge/Docker-GHCR%20Prebuilt%20Backend-2496ED?logo=docker)

<p align="center">
  <b>Simple-Trader</b> is an institutional-grade, 24/7 autonomous quantitative trading terminal and Progressive Web App (PWA).
  <br />
  Engineered to the Jim Simons / Renaissance Technologies standard: high-throughput Go execution, zero fake data, dynamic .env risk bounds, authentic live market feeds, institutional multi-factor indicators, Bayesian Thompson sampling, and GPU deep residual neural networks.
</p>

[Quick Start](#-quick-start) •
[Asset Catalog](#-unified-25-asset-universe) •
[Quantitative Indicators](#-institutional-quantitative-indicators) •
[Live Feeds & SSE](#-multi-source-live-exchange-feed--sse) •
[Macroeconomic Calendar](#-authentic-macroeconomic-calendar--trading-halts) •
[GPU Machine Learning](#-gpu-accelerated-machine-learning) •
[Dynamic Config](#-dynamic-environment-configuration-zero-hardcoding) •
[Reverse Proxy (Nginx)](#-host-managed-reverse-proxy) •
[API Reference](#-api--sse-endpoints)

<br />

[![Simple-Trader Terminal Preview](docs/assets/dashboard-preview.svg)](https://github.com/rqzbeh/Simple-Trader)

</div>

---

## 🚀 Quick Start

Simple-Trader runs as an autonomous containerized stack (PostgreSQL 16 + Redis 7 + Pure Go Backend) designed to sit cleanly behind an Nginx instance installed directly on your VPS host.

### 1. Launch with Docker Compose

```bash
# Clone the repository
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

# Configure environment variables
cp .env.example .env
# Edit .env with your credentials and AI gateway API key

# Start PostgreSQL 16, Redis 7, and Simple-Trader Go Backend
docker compose up -d
```

### 2. Verify System Health

```bash
curl http://localhost:8080/health
# Response: {"status":"healthy","version":"2.0.0-pure-go","model_id":"custom-model-id"}
```

---

## 🌐 Unified 25-Asset Universe

Simple-Trader manages an institutional balanced portfolio of 25 global assets, strictly categorized into wealth-preservation Core Commodities and tactical liquid Alpha Cryptocurrencies:

| Category | Assets | Exposure Groups | Feed Source | Contract Type |
| :--- | :--- | :--- | :--- | :--- |
| **CORE (8 Commodities)** | PAXG/USDT, XAU/USDT | GOLD | Binance Spot / Futures | Physical / Futures |
| | XAG/USDT | SILVER | Binance Futures | High-Liquidity Futures |
| | COPPER/USDT | COPPER | Binance Futures | Industrial Electrification |
| | XPT/USDT, XPD/USDT | PLATINUM, PALLADIUM | Binance Futures | Green Hydrogen / Auto Catalysts |
| | OIL/USDT | OIL | Yahoo Finance (CL=F) | WTI Crude Oil Benchmark |
| | ALU/USDT | ALUMINUM | Yahoo Finance (ALI=F) | COMEX Industrial Metal |
| **ALPHA (17 Cryptocurrencies)** | BTC/USDT, ETH/USDT, SOL/USDT, BNB/USDT | BTC, ETH, SOL, BNB | Binance Spot Batch | Top Layer-1 Liquid Coins |
| | XRP/USDT, DOGE/USDT, ADA/USDT, AVAX/USDT | XRP, DOGE, ADA, AVAX | Binance Spot Batch | High-Beta Crypto Universe |
| | SUI/USDT, LINK/USDT, DOT/USDT, NEAR/USDT | SUI, LINK, DOT, NEAR | Binance Spot Batch | DeFi / Infrastructure Tokens |
| | LTC/USDT, BCH/USDT, UNI/USDT, APT/USDT, TON/USDT | LTC, BCH, UNI, APT, TON | Binance Spot Batch | Large-Cap Liquid Crypto |

---

## 📊 Institutional Quantitative Indicators

Unlike retail trading platforms that rely solely on simple moving averages, Simple-Trader computes a deep multi-factor quantitative feature matrix directly from authentic exchange candles with zero fake data:

1. **Garman-Klass Volatility Estimator ($\sigma_{GK}$)**:
   $$\sigma^2_{GK} = 0.5 \ln\left(\frac{H}{L}\right)^2 - (2\ln 2 - 1) \ln\left(\frac{C}{O}\right)^2$$
   Provides an unbiased extreme-value volatility estimate that is $\sim 8\times$ more statistically efficient than classical close-to-close sample variance.

2. **Parkinson Volatility Estimator ($\sigma_P$)**:
   $$\sigma^2_P = \frac{\ln(H/L)^2}{4\ln 2}$$
   Captures intraday price diffusion while filtering out bid-ask bounce noise.

3. **Kaufman Efficiency Ratio (KER)**:
   $$KER = \frac{|P_t - P_{t-n}|}{\sum_{i=1}^n |P_i - P_{i-1}|}$$
   Quantifies the fractal signal-to-noise ratio. Approaching $1.0$ in persistent directional trends, and approaching $0.0$ in choppy Brownian noise. Acts as a dynamic trend conviction multiplier.

4. **Chaikin Money Flow (CMF 20)**:
   $$CLV = \frac{(Close - Low) - (High - Close)}{High - Low}, \quad CMF = \frac{\sum CLV \cdot Volume}{\sum Volume}$$
   Detects institutional accumulation ($CMF > +0.05$) versus distribution ($CMF < -0.05$).

5. **Level-2 Order Book Imbalance (OBI) & CVD Divergence**:
   Monitors bid/ask book depth imbalance and tracks Cumulative Volume Delta divergence to identify smart money absorption and aggressive buyer exhaustion.

6. **Volatility Regime Classifier**:
   Rolling $ATR_{14} / SMA(ATR_{14}, 50)$ dynamically classifies the market into `LOW_VOL_CONSOLIDATION`, `NORMAL_TRENDING`, `HIGH_VOL_CHOP`, and `VOLATILE_BREAKOUT`.

---

## ⚡ Multi-Source Live Exchange Feed & SSE

- **Decoupled High-Speed Polling**:
  - Binance Spot Batch: Up to 20 liquid crypto tickers polled in a single HTTP request.
  - Binance Futures: Precious & industrial metals (Gold, Silver, Copper, Platinum, Palladium).
  - Yahoo Finance: Energy (WTI Crude) and Industrial Aluminum polled concurrently.
- **Server-Sent Events (SSE)**:
  - Endpoint: `GET /api/v1/events`
  - Zero-Latency Push: Connected clients immediately receive a full hydration burst of all 25 cached asset quotes upon connection, eliminating `$0` cold-start delays.
  - Dual Serialization: Seamlessly supports `change24h` and `change_24h` across frontend and backend.

---

## 📅 Authentic Macroeconomic Calendar & Trading Halts

- **Live Institutional Economic Feed**:
  - Automatically fetches real-time macroeconomic releases from `https://nfs.faireconomy.media/ff_calendar_thisweek.json` with background refresh every 30 minutes.
  - Scrapes scheduled interest rate decisions (FOMC, ECB, BOE, BOJ), CPI inflation figures, Non-Farm Payrolls, and GDP reports.
- **Autonomous Trade Halts (FR-005)**:
  - Engine automatically halts opening new positions inside $[T_{event} - 15\text{min}, T_{event} + 15\text{min}]$ for any high-impact event affecting the asset's base or quote currencies (USD, EUR, etc.), preventing sudden spread blowouts and slippage spikes.

---

## 🧠 GPU-Accelerated Machine Learning

- **Local NVIDIA GeForce RTX 2060 Execution**:
  - PyTorch with CUDA 12.4 acceleration and dedicated VRAM tensor core computation.
  - Architecture: `DeepResAlphaNet` — 3-block Deep Residual LayerNorm MLP with Mish activations, Dropout (0.30), and Weight Decay ($10^{-2}$).
- **Target Engineering**:
  - Volatility-adjusted multi-horizon forward return drift ($2h, 4h, 8h$ consensus normalized by $ATR_{20}$ volatility).
  - Label smoothing ($0.05 / 0.95$) to prevent logit overconfidence on noisy financial distributions.
  - Purged Walk-Forward Cross Validation: Prevents lookahead bias across 20,000 authentic Binance candles.
- **Adaptive Thompson Sampling**:
  - Live trade outcomes update conjugate Beta posteriors for RSI, MACD, SuperTrend, and Microstructure to continuously adjust indicator confluence weights.

---

## ⚙️ Dynamic Environment Configuration (Zero Hardcoding)

All financial, risk, and architectural parameters are configured exclusively via `.env`:

```env
# Server & Database
PORT=8080
DATABASE_URL=postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable
REDIS_URL=redis://localhost:6379/0

# AI Engine Gateway
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
| `GET` | `/api/v1/events` | Real-time Server-Sent Events stream (ticks, signals, trades) |
| `GET` | `/api/v1/assets` | Enriched 25-asset catalog with live exchange pricing |
| `GET` | `/api/v1/klines` | Authentic Binance historical candlestick data |
| `GET` | `/api/v1/portfolio/summary` | Real-time Mark-to-Market portfolio equity, drawdown, and NAV |
| `GET` | `/api/v1/calendar` | Authentic live macroeconomic releases and trading halt status |
| `POST` | `/api/v1/signals/futures/decide-all` | Scan entire 25-asset portfolio for breaking news and technical confluence |
| `POST` | `/api/v1/signals/futures/decide` | Trigger single-asset quantitative AI evaluation |
| `POST` | `/api/v1/signals/futures/{id}/close` | Execute manual or algorithmic market position close |
| `GET` | `/api/v1/weights` | Current dynamic indicator weights from Thompson Sampler |
| `POST` | `/api/v1/ml/train` | Trigger local RTX 2060 GPU neural network training run |
| `GET` | `/api/v1/ml/status` | Current GPU hardware telemetry, VRAM, and model status |
| `POST` | `/api/v1/backtest/run` | Vectorized backtest and Monte Carlo bootstrap simulation |
| `GET` | `/api/v1/investors` | Unitized investor capital ledger and multi-tenant NAV |

---

## 🧪 Testing & Verification Suite

```bash
# 1. Run all Go unit and race detection tests
go test -race ./...

# 2. Run frontend TypeScript and React unit tests
cd web && bun test

# 3. Build production PWA assets
bun run build

# 4. Train local GPU alpha model on NVIDIA RTX 2060
./ml_env/bin/python ml/train_max_acc.py --symbol BTCUSDT --epochs 35
```

---

## 📄 License

MIT License. Designed and engineered for high-performance quantitative trading operations.
