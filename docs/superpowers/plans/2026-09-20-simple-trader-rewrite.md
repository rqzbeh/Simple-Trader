# Simple-Trader Full Architecture Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Completely rewrite Simple-Trader from its legacy Python codebase into an enterprise-grade quantitative trading platform featuring a compiled Go 1.24 backend, Redis 7 caching and pub/sub layer, PostgreSQL 16 persistence, unified OpenAI-compatible AI intelligence with dynamic indicator weight manipulation, and an installable React 18 + TypeScript PWA built using Bun.

**Architecture:** A concurrent Go 1.24 trading engine executes sub-millisecond indicator calculations, fetches global market data/news, and evaluates trades via a unified OpenAI-compatible LLM endpoint with adaptive in-context weights. High-throughput Redis 7 serves as the in-memory cache and pub/sub bus in front of PostgreSQL 16. A high-contrast, responsive React 18 PWA frontend connects via REST and Server-Sent Events (SSE) for live streaming, charting, weight manipulation, and fine-tuning data export.

**Tech Stack:** Go 1.24, `chi/v5`, `pgx/v5`, `go-redis/v9`, Bun 1.3+, React 18, TypeScript, Tailwind CSS, Lucide Icons, TradingView Lightweight Charts, `vite-plugin-pwa`, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-09-20-simple-trader-rewrite-design.md`

## Global Constraints

- **Backend Language:** Pure Go 1.24 with zero CGO dependencies (`CGO_ENABLED=0`).
- **Frontend Runtime & Tooling:** Bun (`bun install`, `bun run build`) — no Node.js commands in frontend build scripts.
- **Cache Architecture:** Redis 7 must be checked before accessing PostgreSQL for price tickers, active candles, indicator states, and dynamic weights.
- **Legacy Modules Excision:** Zero references to Iranian stocks, TSE/IFB, Codal, Eghtesad, or Islamic Treasury Bonds.
- **Unified AI Endpoint:** Standard OpenAI `/v1/chat/completions` API format supporting arbitrary `AI_MODEL_ID` and dynamic prompt weight injection.
- **PWA Requirement:** Valid Web App Manifest, Service Worker offline caching via `vite-plugin-pwa`, installable on mobile and desktop.

---

### Task 1: Repository Cleanup & Go Project Scaffolding

**Files:**
- Modify: Delete legacy Python files (`simple_trader/`, `tests/`, `tools/`, `main.py`, `requirements.txt`, etc.)
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load() (*config.Config, error)` returning strongly-typed configuration.

- [ ] **Step 1: Write the failing test for configuration loading**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Clearenv()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error loading defaults, got %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.DatabaseURL == "" {
		t.Errorf("expected default DatabaseURL to not be empty")
	}
	if cfg.RedisURL == "" {
		t.Errorf("expected default RedisURL to not be empty")
	}
	if cfg.AIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default AIBaseURL https://api.openai.com/v1, got %s", cfg.AIBaseURL)
	}
}
```

- [ ] **Step 2: Clean up legacy Python files and initialize Go module**

```bash
cd /home/redsnow/Simple-Trader
rm -rf simple_trader tests tools grafana simple-trader coingecko_helper.py main.py requirements.txt migrations/001_create_postgres_schema.sql
go mod init github.com/rqzbeh/simple-trader
```

- [ ] **Step 3: Verify test fails**

Run: `go test ./internal/config/...`  
Expected: Compilation failure because `internal/config` does not exist yet.

- [ ] **Step 4: Implement minimal configuration loader and main entrypoint**

Create `internal/config/config.go`:
```go
package config

import (
	"os"
)

