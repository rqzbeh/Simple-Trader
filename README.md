# Simple-Trader • Institutional Autonomous Quantitative Trading Terminal

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.24%2B%20(Zero%20CGO)-00ADD8?logo=go)
![Frontend](https://img.shields.io/badge/Frontend-React%2019%20%2B%20TypeScript%20(Bun)-f472b6?logo=bun)
![Architecture](https://img.shields.io/badge/Architecture-Host%20Nginx%20%7C%20Go%208080%20%7C%20Postgres%20%7C%20Redis-0284c7)
![Decision Core](https://img.shields.io/badge/Decision%20Core-Jev%20%2B%209Router-8b5cf6)
![Version](https://img.shields.io/badge/version-3.0.0--decision--core-purple)
![Docker](https://img.shields.io/badge/Docker-GHCR%20Prebuilt%20Backend-2496ED?logo=docker)

<p align="center">
  <b>Simple-Trader</b> is an institutional-grade, 24/7 autonomous quantitative trading terminal and Progressive Web App (PWA).
  <br />
  v3.0: <b>exactly one decision core — Jev + 9Router.</b> News gathering, indicators, and ML models are context feeders with no agency.
  Zero fallbacks. Zero silent degradation. Every failure is an explicit, named error.
</p>

[Quick Start](#-quick-start) •
[Decision Core](#-decision-core-v30) •
[Asset Universe](#-asset-universe-123-instruments) •
[Signal Pipeline](#-signal-pipeline--position-lifecycle) •
[Indicators](#-institutional-quantitative-indicators) •
[ML Stats Feeder](#-gpu-machine-learning--statistics-feeder) •
[Dynamic Config](#-dynamic-environment-configuration) •
[API Reference](#-api--sse-endpoints)

</div>

---

## 🧠 Decision Core (v3.0)

**Law**: one decision path owns entry, exit, and news impact. Everything else is data.

```
┌────────────── CONTEXT LAYER (no agency) ──────────────┐
│ news clusters · indicators · gates (as fields) ·      │
│ LSTM stats (success rates only) · ATR levels ·        │
│ funding, volatility, macro calendar                   │
│            → one State Object per cycle               │
└──────────────────────┬────────────────────────────────┘
                       ▼
        ┌── DECISION CORE: Jev + 9Router ──┐
        │  Jev first (typed, ~0.5s, cheap) │
        │  confidence < threshold          │
        │   → escalate to 9Router (final)  │
        │  free text (thesis/Telegram)      │
        │   → 9Router                      │
        └──────────────┬───────────────────┘
                       ▼
        typed decision → execution engine
        (LONG/SHORT/NO_TRADE → order side conversion)
```

**Routing rules**
- **Jev first** for entry, exit, news classification: `Choice` (LONG/SHORT/NO_TRADE, BULLISH/BEARISH/NEUTRAL/MIXED), `Noul` (exit-now, catalyst), `Score` (severity) — full probability distributions, calibrated confidence.
- **9Router** invoked when Jev confidence < `SHADOW`/routing threshold, or when text is required. Its answer is final when invoked. Route recorded per decision: `jev_direct` or `escalated`.
- **One final writer per decision.** No parallel judges, no races, no vetoes.

**Zero-fallback policy (enforced by tests)**
- Keyword sentiment lexicon: **deleted**.
- `fallbackHeuristic` (hard-coded BUY/HOLD guess, 11 call sites): **deleted**.
- Missing model, timeout, malformed answer, unreachable feeder → **explicit error** with component + reason + cycle id. Never a neutral default, never a substituted judge.

**Futures vocabulary**: the core speaks position intent only (`LONG`/`SHORT`/`NO_TRADE`). Order sides (`BUY`/`SELL`) exist only in the execution layer — short entry = SELL order, short exit = BUY order.

Spec: [`specs/013-jev-shadow-eval/spec.md`](specs/013-jev-shadow-eval/spec.md) (v3.0) · Research: [`docs/RESEARCH-jev-callisifer.md`](docs/RESEARCH-jev-callisifer.md)

---

## 🚀 Quick Start

```bash
git clone https://github.com/rqzbeh/Simple-Trader.git
cd Simple-Trader
cp .env.example .env
# Edit .env: TYPESAFE_API_KEY (Jev), NINEROUTER_URL/KEY (9Router), admin password, risk bounds
docker compose up -d
```

Verify:

```bash
curl http://localhost:8080/health
# {"status":"healthy","version":"3.0.0-decision-core", ...}
curl http://localhost:8080/api/v1/assets | jq '.assets | length'
```

---

## 🌐 Asset Universe (123 Instruments)

Single source of truth: `internal/market/assets.go`, served via `GET /api/v1/assets`.

**CORE (10 commodities)**: PAXG, XAU (GOLD) · XAG (SILVER) · COPPER · XPT/XPD (PLATINUM/PALLADIUM) · OIL/BRENT/NG (energy, Yahoo Finance `CL=F`/`BZ=F`/`NG=F`).

**ALPHA (113 cryptocurrencies)**: every non-stablecoin CoinMarketCap top-200 listing on all four Binance, KuCoin, CoinEx, Kraken (verified against public market APIs). Correlated exposure groups block duplicate risk (PAXG signal blocks new XAU signal). Order sizes from live `LOT_SIZE` steps; display decimals from live price scale.

---

## 📡 Signal Pipeline & Position Lifecycle

Two-sided (LONG/SHORT), **news-catalyst-first**: no catalyst, no trade — technicals alone never trigger entries. The core consumes both; neither decides alone.

```
Live ticks ─┐
News crawler ─┴─▶ Context layer: one State Object (candles, indicators, cluster labels,
                   LSTM stats, ATR levels, gates-as-fields)
                        │
                        ▼
              DECISION CORE (Jev → [escalate] → 9Router)
                        │  typed: LONG / SHORT / NO_TRADE
                        ▼
              FuturesTradeSignal (PostgreSQL)
                        │
          ┌─────────────┴─────────────┐
          ▼                           ▼
   ExecutionEngine            SSE + Telegram broadcast
   (order-side conversion)    (entry + resolution events)
          │
          ▼
   Exit cycle: core judges exit-now (Jev → escalate),
   ATR stop levels are context, not triggers
          │
          ▼
   Position closed → margin released → Thompson posterior updated
```

Invariants:
- **Signals = positions.** Every ACTIVE signal has a matching engine position; rehydration restores pairing from PostgreSQL on startup.
- **ATR bounds are context.** SL = `1.5 × NATR`, TP = `3.0 × NATR` from live snapshot; the core receives them as data and decides.
- **Sizing integrity.** Position size from clamped margin; persisted allocation never disagrees with engine.
- **Concurrency guard.** `MAX_CONCURRENT_SIGNALS` (default 5) + correlated-commodity blocking.
- **Manual close immediate.** Closing a signal closes the engine position in-request.

---

## 📊 Institutional Quantitative Indicators (context feeders)

Computed from authentic exchange candles, zero synthetic data: Garman-Klass & Parkinson volatility estimators, Kaufman Efficiency Ratio, Chaikin Money Flow, RSI, MACD, Bollinger Bands, SuperTrend, EMA, VWAP, confluence. **Their outputs are fields in the State Object — they never decide.**

---

## 📡 Multi-Source Live Exchange Feed & SSE

Public exchange REST/WS feeds, Redis microsecond ticker cache, indicator snapshot storage, SSE broadcast with full hydration burst on connect (no cold-start `$0` prices). Dual serialization accepts `change24h`/`change_24h`.

---

## 📅 Macroeconomic Calendar & Trading Halts

- Live institutional feed (`ECONOMIC_CALENDAR_URL`, default ForexFactory JSON), refreshed 30 min.
- New entries halt within $[T_{event} - W, T_{event} + W]$ around high-impact events (`CALENDAR_HALT_MINUTES`, default 15) — a hard safety gate expressed as a state field.

---

## 🧠 GPU Machine Learning → Statistics Feeder

- Local NVIDIA RTX 2060, PyTorch CUDA: `ml/train_gpu.py` (LSTM), `ml/train_max_acc.py` (Residual MLP + Attention), `ml/train_multi_asset.py` (cross-asset, ~54% out-of-sample directional accuracy).
- **v3.0 role**: historical fact only — *indicator X settled successful 63% of time in regime Y*, most impactful features. Emitted as labeled context fields. **It never votes, weights, or blocks.**
- Adaptive Thompson Sampling: realized outcomes update Beta posteriors per indicator (RSI, MACD, SuperTrend, Microstructure, CMF, KER) → statistics, also context.

---

## ⚙️ Dynamic Environment Configuration

All parameters via `.env` — zero hardcoding:

```env
# Server
PORT=8080
DATABASE_URL=postgres://trader:REDACTED_DB_PASSWORD@localhost:5432/simple_trader?sslmode=disable
REDIS_URL=redis://localhost:6379/0

# Decision Core
TYPESAFE_API_KEY=your_typesafe_key        # Jev (System One)
NINEROUTER_URL=https://your-gateway/v1    # 9Router (OpenAI-compatible)
NINEROUTER_KEY=your_gateway_key
ROUTING_CONFIDENCE_THRESHOLD=0.75         # below → 9Router escalation
AI_TIMEOUT_SECONDS=30

# Risk
INITIAL_CAPITAL=100000.0
MAX_RISK_PER_TRADE_PCT=0.02
MAX_DRAWDOWN_LIMIT_PCT=0.10
MAX_CONCURRENT_SIGNALS=5
MIN_RISK_TO_REWARD_RATIO=2.5
DEFAULT_LEVERAGE=8
CALENDAR_HALT_MINUTES=15
ADMIN_PASSWORD=your_secure_admin_password
```

**Fail-fast, no masking**: unreachable DB → bounded-time in-memory mode with explicit log; missing `TYPESAFE_API_KEY`/threshold → startup error; any feeder failure → named error in state. Nothing silently pretends to be healthy.

---

## 🛡️ Host-Managed Reverse Proxy (Nginx)

Backend + React PWA on `:8080`; host Nginx handles SSL, gzip, SSE streaming:

```nginx
upstream simple_trader_backend { server 127.0.0.1:8080; keepalive 32; }
```

Full config: [`docs/DEPLOYMENT_NGINX.md`](docs/DEPLOYMENT_NGINX.md).

---

## 📡 API & SSE Endpoints

Core routes: `GET /health` · `GET /api/v1/assets` · signals/positions/trades CRUD · `GET /api/v1/stream` (SSE) · macro calendar · screener · admin config. v3.0 adds shadow/evidence report routes (see spec-013 `contracts/`).

---

## 🧪 Testing & Evidence

- Constitution VI: TDD required — unit + integration tests per subsystem; regression shielding.
- Spec-Kit pipeline for any trading-logic change: `/speckit-specify → plan → tasks → implement → converge`.
- Constitution VIII: risk parameters only from measured distributions, gated on recorded evidence.
- Zero-fallback policy verified by chaos tests: every injected failure must surface as an explicit error (SC-007).

## 📄 License

MIT
