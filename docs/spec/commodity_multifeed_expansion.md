# Specification: Commodity Expansion, Correlated Bundling & Multi-Source Price Aggregator

- **Status**: APPROVED
- **Date**: 2026-09-22
- **Author**: Simple-Trader Core Architecture Team
- **Approach**: Spec-Driven Development (SDD) & Test-Driven Development (TDD)

---

## 1. Overview & Business Requirements

Simple-Trader is an institutional-grade autonomous trading and portfolio management engine. To safeguard capital and maximize high-conviction risk-adjusted returns, the engine requires:

1. **Expanded Institutional Commodities**:
   - Expansion beyond Gold and Silver to include **Copper** (`COPPER/USDT`), **Platinum** (`XPT/USDT`), **Palladium** (`XPD/USDT`), **Crude Oil** (`OIL/USDT` / WTI), and **Aluminum** (`ALU/USDT`).
   - Categorization strictly under the `CORE` bucket (Inflation Hedges & Wealth Preservation).

2. **Correlated Commodity Exposure Bundling**:
   - Multiple instruments representing the same underlying commodity (e.g., `PAXG/USDT`, `XAUT/USDT`, and `XAU/USDT` for Gold) MUST share a common `ExposureGroup`.
   - **Single Risk Invariant**: Capital allocation and risk management shall treat the exposure group as a unified single risk budget. The engine shall NEVER open concurrent positions or generate conflicting/redundant active signals for correlated instruments within the same exposure group.
   - Cash shall not be split between correlated assets (e.g. PAXG and XAUT).

3. **Multi-Source Resilient Market Data Feeds**:
   - Redundancy and consensus across tier-1 exchanges and institutional data providers:
     - **Binance** (Dual Spot + USDT-M Futures)
     - **KuCoin** (Spot Level 1 + USDT-M Futures)
     - **CoinEx** (Spot v2 + USDT-M Futures v2)
     - **TradingView** (Crypto Scanner + CFD / Commodity Scanner)
     - **Yahoo Finance** (Macro Energy & Metals Futures: `GC=F`, `SI=F`, `HG=F`, `PL=F`, `PA=F`, `CL=F`, `ALI=F`)
   - An intelligent `MultiSourcePriceAggregator` implementing fallback failover: if primary feed fails or is rate-limited, failover to secondary, tertiary, etc., seamlessly with zero downtime and zero synthetic/mock data.

4. **Expanded Crypto Universe (Top Universally Supported Coins)**:
   - High-liquidity tier-1 crypto assets universally tradeable across major crypto exchanges:
     - `BTC/USDT`, `ETH/USDT`, `SOL/USDT`, `BNB/USDT`, `XRP/USDT`, `DOGE/USDT`, `ADA/USDT`, `AVAX/USDT`, `SUI/USDT`, `LINK/USDT`, `DOT/USDT`, `NEAR/USDT`, `LTC/USDT`, `BCH/USDT`, `UNI/USDT`, `APT/USDT`.
   - Categorization strictly under the `ALPHA` bucket.

5. **Complete Removal of Forex & Irrelevant Assets**:
   - Zero runtime queries, zero asset definitions, and zero defensive exclusion checks for `EUR/USDT`, `EURC`, `IRR`, `TSE`, or `IFB`. No wasted compute on assets Simple-Trader does not trade.

6. **Telegram Dispatch Reliability**:
   - Manual or background "Scan All Assets" batch operations must reliably dispatch real-time Telegram alerts for newly generated signals without suppression.

---

## 2. Interface Contracts & Data Models

### 2.1 Asset Definition & Exposure Groups

```go
type AssetDefinition struct {
    Symbol        string  `json:"symbol"`         // Standardized internal symbol (e.g., "COPPER/USDT")
    Name          string  `json:"name"`           // Display name (e.g. "Copper / Tether")
    Bucket        string  `json:"bucket"`         // "CORE" or "ALPHA"
    ExposureGroup string  `json:"exposure_group"` // Bundle group (e.g. "GOLD", "SILVER", "COPPER", "OIL")
    FeedSource    string  `json:"feed_source"`    // "BINANCE", "KUCOIN", "COINEX", "YAHOO", "TRADINGVIEW"
    SourceParam   string  `json:"source_param"`   // Exchange query parameter (e.g. "COPPERUSDT")
    MinSize       float64 `json:"min_size"`       // Minimum order quantity
    Decimals      int     `json:"decimals"`       // Price formatting decimals
}
```

#### Exposure Group Catalog:
- `GOLD`: `PAXG/USDT`, `XAUT/USDT`, `XAU/USDT`
- `SILVER`: `XAG/USDT`
- `COPPER`: `COPPER/USDT`
- `PLATINUM`: `XPT/USDT`
- `PALLADIUM`: `XPD/USDT`
- `OIL`: `OIL/USDT`
- `ALUMINUM`: `ALU/USDT`
- Crypto assets use their individual base asset code (`BTC`, `ETH`, `SOL`, etc.) as their default `ExposureGroup`.

