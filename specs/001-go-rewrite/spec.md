# Feature Specification: Simple-Trader Go + Bun React PWA Rewrite

**Feature ID**: 001-go-rewrite  
**Specification Owner**: Antigravity / Pair Programming Agent  
**Status**: Approved & Active  
**Created**: 2026-09-20  

---

## 1. Context & Problem Statement

Simple-Trader's legacy Python implementation suffered from:
- Multi-second indicator computation and GIL bottlenecks.
- Fragile SQLite file locking and lack of in-memory caching.
- Entangled legacy modules for the Iranian bourse (`iran.py`, TSE/IFB, Codal, Eghtesad, Islamic treasury bonds) which are decoupled from global financial markets and unwanted.
- Fragmented LLM integrations with conflicting provider schemas and lack of systematic trade feedback.
- A dated Python/FastAPI HTML template dashboard lacking progressive web app (PWA) offline capabilities, modern TradingView charts, and responsive mobile/desktop installability.

## 2. User Scenarios & Acceptance Criteria

### User Scenario 1: Autonomous High-Speed Market Analysis & Confluence Scoring
- **Given** real-time OHLCV market feeds for Core (Gold, Silver) and Alpha (Crypto, Major Forex, Crude Oil),
- **When** the Go technical indicator engine resamples price bars,
- **Then** it calculates RSI (Wilder), MACD, Bollinger Bands, ATR, SuperTrend, EMA ribbon, VWAP, and Confluence score in sub-millisecond time without CGO dependencies.

### User Scenario 2: Caching & PubSub First Architecture
- **Given** incoming price ticks and computed indicators,
- **When** the system generates market updates,
- **Then** it writes to Redis 7 first for instant in-memory lookup and SSE broadcast, while persisting durable data to PostgreSQL 16.

### User Scenario 3: Unified OpenAI-Compatible Intelligence & Adaptive Learning
- **Given** an OpenAI-compatible endpoint with user-defined `AI_MODEL_ID`,
- **When** evaluating market setups,
- **Then** the LLM prompt receives current learned indicator weights, recent failure causes, and real-time news headlines.
- **And** when a trade closes with profit or loss, the system runs root-cause attribution, adjusts indicator weights dynamically, and logs structured JSONL fine-tuning pairs.

### User Scenario 4: Installable React 18 TypeScript PWA with Bun
- **Given** the user accessing the web interface,
- **When** opening on desktop or mobile,
- **Then** the application is installable as a PWA with service worker caching, instant dark/light mode toggle, TradingView Lightweight Charts, real-time SSE streaming, dynamic indicator weight sliders, and one-click JSONL export.

### User Scenario 5: Spec-Driven Single-Command Production Deployment
- **Given** the repository root,
- **When** running `docker compose up -d`,
- **Then** PostgreSQL 16, Redis 7, the Go backend service, and the Bun-built Nginx PWA frontend container launch healthy with zero manual configuration.

---

## 3. Non-Functional Requirements & Security
- **No Iranian Market Code:** Zero references to TSE, IFB, Codal, Eghtesad, or Iranian bonds.
- **Zero CGO:** Compiles with `CGO_ENABLED=0` for minimal Alpine/Scratch image sizes.
- **High Test Coverage:** Comprehensive unit and regression tests for indicators, cache, risk engine, and learning loop.
