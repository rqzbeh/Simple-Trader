# Simple-Trader Constitution

## Core Principles

### I. Production-Ready Zero-CGO Go Backend
The backend must be built using pure Go 1.24 with zero CGO dependencies (`CGO_ENABLED=0`). All mathematical formulas for technical indicators (RSI, MACD, Bollinger Bands, ATR, SuperTrend, EMA, VWAP, Confluence) must be implemented natively in Go to guarantee microsecond execution, concurrency safety, and seamless multi-platform container compilation.

### II. Redis Cache & Pub/Sub First Before PostgreSQL
All high-frequency reads (market tickers, latest candles, calculated indicator snapshots, dynamic weights) must hit Redis 7 first. PostgreSQL 16 acts as the durable relational persistence store for audit trails, executed trades, historical candles, and fine-tuning datasets. Real-time client updates must stream via Redis Pub/Sub through Server-Sent Events (SSE).

### III. Bun Runtime & React 18 TypeScript PWA
The frontend must be managed strictly using Bun 1.3+ (no Node.js in frontend scripts). The application must be a certified Progressive Web App (PWA) equipped with an offline Service Worker, Web App Manifest, responsive layout, dark/light theme switching, and TradingView Lightweight Charts.

### IV. Complete Purge of Legacy Iranian Market Dependencies
All references, modules, calculations, and data sources related to the Iranian bourse (`iran.py`, TSE, IFB, Codal, Eghtesad, Islamic treasury bonds) are strictly forbidden and permanently excised. The platform operates exclusively on liquid global markets: Core (Gold `XAU/USD`, Silver `XAG/USD`) and Alpha (Crypto `BTC/USD`, `ETH/USD`, `SOL/USD`, Forex `EUR/USD`, Oil `WTI/USD`).

### V. Unified OpenAI-Compatible Intelligence & Adaptive Learning
The AI engine connects to any OpenAI-compatible `/v1/chat/completions` endpoint with configurable `AI_MODEL_ID`. Decisions are dynamically modulated by in-context learned indicator weights. Every closed trade undergoes post-mortem attribution: winning setups reward aligned indicators; losing trades penalize misleading indicators with regret minimization. A continuous JSONL fine-tuning pipeline captures all trade contexts for custom LLM training.

### VI. Test-Driven Verification & Regression Shielding
Test-first discipline is mandatory. Every subsystem must include comprehensive unit and integration tests (indicators, cache serialization, API endpoints, risk management, trade execution, and weight optimization). Future modifications must not break existing behavior.

### VII. Spec-Driven Single-Command Deployment
The deployment must be deterministic, reproducible, and verifiable via Docker Compose orchestrating PostgreSQL 16, Redis 7, the Go backend service, and the Bun-built Nginx PWA frontend.

## Governance
This Constitution defines the architectural baseline of Simple-Trader. Any modifications to interfaces, models, or core principles must be reflected in the spec and verified with automated test suites before deployment.

**Version**: 1.0.0 | **Ratified**: 2026-09-20 | **Last Amended**: 2026-09-20