### 2.2 Price Provider Interfaces (ISP & DIP)

```go
type PriceProvider interface {
    GetLatestPrice(ctx context.Context, symbol string) (float64, error)
}

type MarketStatsProvider interface {
    Get24hStats(symbol string) (price, volume24h, spreadBps float64, err error)
}

type TickerFetcher interface {
    FetchTicker(ctx context.Context, symbol string) (*cache.TickerQuote, error)
}
```

### 2.3 Multi-Source Providers
1. `BinanceFetcher`: Queries Binance Spot API, falls back to USDT-M Futures (`fapi.binance.com`).
2. `KuCoinFetcher`:
   - Spot: `https://api.kucoin.com/api/v1/market/orderbook/level1?symbol={BASE}-{QUOTE}`
   - Futures: `https://api-futures.kucoin.com/api/v1/ticker?symbol={BASE}USDTM`
3. `CoinExFetcher`:
   - Spot: `https://api.coinex.com/v2/spot/ticker?market={BASE}{QUOTE}`
   - Futures: `https://api.coinex.com/v2/futures/ticker?market={BASE}{QUOTE}`
4. `YahooFinanceFetcher`:
   - Chart API: `https://query1.finance.yahoo.com/v8/finance/chart/{symbol}?interval=1d&range=1d`
5. `TradingViewFetcher`:
   - Crypto Scanner: `POST https://scanner.tradingview.com/crypto/scan`
   - CFD Scanner: `POST https://scanner.tradingview.com/cfd/scan`
6. `MultiSourcePriceAggregator`:
   - Chains the fetchers in prioritized order.
   - If provider 1 errors or returns zero/negative, it advances to provider 2, then provider 3, etc.
   - Caches valid quotes in Redis/memory with short TTL (e.g. 5 seconds) to prevent rate limits.

---

## 3. Invariants & Business Logic Rules

### Rule 1: Correlated Commodity Concurrency Prohibition
- When generating a signal or executing a trade for `symbol`:
  1. Determine `group = GetExposureGroup(symbol)`.
  2. If `group` is a commodity group (`GOLD`, `SILVER`, `COPPER`, `PLATINUM`, `PALLADIUM`, `OIL`, `ALUMINUM`):
     - Query database or engine for ANY active signal or open position where `GetExposureGroup(existingSymbol) == group`.
     - If found: **REJECT or HOLD**. Return message: `"Correlated commodity exposure already active for group: %s (%s)"`.
     - Result: No concurrent trades or signals in PAXG + XAUT or multiple gold instruments.

### Rule 2: Dynamic Bucket & Definition Lookup
- Eliminate all hardcoded `if symbol == "PAXG/USDT" || symbol == "XAG/USDT" { bucket = "CORE" }`.
- Always call `market.GetBucket(symbol)` or `market.FindAsset(symbol)`. If symbol is defined as `CORE`, bucket is `"CORE"`, else `"ALPHA"`.

### Rule 3: Zero Synthetic Data & Fallbacks
- Screener: if price fetch fails, asset marked `REJECTED` with live error, never price 100.0.
- Backtest: requires real klines from exchange downloader, never sine waves.
- Feeds: all prices come from authentic exchange APIs.

---

## 4. Test Strategy & Acceptance Criteria

### 4.1 Unit Test Coverage
1. `internal/market/assets_test.go`:
   - Verify all 16 crypto assets and 7 commodity exposure groups are correctly classified in `CORE` or `ALPHA`.
   - Verify `GetExposureGroup` properly maps `PAXG/USDT`, `XAUT/USDT`, `XAU/USDT` to `"GOLD"`.
   - Verify `AreCorrelatedCommodities` returns true for PAXG & XAUT, PAXG & XAU, but false for PAXG & XAG, PAXG & BTC.
   - Verify `GetCorrelatedSymbols("PAXG/USDT")` returns all Gold variants.
2. `internal/market/multi_source_test.go`:
   - Mock HTTP server testing `KuCoinFetcher` (Spot & Futures responses).
   - Mock HTTP server testing `CoinExFetcher` (Spot & Futures responses).
   - Mock HTTP server testing `TradingViewFetcher` (Crypto & CFD responses).
   - Test `MultiSourcePriceAggregator` failover sequence (Primary fail -> Secondary success; Primary success -> Secondary not called).
3. `internal/server/signal_handlers_test.go`:
   - Test signal generation prevents duplicate signals across correlated commodities (`PAXG/USDT` active -> reject `XAUT/USDT`).
   - Test "Scan All Assets" dispatches Telegram alerts when new signals are generated.
   - Verify no Forex pairs are processed.
4. `web/src/App.test.ts`:
   - Verify all supported crypto and commodity assets appear with correct buckets and exposure groups.
   - Verify zero EUR or non-existent forex assets exist.

### 4.2 Integration & Docker Verification
- `go test -v -race ./...` passes 100%.
- `bun test` passes 100%.
- Docker Compose stack starts healthy with prebuilt/fresh build.