type Config struct {
	Port         string
	DatabaseURL  string
	RedisURL     string
	AIBaseURL    string
	AIAPIKey     string
	AIModelID    string
	AITemp       string
	LogLevel     string
	IsProduction bool
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func Load() (*Config, error) {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/simple_trader?sslmode=disable"),
		RedisURL:     getEnv("REDIS_URL", "redis://localhost:6379/0"),
		AIBaseURL:    getEnv("AI_BASE_URL", "https://api.openai.com/v1"),
		AIAPIKey:     getEnv("AI_API_KEY", ""),
		AIModelID:    getEnv("AI_MODEL_ID", "gpt-4o"),
		AITemp:       getEnv("AI_TEMPERATURE", "0.2"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		IsProduction: getEnv("ENV", "development") == "production",
	}, nil
}
```

Create `cmd/server/main.go`:
```go
package main

import (
	"log"

	"github.com/rqzbeh/simple-trader/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	log.Printf("Starting Simple-Trader Go Engine on port %s", cfg.Port)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./internal/config/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: scaffold Go module, remove legacy Python code, add config loader"
```

---

### Task 2: PostgreSQL Schema Migrations & DB Store

**Files:**
- Create: `migrations/000001_init_schema.up.sql`
- Create: `migrations/000001_init_schema.down.sql`
- Create: `internal/db/db.go`
- Create: `internal/db/models.go`
- Test: `internal/db/db_test.go`

**Interfaces:**
- Consumes: `config.Config.DatabaseURL`
- Produces: `db.NewPool(ctx, dbURL) (*db.Store, error)`, `store.RecordSignal(...)`, `store.RecordTrade(...)`, `store.GetIndicatorWeights(...)`

- [ ] **Step 1: Create SQL migration files**

Create `migrations/000001_init_schema.up.sql` containing all 6 tables from the spec:
- `candles`
- `news_items`
- `indicator_weights`
- `signals`
- `trades`
- `fine_tune_records`

Create `migrations/000001_init_schema.down.sql`:
```sql
DROP TABLE IF EXISTS fine_tune_records CASCADE;
DROP TABLE IF EXISTS trades CASCADE;
DROP TABLE IF EXISTS signals CASCADE;
DROP TABLE IF EXISTS indicator_weights CASCADE;
DROP TABLE IF EXISTS news_items CASCADE;
DROP TABLE IF EXISTS candles CASCADE;
```

- [ ] **Step 2: Write failing unit test for DB Store initialization & models**

```go
// internal/db/db_test.go
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

func TestParseDatabaseURL(t *testing.T) {
	_, err := db.ValidateURL("postgres://postgres:postgres@localhost:5432/simple_trader?sslmode=disable")
	if err != nil {
		t.Fatalf("expected valid url, got error: %v", err)
	}
}

func TestSignalModelValidation(t *testing.T) {
	sig := db.Signal{
		Symbol:          "BTC/USD",
		Side:            "BUY",
		Bucket:          "ALPHA",
		EntryPrice:      65000.0,
		StopLoss:        63500.0,
		TakeProfit:      68000.0,
		Confidence:      0.88,
		ConfluenceScore: 0.92,
		Status:          "OPEN",
		CreatedAt:       time.Now(),
	}
	if err := sig.Validate(); err != nil {
		t.Errorf("expected valid signal, got: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v ./internal/db/...`  
Expected: FAIL compilation errors (`db.ValidateURL`, `db.Signal` undefined).

- [ ] **Step 4: Implement DB connection pool and models**

Install `pgx/v5`:
```bash
go get github.com/jackc/pgx/v5 github.com/jackc/pgx/v5/pgxpool
```

Create `internal/db/models.go` with domain structs (`Candle`, `NewsItem`, `IndicatorWeight`, `Signal`, `Trade`, `FineTuneRecord`) and validation methods.
Create `internal/db/db.go` with `Store` wrapping `*pgxpool.Pool` and schema bootstrapping.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./internal/db/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add migrations internal/db go.mod go.sum
git commit -m "feat(db): add PostgreSQL schema migrations and pgx store models"
```

---

### Task 3: Redis In-Memory Cache & Pub/Sub Client

**Files:**
- Create: `internal/cache/redis.go`
- Create: `internal/cache/keys.go`
- Test: `internal/cache/redis_test.go`

**Interfaces:**
- Consumes: `config.Config.RedisURL`
- Produces: `cache.New(redisURL) (*cache.Client, error)`, `client.SetTicker(ctx, symbol, ticker)`, `client.GetTicker(ctx, symbol)`, `client.Publish(ctx, channel, msg)`

- [ ] **Step 1: Write failing unit test for cache key generation and serialization**

```go
// internal/cache/redis_test.go
package cache_test

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

func TestCacheKeys(t *testing.T) {
	tickerKey := cache.TickerKey("BTC/USD")
	expectedTicker := "ticker:BTC/USD"
	if tickerKey != expectedTicker {
		t.Errorf("expected %s, got %s", expectedTicker, tickerKey)
	}

	candleKey := cache.CandleKey("ETH/USD", "1h")
	expectedCandle := "candles:ETH/USD:1h"
	if candleKey != expectedCandle {
		t.Errorf("expected %s, got %s", expectedCandle, candleKey)
	}
}

func TestTickerSerialization(t *testing.T) {
	orig := cache.TickerQuote{
		Symbol:    "BTC/USD",
		Price:     67450.25,
		Change24h: 3.42,
		High24h:   68100.0,
		Low24h:    65200.0,
		Volume:    18450.5,
		UpdatedAt: time.Now().Unix(),
	}

	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal ticker: %v", err)
	}

	var parsed cache.TickerQuote
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatalf("failed to unmarshal ticker: %v", err)
	}

	if parsed.Symbol != orig.Symbol || parsed.Price != orig.Price {
		t.Errorf("mismatch in serialized ticker values")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/cache/...`  
Expected: FAIL (`cache` package not defined).

- [ ] **Step 3: Add `go-redis` and implement Redis client and helpers**

```bash
go get github.com/redis/go-redis/v9
```

Create `internal/cache/keys.go` with key generators and data structs.
Create `internal/cache/redis.go` with `Client` wrapping `*redis.Client` providing safe fallback if Redis is temporarily unreachable.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/cache/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cache go.mod go.sum
git commit -m "feat(cache): add Redis client, key helpers, and ticker serialization"
```

---

### Task 4: High-Performance Technical Indicator Engine (Pure Go)

**Files:**
- Create: `internal/indicators/types.go`
- Create: `internal/indicators/rsi.go`
- Create: `internal/indicators/macd.go`
- Create: `internal/indicators/bollinger.go`
- Create: `internal/indicators/atr.go`
- Create: `internal/indicators/supertrend.go`
- Create: `internal/indicators/ema.go`
- Create: `internal/indicators/vwap.go`
- Create: `internal/indicators/confluence.go`
- Test: `internal/indicators/indicators_test.go`

**Interfaces:**
- Consumes: Slice of `db.Candle` (`[]float64` for Open, High, Low, Close, Volume)
- Produces: `CalculateRSI(closes, period)`, `CalculateMACD(closes, fast, slow, signal)`, `CalculateBollinger(closes, period, stdDev)`, `CalculateSuperTrend(candles, period, multiplier)`, `CalculateConfluence(snapshot, weights)`

- [ ] **Step 1: Write failing test suite for RSI, MACD, Bollinger, and SuperTrend**

```go
// internal/indicators/indicators_test.go
package indicators_test

import (
	"math"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/indicators"
)

func TestRSI(t *testing.T) {
	// Monotonically increasing closes should produce RSI approaching 100
	closes := make([]float64, 30)
	for i := range closes {
		closes[i] = float64(100 + i*5)
	}

	rsi := indicators.CalculateRSI(closes, 14)
	lastRSI := rsi[len(rsi)-1]
	if lastRSI < 95.0 {
		t.Errorf("expected RSI close to 100 on steep uptrend, got %f", lastRSI)
	}
}

func TestMACD(t *testing.T) {
	closes := make([]float64, 50)
	for i := range closes {
		closes[i] = 100.0 + math.Sin(float64(i))*10.0
	}

	res := indicators.CalculateMACD(closes, 12, 26, 9)
	if len(res.MACD) != len(closes) || len(res.Signal) != len(closes) || len(res.Histogram) != len(closes) {
		t.Errorf("expected MACD outputs to match input length")
	}
}

func TestBollingerBands(t *testing.T) {
	closes := []float64{10, 11, 10, 12, 10, 11, 10, 12, 10, 11, 10, 12, 10, 11, 10, 12, 10, 11, 10, 12}
	bb := indicators.CalculateBollinger(closes, 10, 2.0)
	if bb.Upper[len(bb.Upper)-1] <= bb.Middle[len(bb.Middle)-1] {
		t.Errorf("upper band must be greater than middle band")
	}
	if bb.Lower[len(bb.Lower)-1] >= bb.Middle[len(bb.Middle)-1] {
		t.Errorf("lower band must be less than middle band")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/indicators/...`  
Expected: FAIL (`indicators` package not implemented).

- [ ] **Step 3: Implement pure Go indicator mathematical formulas**

Implement Wilder's RSI, standard MACD with EMA smoothing, Bollinger Bands with rolling variance, ATR with True Range calculation, SuperTrend with trailing bands, VWAP, and Confluence aggregator.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/indicators/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/indicators
git commit -m "feat(indicators): implement pure Go RSI, MACD, Bollinger, ATR, SuperTrend, and Confluence"
```

---

### Task 5: Global Market Ingestion & News Deduplication

**Files:**
- Create: `internal/market/fetcher.go`
- Create: `internal/market/binance.go`
- Create: `internal/market/yahoo.go`
- Create: `internal/news/fetcher.go`
- Create: `internal/news/rss.go`
- Test: `internal/market/market_test.go`
- Test: `internal/news/news_test.go`

**Interfaces:**
- Produces: `market.FetchCandles(ctx, symbol, timeframe, limit) ([]db.Candle, error)`, `market.FetchTicker(ctx, symbol) (*cache.TickerQuote, error)`
- Produces: `news.FetchLatestNews(ctx) ([]db.NewsItem, error)`

- [ ] **Step 1: Write failing unit test for news deduplication and ticker mapping**

```go
// internal/news/news_test.go
package news_test

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/news"
)

func TestHashGeneration(t *testing.T) {
	item := news.Article{
		Source:      "CoinDesk",
		Headline:    "Bitcoin Breaks $70,000 as Institutional Inflows Surge",
		URL:         "https://coindesk.com/markets/btc-breaks-70k",
		PublishedAt: time.Now(),
	}

	hash1 := item.ComputeHash()
	hash2 := item.ComputeHash()
	if hash1 == "" || hash1 != hash2 {
		t.Errorf("expected deterministic non-empty SHA-256 hash")
	}
}

func TestAssetTagExtraction(t *testing.T) {
	tests := []struct {
		headline string
		expected string
	}{
		{"Ethereum Staking Yields Hit 6-Month High", "ETH/USD"},
		{"Solana DEX Volume Flips Ethereum", "SOL/USD"},
		{"Federal Reserve Holds Gold Reserves Steady", "XAU/USD"},
		{"WTI Crude Prices Jump on OPEC Supply Cut", "WTI/USD"},
	}

	for _, tc := range tests {
		tag := news.ExtractAssetTag(tc.headline)
		if tag != tc.expected {
			t.Errorf("headline '%s': expected tag '%s', got '%s'", tc.headline, tc.expected, tag)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/news/...`  
Expected: FAIL.

- [ ] **Step 3: Implement Market and News Ingestion**

Create Binance public client for Crypto (`BTC/USD`, `ETH/USD`, `SOL/USD`, `BNB/USD`).
Create Yahoo Finance public chart client for Gold (`XAU/USD`), Silver (`XAG/USD`), Oil (`WTI/USD`), and Forex (`EUR/USD`).
Create RSS XML parser for CoinDesk, Reuters Markets, and FXStreet feeds with SHA-256 deduplication and sentiment heuristics.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/market/... ./internal/news/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/market internal/news
git commit -m "feat(market,news): add Binance and Yahoo data fetchers with global news RSS scraper"
```

---

### Task 6: Unified OpenAI-Compatible AI Client & Dynamic Weight Manipulation Engine

**Files:**
- Create: `internal/ai/client.go`
- Create: `internal/ai/prompts.go`
- Create: `internal/learning/optimizer.go`
- Create: `internal/learning/attribution.go`
- Create: `internal/learning/dataset_exporter.go`
- Test: `internal/ai/ai_test.go`
- Test: `internal/learning/learning_test.go`

**Interfaces:**
- Consumes: `config.Config.AIBaseURL`, `AIAPIKey`, `AIModelID`
- Produces: `ai.NewClient(...)`, `client.AnalyzeMarket(ctx, snapshot, weights, news) (*ai.Decision, error)`
- Produces: `learning.OptimizeWeights(ctx, trade, outcome) error`, `learning.ExportFineTuneJSONL(ctx) ([]byte, error)`

- [ ] **Step 1: Write failing test for dynamic prompt weight injection and weight optimization**

```go
// internal/learning/learning_test.go
package learning_test

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/learning"
)

func TestWeightOptimization_WinBonus(t *testing.T) {
	currentWeight := 1.0
	pnlPercent := 4.5 // 4.5% profit
	wasAligned := true

	newWeight := learning.AdjustWeight(currentWeight, pnlPercent, wasAligned)
	if newWeight <= currentWeight {
		t.Errorf("expected weight to increase for aligned indicator on winning trade, got %f", newWeight)
	}
	if newWeight > 3.0 {
		t.Errorf("weight must not exceed maximum ceiling of 3.0, got %f", newWeight)
	}
}

func TestWeightOptimization_LossRegretPenalty(t *testing.T) {
	currentWeight := 1.5
	pnlPercent := -3.0 // 3.0% loss
	wasMisleading := true

	newWeight := learning.AdjustWeight(currentWeight, pnlPercent, !wasMisleading)
	if newWeight >= currentWeight {
		t.Errorf("expected weight to decrease for misleading indicator on losing trade, got %f", newWeight)
	}
	if newWeight < 0.2 {
		t.Errorf("weight must not drop below minimum floor of 0.2, got %f", newWeight)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/learning/...`  
Expected: FAIL (`learning` package functions not defined).

- [ ] **Step 3: Implement OpenAI client and learning engine**

Create `internal/ai/client.go`:
- Formats standard `/v1/chat/completions` request.
- Handles structured JSON parsing into `ai.Decision`.
- Transparently falls back to heuristic scoring if API key is blank or request fails.

Create `internal/learning/optimizer.go`:
- Implements `AdjustWeight(weight, pnlPct, positive)` bounded $[0.2, 3.0]$ with exponential decay towards $1.0$.
- Post-trade root-cause attribution (`attribution.go`).
- Continuous JSONL fine-tuning record generator (`dataset_exporter.go`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/ai/... ./internal/learning/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai internal/learning
git commit -m "feat(ai,learning): implement unified OpenAI client, adaptive weight optimizer, and JSONL exporter"
```

---

### Task 7: Trading Execution, Risk Engine & Core/Alpha Allocator

**Files:**
- Create: `internal/risk/allocator.go`
- Create: `internal/risk/circuit_breaker.go`
- Create: `internal/trader/engine.go`
- Create: `internal/trader/paper_execution.go`
- Test: `internal/risk/risk_test.go`
- Test: `internal/trader/trader_test.go`

**Interfaces:**
- Produces: `allocator.ComputeAllocation(portfolio, marketRegime)`, `trader.NewEngine(...)`, `engine.ProcessCycle(ctx)`

- [ ] **Step 1: Write failing test for Core/Alpha risk allocation and drawdown circuit breaker**

```go
// internal/risk/risk_test.go
package risk_test

import (
	"testing"

	"github.com/rqzbeh/simple-trader/internal/risk"
)

func TestCoreAlphaAllocation(t *testing.T) {
	alloc := risk.ComputeTargetAllocation(100000.0) // $100k portfolio
	if alloc.CoreUSD < 45000.0 || alloc.CoreUSD > 55000.0 {
		t.Errorf("expected ~50%% Core allocation (Gold/Silver), got %f", alloc.CoreUSD)
	}
	if alloc.AlphaUSD < 45000.0 || alloc.AlphaUSD > 55000.0 {
		t.Errorf("expected ~50%% Alpha allocation (Crypto/Forex/Oil), got %f", alloc.AlphaUSD)
	}
}

func TestCircuitBreaker_DailyLoss(t *testing.T) {
	cb := risk.NewCircuitBreaker(0.05) // 5% daily drawdown threshold
	cb.RecordLoss(100000.0, 6000.0)    // 6% drawdown
	if !cb.IsTriggered() {
		t.Errorf("circuit breaker should trigger when daily loss exceeds 5%%")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/risk/...`  
Expected: FAIL.

- [ ] **Step 3: Implement Risk Engine and Paper Execution**

Create `internal/risk/allocator.go` ensuring Gold/Silver preserve capital and hedge alpha risk.
Create `internal/risk/circuit_breaker.go` protecting capital against flash crashes or consecutive losses.
Create `internal/trader/engine.go` and `paper_execution.go` simulating fills with slippage, managing open positions, trailing stops, and triggering post-trade learning on position exit.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/risk/... ./internal/trader/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/risk internal/trader
git commit -m "feat(risk,trader): add Core/Alpha allocator, circuit breaker, and paper execution engine"
```

---

### Task 8: REST API & Real-Time SSE Broadcaster

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/handlers_market.go`
- Create: `internal/api/handlers_trades.go`
- Create: `internal/api/handlers_ai.go`
- Create: `internal/api/handlers_learning.go`
- Create: `internal/api/sse.go`
- Modify: `cmd/server/main.go`
- Test: `internal/api/api_test.go`

**Interfaces:**
- Produces: `http.Handler` with endpoints:
  - `GET /api/v1/health`
  - `GET /api/v1/market/tickers`
  - `GET /api/v1/market/candles?symbol=...&timeframe=...`
  - `GET /api/v1/trades/active`
  - `GET /api/v1/trades/history`
  - `POST /api/v1/trades/close/:id`
  - `GET /api/v1/ai/weights`
  - `POST /api/v1/ai/weights` (manual overrides)
  - `GET /api/v1/learning/dataset.jsonl` (fine-tuning export)
  - `GET /api/v1/stream/events` (Server-Sent Events)

- [ ] **Step 1: Install `chi/v5` and write failing API test**

```bash
go get github.com/go-chi/chi/v5 github.com/go-chi/cors
```

```go
// internal/api/api_test.go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/api"
)

func TestHealthEndpoint(t *testing.T) {
	router := api.NewRouter(nil, nil, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/api/...`  
Expected: FAIL (`api.NewRouter` undefined).

- [ ] **Step 3: Implement API routes, handlers, and SSE event streaming**

Create router with CORS, logging middleware, error recovery, REST routes, and real-time SSE stream for ticks, signals, and trades.
Wire up `cmd/server/main.go` to bind DB, Cache, Engine, and HTTP server with graceful shutdown.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/api/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api cmd/server/main.go go.mod go.sum
git commit -m "feat(api): add Chi REST router, SSE real-time event streaming, and server integration"
```

---

### Task 9: React + TypeScript PWA Frontend Scaffolding with Bun

**Files:**
- Create: `web/package.json`
- Create: `web/bun.lockb`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tailwind.config.js`
- Create: `web/postcss.config.js`
- Create: `web/index.html`
- Create: `web/public/manifest.webmanifest`
- Create: `web/public/favicon.svg`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/index.css`

**Interfaces:**
- Uses: Bun 1.3+ package manager (`bun install`, `bun run build`, `bun run dev`)
- Produces: Production PWA build in `web/dist` with Service Worker registration and Web Manifest.

- [ ] **Step 1: Initialize Vite React TypeScript project using Bun**

```bash
cd /home/redsnow/Simple-Trader
mkdir -p web
cd web
bun init -y
bun add react react-dom lucide-react clsx tailwindcss-animate lightweight-charts
bun add -d typescript @types/react @types/react-dom vite @vitejs/plugin-react vite-plugin-pwa tailwindcss postcss autoprefixer
```

- [ ] **Step 2: Configure Tailwind CSS, PostCSS, and Vite with PWA plugin**

Create `web/vite.config.ts` with `VitePWA`:
- Service worker caching for offline access
- Manifest configuration with theme colors (`#0f172a` dark, `#ffffff` light)
- Icons and standalone display mode

Create `web/tailwind.config.js` enabling `darkMode: 'class'`.
Create `web/public/manifest.webmanifest` and application icon.

- [ ] **Step 3: Build frontend with Bun to verify scaffolding passes**

Run: `cd /home/redsnow/Simple-Trader/web && bun run build`  
Expected: SUCCESS generating `web/dist/` including `sw.js` and `manifest.webmanifest`.

- [ ] **Step 4: Commit**

```bash
git add web
git commit -m "feat(web): initialize React 18 TypeScript PWA using Bun and Tailwind CSS"
```

---

### Task 10: PWA Frontend UI Views & Real-Time Trading Experience

**Files:**
- Create: `web/src/context/ThemeContext.tsx` (Dark/Light mode)
- Create: `web/src/context/StreamContext.tsx` (SSE Real-time connection)
- Create: `web/src/services/api.ts` (Typed API client)
- Create: `web/src/components/Navbar.tsx`
- Create: `web/src/components/MetricCard.tsx`
- Create: `web/src/pages/DashboardView.tsx`
- Create: `web/src/pages/TradingChartView.tsx`
- Create: `web/src/pages/SignalsView.tsx`
- Create: `web/src/pages/PositionsView.tsx`
- Create: `web/src/pages/AIWeightsView.tsx`
- Create: `web/src/pages/SettingsView.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: Go backend REST API and SSE stream on `/api/v1/stream/events`
- Produces: Complete reactive trading UI with dark/light mode toggle, TradingView Lightweight Charts, interactive weight sliders, and one-click JSONL export.

- [ ] **Step 1: Implement Theme Context and Real-Time SSE Stream Provider**

Create `ThemeContext.tsx` with instant local storage persistence and HTML class toggling.
Create `StreamContext.tsx` maintaining connection to `/api/v1/stream/events` with automatic reconnection and event dispatching.

- [ ] **Step 2: Implement Dashboard, Chart, Signals, AI Weights, and Settings Views**

- `DashboardView.tsx`: Hero metrics (Equity, Realized PnL, Win Rate, Drawdown, Core/Alpha meter).
- `TradingChartView.tsx`: TradingView Lightweight Charts candlestick rendering with EMA/SuperTrend overlays.
- `SignalsView.tsx`: Real-time signal cards showing direction, entry, SL/TP, and confluence scores.
- `PositionsView.tsx`: Live open positions with Mark-to-Market P&L and manual close action.
- `AIWeightsView.tsx`: Dynamic indicator weights matrix heatmap, regret scores, and One-Click "Download Fine-Tuning JSONL" button.
- `SettingsView.tsx`: OpenAI endpoint configuration, API key, model-id input, and latency ping tester.

- [ ] **Step 3: Run Bun build to verify type safety and bundle output**

Run: `cd /home/redsnow/Simple-Trader/web && bun run build`  
Expected: SUCCESS with zero TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -m "feat(web): implement complete PWA dashboard, TradingView charts, AI weight matrix, and settings"
```

---

### Task 11: Production Dockerization & Single-Command Deployment

**Files:**
- Create: `Dockerfile.backend`
- Create: `Dockerfile.frontend`
- Create: `nginx.conf`
- Create: `docker-compose.yml`
- Create: `.env.example`
- Create: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Produces: `docker compose up -d` running PostgreSQL 16, Redis 7, Go Backend, and Nginx PWA Frontend.

- [ ] **Step 1: Create Dockerfiles and Nginx reverse proxy configuration**

Create `Dockerfile.backend` (multi-stage Go 1.24 build targeting lightweight Alpine/Scratch image).
Create `Dockerfile.frontend` (multi-stage Bun build serving static PWA assets through Nginx).
Create `nginx.conf` configuring PWA caching headers, gzip, and proxying `/api/` to `backend:8080`.

- [ ] **Step 2: Create Docker Compose orchestrating all 4 services**

Create `docker-compose.yml`:
- `postgres`: image `postgres:16-alpine` with health check
- `redis`: image `redis:7-alpine` with health check
- `backend`: depends on `postgres` and `redis`
- `frontend`: depends on `backend`, exposing port 8080 / 80

- [ ] **Step 3: Create Makefile and production `.env.example`**

Include standard targets: `make build`, `make test`, `make run`, `make docker-up`, `make docker-down`.

- [ ] **Step 4: Verify test suite and local builds pass**

Run: `go test -v ./...`  
Run: `cd web && bun run build`  
Expected: ALL PASS.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.backend Dockerfile.frontend nginx.conf docker-compose.yml .env.example Makefile README.md
git commit -m "feat(deploy): add production Docker Compose, multi-stage builds, and comprehensive README"
```

---

### Task 12: End-to-End System Verification & Final Polish

**Files:**
- Test: `test/e2e_test.go`

- [ ] **Step 1: Write and run end-to-end integration test**

Verify:
1. Go engine boots and connects to DB and Redis.
2. Market data ingestion computes RSI, MACD, and SuperTrend.
3. Signal is generated with in-context weights.
4. Simulated trade executes and triggers learning feedback on close.
5. Dynamic weights are adjusted and fine-tune record is created.
6. PWA frontend assets build and serve properly.

- [ ] **Step 2: Commit final verification**

```bash
git add -A
git commit -m "chore: complete end-to-end system verification and production readiness"
```
