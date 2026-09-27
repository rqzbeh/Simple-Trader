# Feature Specification: Production Hardening, Real-Time Portfolio Intelligence & Telegram Robustness

**Feature Branch / Spec ID**: `007-production-hardening-realtime-signals`  
**Status**: APPROVED & IN PROGRESS  
**Target Date**: 2026-09-21  

---

## 1. Executive Summary & Problem Statement

Simple-Trader v2.0 introduces multi-horizon trading, 3-Tier capital allocations (Tier 1 Cash Buffer, Tier 2 Core Assets, Tier 3 Tactical Alpha), and autonomous AI-informed signal generation. During production evaluation and real-world deployment on VPS infrastructure (`trade.z3df1lter.uk`), several operational gaps were detected:

1. **Telegram Bot Dispatch Diagnostics**:
   - Outbound signal broadcasts through Telegram Bot API fail when the recipient chat or channel has not initiated contact (`Bad Request: chat not found`). The system needs explicit diagnostic guidance and resilient dispatch logging in PostgreSQL.
2. **Environment Synchronization in Frontend UI**:
   - The UI "Weights" tab previously displayed static defaults (`gpt-4o-mini`, `api.openai.com`) rather than reflecting the live runtime environment variables (`AI_BASE_URL`, `AI_MODEL_ID`, `AI_REASONING_EFFORT`).
3. **UI Complexity Simplification**:
   - The interface exposed redundant "Quant Suite" and manual Macro Regime configuration views. Macro allocations and risk matrices must operate autonomously under the hood without cluttering user operations.
4. **SSE Stream Resilience & Mark-to-Market Valuation**:
   - Reverse proxies (Cloudflare, Nginx) drop idle HTTP Server-Sent Event streams exceeding standard timeouts. 15-second heartbeat frames (`event: ping\ndata: keep-alive\n\n`) are required.
   - Core and Alpha equity balances must dynamically fluctuate mark-to-market as market prices update, rather than remaining static.
5. **Real-Data Machine Learning Execution**:
   - Train Deep Alpha neural models and Bayesian posteriors using CUDA on the host GPU using authentic historical Binance candles, persisting checkpoints and metrics.

---

## 2. User Stories & Acceptance Criteria

### User Story 1: Resilient Telegram Dispatch & Diagnostics
- **As a** fund operator / trader,  
- **I want** Telegram signal delivery to report clear diagnostic feedback when a recipient has not initiated contact with the bot, and to log all dispatch statuses in the database,  
- **So that** signals reliably reach my devices and I immediately know how to resolve chat configuration issues.
- **Acceptance Criteria**:
  - `POST /api/v1/telegram/test` returns actionable diagnostics explaining that Telegram requires sending `/start` to `@IUST_Trader_Bot`.
  - Signal transmissions record whether delivery succeeded or was skipped/failed in database records.

### User Story 2: Live Environment Configuration Binding in UI
- **As an** administrator configuring custom AI gateways (OmniRoute),  
- **I want** the frontend Weights tab to reflect the active `.env` configuration (`AI_BASE_URL`, `AI_MODEL_ID`, `AI_REASONING_EFFORT`),  
- **So that** I am certain which AI model and endpoint are executing decisions without manual re-entry.
- **Acceptance Criteria**:
  - Backend exposes `/api/v1/system/config` returning masked API key status, active model ID, base URL, and reasoning effort.
  - Weights tab queries `/api/v1/system/config` on load and binds inputs to the runtime values.

### User Story 3: UI Simplification
- **As an** operator,  
- **I want** the UI to focus strictly on actionable trading signals, portfolio performance, and investor accounts,  
- **So that** complex internal mathematical subsystems run autonomously in the background without redundant tabs.
- **Acceptance Criteria**:
  - "Quant Suite" tab is removed from the top navigation bar.
  - Macro Regime calculations run dynamically in the backend background daemon; manual macro overriding is retired.

### User Story 4: Persistent SSE Streams & Real-Time Mark-to-Market Valuation
- **As a** user monitoring live portfolio state,  
- **I want** the SSE stream to remain connected indefinitely through Cloudflare/Nginx proxies, and portfolio equity values to fluctuate with real-time market updates,  
- **So that** balances reflect actual mark-to-market asset valuations without page reloads.
- **Acceptance Criteria**:
  - SSE broadcaster emits 15-second keep-alive frames.
  - Price ticks update Core and Alpha asset valuations mark-to-market, recalculating total portfolio equity dynamically.

### User Story 5: Real-Data Local GPU ML Training & Artifact Deployment
- **As a** quantitative engineer,  
- **I want** deep learning models trained to peak accuracy on authentic Binance price history using local GPU CUDA,  
- **So that** predictive weights reflect real market dynamics without mockups, and can be synced to Git and VPS.
- **Acceptance Criteria**:
  - PyTorch scripts run on local NVIDIA GeForce RTX 2060 GPU with CUDA acceleration.
  - Model weights, checkpoints, and evaluation metrics are persisted in `models/`.
