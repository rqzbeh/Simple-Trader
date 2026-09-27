# Data Model: Multi-Horizon 3-Tier Liquidity Allocator, Live News Ingestion, Dynamic Crypto Screener, and Investor Capital Ledger

**Feature**: `specs/005-multi-horizon-investor-ledger`  
**Date**: 2026-09-20  
**Status**: Completed

---

## 1. Relational Entities (PostgreSQL 16)

### `investors`
Represents an administrative profile of an individual capital contributor.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | Unique investor identifier |
| `name` | `VARCHAR(255)` | `NOT NULL` | Full investor name or entity identifier |
| `contact_tag` | `VARCHAR(255)` | `NOT NULL` | Email, Telegram handle, or client reference |
| `notes` | `TEXT` | `DEFAULT ''` | Administrative notes |
| `total_deposited` | `NUMERIC(18, 4)` | `NOT NULL DEFAULT 0.0000` | Cumulative total of all cash deposits |
| `total_withdrawn` | `NUMERIC(18, 4)` | `NOT NULL DEFAULT 0.0000` | Cumulative total of all withdrawals |
| `pool_units` | `NUMERIC(24, 8)` | `NOT NULL DEFAULT 0.00000000` | Total ownership units in the portfolio pool |
| `status` | `VARCHAR(32)` | `NOT NULL DEFAULT 'ACTIVE'` | `ACTIVE`, `FROZEN`, `CLOSED` |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Record creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Record update timestamp |

### `investor_transactions`
Immutable audit ledger recording all financial movements per investor.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | `UUID` | `PRIMARY KEY DEFAULT gen_random_uuid()` | Unique transaction ID |
| `investor_id` | `UUID` | `NOT NULL REFERENCES investors(id) ON DELETE CASCADE` | Associated investor |
| `tx_type` | `VARCHAR(32)` | `NOT NULL` | `DEPOSIT`, `WITHDRAWAL`, `PROFIT_PAYOUT`, `FEE` |
| `amount` | `NUMERIC(18, 4)` | `NOT NULL CHECK (amount > 0)` | Monetary amount in USD |
| `pool_units` | `NUMERIC(24, 8)` | `NOT NULL` | Units issued (positive) or redeemed (negative) |
| `nav_at_execution` | `NUMERIC(18, 6)` | `NOT NULL` | Portfolio NAV per unit at the time of transaction |
| `notes` | `TEXT` | `DEFAULT ''` | Transaction memo or banking reference |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Transaction timestamp |

### `news_articles`
Persisted market headlines ingested by the autonomous crawler.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `content_hash` | `CHAR(64)` | `PRIMARY KEY` | SHA-256 hash of normalized title |
| `title` | `TEXT` | `NOT NULL` | Headline title |
| `source` | `VARCHAR(64)` | `NOT NULL` | Source feed (`CryptoPanic`, `YahooFinance`, etc.) |
| `url` | `TEXT` | `NOT NULL` | Source link |
| `sentiment_score` | `NUMERIC(6, 4)` | `NOT NULL` | Lexicon score in `[-1.0000, +1.0000]` |
| `polarity` | `VARCHAR(16)` | `NOT NULL` | `BULLISH`, `BEARISH`, `NEUTRAL` |
| `key_phrases` | `TEXT[]` | `NOT NULL DEFAULT '{}'` | Matching lexicon phrases |
| `published_at` | `TIMESTAMPTZ` | `NOT NULL` | Publication timestamp |
| `ingested_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | Ingestion timestamp |

---

## 2. In-Memory & Redis Data Structures (Redis 7)

### `investor:pool_nav`
- **Type**: String (JSON / Float)
- **Key**: `investor:pool_nav`
- **Fields**:
  - `nav`: Current NAV per unit (e.g. `1.124500`)
  - `total_equity`: Master portfolio total equity in USD
  - `total_units`: Sum of all `pool_units` across active investors
  - `tier1_cash`: Total unencumbered Tier 1 cash buffer
  - `tier2_core`: Value of Gold/Silver positions
  - `tier3_tactical`: Value of active swing positions + tactical margin
  - `updated_at`: Unix timestamp

### `screener:active_universe`
- **Type**: Set / Hash
- **Key**: `screener:active_universe`
- **Description**: Symbols currently passing liquidity thresholds (e.g. `BTC/USD`, `ETH/USD`, `SOL/USD`, `AVAX/USD`, `LINK/USD`, `SUI/USD`).

### `news:seen_hashes`
- **Type**: Set with 72-hour TTL
- **Key**: `news:seen_hashes`
- **Description**: Set of SHA-256 hashes to ensure deduplication across polling runs.

---

## 3. Go Domain Models & Structs

```go
type Investor struct {
    ID             string    `json:"id"`
    Name           string    `json:"name"`
    ContactTag     string    `json:"contact_tag"`
    Notes          string    `json:"notes"`
    TotalDeposited float64   `json:"total_deposited"`
    TotalWithdrawn float64   `json:"total_withdrawn"`
    PoolUnits      float64   `json:"pool_units"`
    Status         string    `json:"status"` // ACTIVE, FROZEN, CLOSED
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`

    // Computed / Dynamic fields (rendered in UI)
    CurrentEquity  float64   `json:"current_equity"`
    NetProfit      float64   `json:"net_profit"`
    ROI            float64   `json:"roi"`            // Percentage
    PoolSharePct   float64   `json:"pool_share_pct"` // e.g. 12.5%
}

type CapitalTransaction struct {
    ID              string    `json:"id"`
    InvestorID      string    `json:"investor_id"`
    TxType          string    `json:"tx_type"` // DEPOSIT, WITHDRAWAL, PROFIT_PAYOUT
    Amount          float64   `json:"amount"`
    PoolUnits       float64   `json:"pool_units"`
    NAVAtExecution  float64   `json:"nav_at_execution"`
    Notes           string    `json:"notes"`
    CreatedAt       time.Time `json:"created_at"`
}

type ThreeTierAllocation struct {
    TotalEquity        float64 `json:"total_equity"`
    Tier1Cash          float64 `json:"tier1_cash"`          // 15% target
    Tier1TargetPct     float64 `json:"tier1_target_pct"`
    Tier2Core          float64 `json:"tier2_core"`          // 45% target (Gold, Silver)
    Tier2TargetPct     float64 `json:"tier2_target_pct"`
    Tier3Tactical      float64 `json:"tier3_tactical"`      // 40% target (3h Swing)
    Tier3TargetPct     float64 `json:"tier3_target_pct"`
    AvailableForWithdrawal float64 `json:"available_for_withdrawal"`
}

type ScreenedAsset struct {
    Symbol        string    `json:"symbol"`
    Price         float64   `json:"price"`
    Volume24h     float64   `json:"volume_24h"`
    BidAskSpread  float64   `json:"bid_ask_spread"` // in basis points or percentage
    Status        string    `json:"status"`         // ACTIVE, FROZEN, DISQUALIFIED
    LastCheckedAt time.Time `json:"last_checked_at"`
}
```
