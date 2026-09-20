# Simple-Trader • Institutional Autonomous Quantitative Trading Engine

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.24%2B%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2018%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![Architecture](https://img.shields.io/badge/Architecture-Host%20Nginx%20%7C%20Go%208080%20%7C%20Postgres%20%7C%20Redis-0284c7)
![AI Engine](https://img.shields.io/badge/AI%20Gateway-OmniRoute%20%7C%20Gemini%203.8%20Flash%20(High%20Reasoning)-8b5cf6)
![CI/CD](https://github.com/rqzbeh/Simple-Trader/actions/workflows/ci-cd.yml/badge.svg)
![Docker](https://img.shields.io/badge/Docker-GHCR%20Prebuilt%20Backend-2496ED?logo=docker)

<p align="center">
  <b>Simple-Trader</b> is an institutional-grade, 24/7 autonomous quantitative trading platform and Progressive Web App (PWA).
  <br />
  Engineered with high-throughput Go execution, 3-Tier Multi-Horizon Liquidity Allocation, Dynamic Liquid Crypto Screening ($50M+ vol / 10 bps spread), Real-Time Whale & Political Market-Mover Tracking, Multi-Tenant Investor Capital Ledger with unitized NAV accounting, and OmniRoute AI reasoning integration.
</p>

[Quick Start](#-quick-start) •
[Host Nginx Setup](#-host-managed-nginx-deployment) •
[Architecture](#-system-architecture) •
[3-Tier Allocation](#-multi-horizon-3-tier-liquidity-allocation) •
[Crypto Screener](#-dynamic-liquid-crypto-screener) •
[Whale & Politician Intelligence](#-real-time-whale-alerts--politician-trade-intelligence) •
[Investor Capital Ledger](#-investor-capital-ledger--unitized-nav) •
[OmniRoute AI Engine](#-omniroute-ai-autonomous-decision-engine) •
[API Reference](#-api--sse-endpoints)

<br />

[![Simple-Trader Terminal Preview](docs/assets/dashboard-preview.svg)](https://github.com/rqzbeh/Simple-Trader)

</div>

---

## 🚀 Quick Start

Simple-Trader runs as a streamlined, high-performance containerized stack (PostgreSQL 16 + Redis 7 + Pure Go Backend) designed to sit cleanly behind an Nginx instance installed directly on your VPS host.

### 1. Download & Launch with Docker Compose

```bash
# Clone the repository
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader

# Configure environment variables
cp .env.example .env
# Edit .env with your credentials and OmniRoute API key

# Start PostgreSQL 16, Redis 7, and Simple-Trader Go Backend
docker compose up -d
```

### 2. Verify System Health

```bash
curl -s http://127.0.0.1:8080/health | jq
```

Response:
```json
{
  "model_id": "antigravity/gemini-3.8-flash-tiered",
  "status": "healthy",
  "version": "2.0.0-pure-go"
}
```

---

## 🌐 Host-Managed Nginx Deployment

Rather than isolating Nginx inside a Docker container, Simple-Trader is designed for real-world VPS deployments where the server administrator manages Nginx directly on the host system. This allows seamless Let's Encrypt SSL/TLS management via `certbot`, custom DDoS rate-limiting, and native host integration.

The Go backend (`http://127.0.0.1:8080`) serves both the REST API, SSE streaming endpoints, and the compiled React 18 PWA frontend directly with Single-Page Application (SPA) client-side route fallback.

### Production Nginx Configuration (`/etc/nginx/sites-available/simple-trader`)

```nginx
upstream simple_trader_backend {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name trading.yourdomain.com; # Replace with your domain or VPS IP

    # High-Performance Gzip Compression
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_proxied expired no-cache no-store private auth;
    gzip_types text/plain text/css text/xml text/javascript application/javascript application/json image/svg+xml;
    gzip_disable "MSIE [1-6]\.";

    client_max_body_size 20M;

    # 1. System Health Check
    location /health {
        proxy_pass http://simple_trader_backend/health;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 2. Real-Time Server-Sent Events (SSE) Stream
    location /api/v1/events {
        proxy_pass http://simple_trader_backend/api/v1/events;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable proxy buffering for zero-latency streaming
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }

    # 3. Backend REST API
    location /api/ {
        proxy_pass http://simple_trader_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }

    # 4. Web UI (PWA) & Static Assets
    # Directly served by the Go backend with automatic SPA index.html fallback
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

Enable and reload Nginx:
```bash
sudo ln -s /etc/nginx/sites-available/simple-trader /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

---

## 🏛️ System Architecture

<div align="center">

```
  [ Internet / Traders / Mobile PWA ]
                   │
                   ▼
  ┌─────────────────────────────────────────────────────────┐
  │         Host VPS Nginx Reverse Proxy (Port 80 / 443)    │
  │     SSL Termination, Rate Limiting, SSE Passthrough     │
  └────────────────────────┬────────────────────────────────┘
                           │ Reverse Proxy to 127.0.0.1:8080
                           ▼
  ┌─────────────────────────────────────────────────────────┐
  │         Simple-Trader Autonomous Go 1.24 Core           │
  │  ┌───────────────────────┐   ┌───────────────────────┐  │
  │  │  3-Tier Liquidity     │   │ Dynamic Crypto        │  │
  │  │  Allocator Engine     │   │ Screener ($50M/10bps) │  │
  │  └───────────────────────┘   └───────────────────────┘  │
  │  ┌───────────────────────┐   ┌───────────────────────┐  │
  │  │  Whale & Politician   │   │ Investor Capital      │  │
  │  │  News Crawler (NLP)   │   │ Ledger (Unitized NAV) │  │
  │  └───────────────────────┘   └───────────────────────┘  │
  │  ┌───────────────────────┐   ┌───────────────────────┐  │
  │  │  Vectorized Backtest  │   │ Static SPA Web Asset  │  │
  │  │  & Monte Carlo Engine │   │ Direct File Server    │  │
  │  └───────────────────────┘   └───────────────────────┘  │
  └───────────────┬───────────────────────────┬─────────────┘
                  │                           │
                  ▼                           ▼
  ┌──────────────────────────────┐  ┌───────────────────────┐
  │    PostgreSQL 16 Database    │  │     Redis 7 Cache     │
  │ • Investor Capital Ledger    │  │ • Tick Stream Cache   │
  │ • Deduplicated News Archive  │  │ • Active Universe Set │
  │ • Screener Snapshots         │  │ • Pub/Sub Events      │
  └──────────────────────────────┘  └───────────────────────┘
                  │
                  ▼
  ┌─────────────────────────────────────────────────────────┐
  │       OmniRoute AI Inference Gateway (HTTP REST)        │
  │  Model: antigravity/gemini-3.8-flash-tiered             │
  │  Reasoning Effort: high                                 │
  │  Institutional Multi-Horizon Bayesian Risk Analysis     │
  └─────────────────────────────────────────────────────────┘
```

</div>

---

## ⚖️ Multi-Horizon 3-Tier Liquidity Allocation

Trading funds are governed by a disciplined, multi-horizon liquidity management model that prevents capital lockup, protects user redemption liquidity, and balances high-frequency tactical gains with macro capital preservation:

```
Total Capital ($100,000 baseline)
 │
 ├── Tier 1: Liquidity & Cash Buffer (15% Target • $15,000)
 │    └── Risk-free reserve (USD/USDC) exclusively backing investor withdrawals and margin safety
 │
 ├── Tier 2: Short-Term Tactical Alpha (40% Target • $40,000)
 │    └── 3-hour candle aggregation, order book microstructure (OBI/CVD), breakout momentum
 │
 └── Tier 3: Core Capital Preservation (45% Target • $45,000)
      └── Macro inflation hedges (XAU/USD Gold Spot, XAG/USD Silver Spot, BTC/USD Core Reserve)
```

- **Dynamic Auto-Rebalancing**: If market movements cause any tier to deviate by more than $\pm 5.0\%$ from target, the engine calculates deterministic rebalance transfers.
- **Liquidity Lock Protection**: Investor withdrawals cannot breach or force liquidation of Tier 2 alpha trades; they are satisfied directly from Tier 1 liquid cash.

---

## 🔍 Dynamic Liquid Crypto Screener

To prevent execution slippage in illiquid pairs, Simple-Trader continuously screens candidate crypto assets using institutional liquidity filters:

- **24-Hour Trading Volume Threshold**: Minimum **$50,000,000 USD** daily turnover.
- **Bid-Ask Spread Threshold**: Maximum **10.0 basis points (0.10%)** spread.
- **Active Trading Universe**: Only pairs passing both criteria simultaneously are admitted into the tactical trading universe (e.g. `BTC/USD`, `ETH/USD`, `SOL/USD`, `BNB/USD`, `XRP/USD`, `ADA/USD`, `DOGE/USD`, `AVAX/USD`, `LINK/USD`, `DOT/USD`, `NEAR/USD`, `SUI/USD`, `UNI/USD`, `ENA/USD`).
- **Live Provider**: Connects to Binance live order books and 24h ticker metrics, caching snapshots into PostgreSQL and Redis.

---

## 🐋 Real-Time Whale Alerts & Politician Trade Intelligence

Simple-Trader integrates automated financial intelligence feeds that track large-scale market manipulation, institutional accumulation, and regulatory moves:

### 1. Ingestion Sources
- **Crypto Whale Tracker**: On-chain transfer monitoring via Whale Alert, Arkham, and Lookonchain queries for massive exchange deposits and cold wallet sweeps.
- **Politician & Insider Disclosures**: Congressional trading disclosures (Capitol Trades, Pelosi trades, Senate financial filings).
- **Political Crypto Ventures**: Real-time developments around high-profile political tokens, World Liberty Financial, and legislative endorsements.
- **Mainstream & Crypto Press**: Yahoo Finance, CoinDesk, CoinTelegraph, Decrypt.

### 2. SHA-256 Deduplication
Headlines are normalized and fingerprinted with SHA-256 to prevent duplicate sentiment skew across multiple news aggregators.

### 3. Quantitative Financial NLP Lexicon
Sentiment is scored using specialized quantitative terminology and compressed via hyperbolic tangent:

$$\text{Sentiment Score} = \tanh\left(\frac{\text{Raw Score}}{\max(1.0, N \times 0.5)}\right) \in [-1.0, +1.0]$$

- **Polarity Classifications**:
  - `BULLISH` ($\ge +0.20$): Accumulation, ETF inflows, rate cuts, whale cold-wallet transfers, legislative backing.
  - `BEARISH` ($\le -0.20$): Whale dumps, exchange deposits, SEC subpoenas, insider selling, insolvency.
  - `NEUTRAL`: Balanced or non-directional newsflow.

---

## 💼 Investor Capital Ledger & Unitized NAV

For multi-tenant capital pooling, Simple-Trader implements a Wall-Street-grade unitized Net Asset Value (NAV) ledger stored durably in PostgreSQL 16:

- **Zero-Dilution NAV Accounting**:
  $$\text{NAV} = \frac{\text{Current Total Equity}}{\text{Total Pool Units Issued}}$$
- **Deposits**: Mint units proportional to current NAV:
  $$\text{Units Minted} = \frac{\text{Deposit Amount}}{\text{NAV}}$$
- **Withdrawals**: Burn units at the current NAV without diluting existing participants:
  $$\text{Units Burned} = \frac{\text{Withdrawal Amount}}{\text{NAV}}$$
- **Tier 1 Cash Buffer Gate**: The system rejects withdrawal requests exceeding the Tier 1 Cash Buffer with HTTP 400 (`withdrawal exceeds available cash buffer`), preventing forced liquidation of active tactical trading positions.

---

## 🤖 OmniRoute AI Autonomous Decision Engine

Simple-Trader connects directly to the high-performance **OmniRoute Gateway** using standard OpenAI chat completion protocols:

- **Endpoint**: `https://omniroute.z3df1lter.uk/v1`
- **Model**: `antigravity/gemini-3.8-flash-tiered`
- **Reasoning Effort**: `high`
- **Institutional System Prompt**: Deep quantitative multi-horizon analysis evaluating technical indicators (RSI, SuperTrend, MACD, Confluence), order book microstructure (OBI, CVD), economic calendar blackout windows, and real-time whale/politician sentiment headlines.
- **Structured JSON Output**:
  ```json
  {
    "decision": "BUY",
    "confidence": 0.88,
    "suggested_size_pct": 0.02,
    "suggested_stop_loss_pct": 1.25,
    "suggested_take_profit_pct": 3.75,
    "regime": "NORMAL_TRENDING",
    "estimated_win_probability": 0.72,
    "reasoning": "Strong confluence between SuperTrend bullish breakout, positive CVD absorption, and institutional spot ETF accumulation headlines."
  }
  ```

---

## 📡 API & SSE Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | System status, Go engine version, active AI model |
| `GET` | `/manifest.json` | PWA manifest for standalone mobile/desktop installation |
| `POST` | `/api/v1/auth/login` | Secure admin login with bcrypt verification and sliding-window rate limiting |
| `GET` | `/api/v1/auth/session` | Validate session token or cookie authentication state |
| `POST` | `/api/v1/auth/logout` | Invalidate session token and clear authentication cookie |
| `GET` | `/api/v1/events` | Real-time Server-Sent Events (SSE) live tick and signal stream |
| `GET` | `/api/v1/assets` | Active tradable assets with current quotes |
| `GET` | `/api/v1/signals/futures` | List active or closed two-sided trade signals (BUY/LONG & SELL/SHORT) |
| `POST` | `/api/v1/signals/futures/decide` | Trigger AI market evaluation driven by breaking news catalysts |
| `POST` | `/api/v1/signals/futures/{id}/close` | Close signal position with realized PnL and trigger Bayesian fine-tuning |
| `GET` | `/api/v1/macro/regime` | Dynamic macroeconomic regime state (Crisis / Normal / Dovish Expansion) |
| `GET` | `/api/v1/telegram/config` | Retrieve configured Telegram bot and notification settings |
| `POST` | `/api/v1/telegram/config` | Update Telegram bot token and target chat ID |
| `GET` | `/api/v1/ml/status` | Real hardware telemetry, CUDA status, and Bayesian posteriors |
| `GET` | `/api/v1/weights` | Active indicator weight multipliers (RSI, SuperTrend, MACD, etc.) |
| `GET` | `/api/v1/allocator/tiers` | 3-Tier Multi-Horizon Liquidity allocation breakdown |
| `GET` | `/api/v1/market/screener` | Dynamic crypto screener results ($50M vol / 10bps spread) |
| `GET` | `/api/v1/news/stream` | Live ingested news stream and aggregate NLP sentiment report |
| `POST` | `/api/v1/trade/decide` | On-demand AI trade decision via OmniRoute Gateway |
| `GET` | `/api/v1/investors` | List registered capital investors and unit balances |
| `POST` | `/api/v1/investors` | Register new capital investor with initial deposit |
| `GET` | `/api/v1/investors/{id}` | Retrieve investor profile, current equity, and transaction history |
| `POST` | `/api/v1/investors/{id}/deposit` | Deposit additional capital and mint pool units |
| `POST` | `/api/v1/investors/{id}/withdraw` | Withdraw capital (protected by Tier 1 cash buffer) |

---

## 🧪 Comprehensive Verification Suite

Run the end-to-end integration and verification script against your running stack:

```bash
python3 scripts/verify_e2e_pipeline.py
```

Output:
```text
================================================================
SIMPLE-TRADER V2.0 SYSTEM INTEGRATION & VERIFICATION TEST SUITE
================================================================

[TEST 1] System Health & Unified PWA Serving
 -> Backend Health: healthy | Model: antigravity/gemini-3.8-flash-tiered | Version: 2.0.0-pure-go
 -> PWA Manifest verified: Name='Simple-Trader AI Terminal', Display='standalone'

[TEST 2] Dynamic Liquid Crypto Screener ($50M Vol / 10bps Spread)
 -> Screened Assets Count: 20 | Active Universe: 11 symbols
 -> Active Universe Symbols: ['BTC/USD', 'ETH/USD', 'SOL/USD', 'BNB/USD', 'XRP/USD', 'DOGE/USD']...

[TEST 3] Real-time News Ingestion & NLP Sentiment Feed (Whale + Political + Crypto)
 -> Ingested Real-Time Articles: 100 across RSS feeds (WhaleAlert, TrumpVentures, Yahoo, CoinDesk)
 -> Aggregate Sentiment: Score=-0.08 | Polarity=NEUTRAL | Key terms matched=14

[TEST 4] Dynamic Macroeconomic Regime Allocation (3-Tier Real-World Allocation)
 -> Active Macro Regime: NORMAL (Score: 0.635)
 -> Targets: Cash=15.0% | Core=45.0% | Alpha=40.0%

[TEST 5] Investor Capital Ledger System (PostgreSQL 16 Multi-Tenant)
 -> Registered Investor ID: fd315fa8-553a... (Dr. Arash Vahid)
 -> Deposited Additional: $5,000.00 | Units Minted @ NAV=1.1656
 -> Testing Liquidity Protection: Attempting withdrawal exceeding Tier 1 Cash Buffer...
 -> Successfully REJECTED excessive withdrawal: insufficient Tier 1 liquidity reserve
 -> Successfully Executed Withdrawal: $2,500.00

[TEST 6] Telegram Signals Bot Integration & Persistence
 -> Telegram Config Saved: Token Masked=795...hrU | ChatID=3239664627 | Enabled=True

[TEST 7] Secure Authentication & Password Protection
 -> Initial Session State: Authenticated=False
 -> Admin Login Success: Token=4c99...e885
 -> Validated Authenticated Session: Masked Token=4c99...e885

[TEST 8] Two-Sided Futures Trade Signals & Bayesian Learning
 -> Signal Evaluated: Signal #1 | LONG BTC/USD 5x
    Entry: $65,000.00 | SL: $63,700.00 | TP1: $68,120.00 | R:R 1:2.40
    Capital: $21,500.00 (20.0% of Alpha Tier)
    Catalyst: "Fed surprise 50 bps rate cut paired with 12,500 BTC institutional whale accumulation"
 -> Closed Signal #1: Exit=$66,625.00 | Realized ROI=12.50%

[TEST 9] Real-Data Machine Learning & Bayesian Posteriors Telemetry
 -> Hardware: NVIDIA GeForce RTX 2060 | CUDA Enabled: True
    - MACD: α=2.0, β=2.0 (Posterior Mean: 50.0%)
    - RSI: α=2.0, β=2.0 (Posterior Mean: 50.0%)
    - SUPERTREND: α=2.0, β=2.0 (Posterior Mean: 50.0%)

[TEST 10] Live AI Trade Decision via OmniRoute Gateway
 -> Target Model: antigravity/gemini-3.8-flash-tiered
 -> Gateway Endpoint: https://omniroute.z3df1lter.uk/v1
 -> OmniRoute Live AI Call Completed in 4.63s!
 -> Decision: BUY | Confidence: 0.85 | Win Prob: 0.73
 -> Regime: BULL | Stop Loss: 1.80% | Take Profit: 4.20%

================================================================
ALL 10 END-TO-END PIPELINE VERIFICATION SUITES PASSED FLAWLESSLY!
Simple-Trader v2.0 Docker Stack is 100% Production Ready.
================================================================
```

---

## 📄 License

Simple-Trader is open-source software licensed under the [MIT License](LICENSE).
