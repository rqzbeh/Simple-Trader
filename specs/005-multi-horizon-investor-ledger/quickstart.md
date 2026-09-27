# Quickstart & Validation Guide: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Feature**: `specs/005-multi-horizon-investor-ledger`  
**Date**: 2026-09-20  
**Status**: Ready for Validation

---

## 1. Prerequisites & Environment Setup

Ensure the PostgreSQL 16 and Redis 7 services are accessible, or launch them via Docker Compose:

```bash
cd /home/redsnow/Simple-Trader
docker compose up -d postgres redis
```

---

## 2. Database Migration Verification

Apply the investor ledger database schema migration:

```bash
# Migration script execution
go run ./cmd/migrate
# Or apply directly via psql if running local dev
```

Verify tables created:
```sql
\dt investors
\dt investor_transactions
\dt news_articles
```

---

## 3. End-to-End Validation Scenarios

### Scenario 1: Non-Login Investor Registration & Initial Capital Attribution
1. **Action**: Create an investor profile with $10,000 USD initial deposit:
   ```bash
   curl -s -X POST http://localhost:8080/api/v1/investors \
     -H "Content-Type: application/json" \
     -d '{"name": "Alice Johnson", "contact_tag": "alice@hedgefund.io", "initial_deposit": 10000.0, "notes": "Founding LP"}' | jq
   ```
2. **Expected Result**:
   - HTTP status `201 Created`
   - Initial NAV is `1.000000`, `pool_units` credited = `10000.00000000`
   - `current_equity` = `$10,000.00`, `roi` = `0.0%`, `pool_share_pct` = `100.0%`
   - Initial deposit transaction recorded in `investor_transactions`.

### Scenario 2: Portfolio Trading Gain & Second Investor Deposit (No Dilution)
1. **Action**: Simulate a portfolio gain (+10%), increasing master equity to $11,000.
2. **Action**: Register second investor Bob with $11,000 deposit:
   ```bash
   curl -s -X POST http://localhost:8080/api/v1/investors \
     -H "Content-Type: application/json" \
     -d '{"name": "Bob Vance", "contact_tag": "bob@vance.com", "initial_deposit": 11000.0}' | jq
   ```
3. **Expected Result**:
   - At time of deposit, NAV is $11,000 / 10,000 units = `$1.100000` per unit.
   - Bob receives $11,000 / $1.10 = `10000.00000000` units.
   - Total units in pool = 20,000. Total equity = $22,000.
   - Alice: 10,000 units × $1.10 = $11,000 equity (+10.0% ROI).
   - Bob: 10,000 units × $1.10 = $11,000 equity (0.0% ROI).
   - Alice's gain is not diluted by Bob's capital arrival.

### Scenario 3: 3-Tier Liquidity Buffer & Zero-Slippage Withdrawal
1. **Action**: Check 3-Tier allocation breakdown:
   ```bash
   curl -s http://localhost:8080/api/v1/allocator/tiers | jq
   ```
   Verify:
   - `tier1_cash` holds at least 15% ($3,300)
   - `tier2_core` holds 45% ($9,900)
   - `tier3_tactical` holds 40% ($8,800)
2. **Action**: Alice withdraws $2,000:
   ```bash
   curl -s -X POST http://localhost:8080/api/v1/investors/<alice-id>/withdraw \
     -H "Content-Type: application/json" \
     -d '{"amount": 2000.0, "notes": "Partial distribution"}' | jq
   ```
3. **Expected Result**:
   - $2,000 is directly deducted from `tier1_cash` without touching Gold/Silver or active crypto trades.
   - Alice's units redeemed: $2,000 / $1.10 = `1818.18181818` units.
   - Alice remaining equity = $9,000. Total withdrawn = $2,000. Total deposited = $10,000.
   - Alice net profit = $9,000 + $2,000 - $10,000 = `+$1,000` (+10.0% ROI).

### Scenario 4: Live News Ingestion & Sentiment Circuit
1. **Action**: Query the live news endpoint:
   ```bash
   curl -s http://localhost:8080/api/v1/news/stream | jq '.[0:5]'
   ```
2. **Expected Result**:
   - Returns recent headlines with SHA-256 `content_hash`, normalized `sentiment_score` in `[-1.0, 1.0]`, and `polarity` tag (`BULLISH`, `BEARISH`, `NEUTRAL`).
   - Duplicate headlines are dropped via SHA-256 cache.

### Scenario 5: Dynamic Crypto Screener
1. **Action**: Query active screened universe:
   ```bash
   curl -s http://localhost:8080/api/v1/market/screener | jq
   ```
2. **Expected Result**:
   - Returns qualified pairs with `volume_24h > 50000000` and `bid_ask_spread <= 0.0010`.
   - Low liquidity coins are absent or flagged `DISQUALIFIED`.

---

## 4. Automated Test Suite Execution

Run all Go unit and integration tests:
```bash
CGO_ENABLED=0 go test -v -race ./internal/db/... ./internal/trader/... ./internal/market/...
```

Run frontend build and tests:
```bash
cd /home/redsnow/Simple-Trader/web
bun test
bun run build
```
