package db

import (
	"time"
)

// Investor represents an administrative non-login investor profile.
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

	// Dynamically computed pro-rata fields
	CurrentEquity float64 `json:"current_equity"`
	NetProfit     float64 `json:"net_profit"`
	ROI           float64 `json:"roi"`
	PoolSharePct  float64 `json:"pool_share_pct"`
}

// CapitalTransaction represents an immutable financial flow for an investor.
type CapitalTransaction struct {
	ID             string    `json:"id"`
	InvestorID     string    `json:"investor_id"`
	TxType         string    `json:"tx_type"` // DEPOSIT, WITHDRAWAL, PROFIT_PAYOUT, FEE
	Amount         float64   `json:"amount"`
	PoolUnits      float64   `json:"pool_units"`
	NAVAtExecution float64   `json:"nav_at_execution"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
}

// PortfolioNAVHistory records historical master NAV checkpoints.
type PortfolioNAVHistory struct {
	ID               int64     `json:"id"`
	TotalEquity      float64   `json:"total_equity"`
	Tier1CashReserve float64   `json:"tier1_cash_reserve"`
	Tier2CoreEquity  float64   `json:"tier2_core_equity"`
	Tier3AlphaEquity float64   `json:"tier3_alpha_equity"`
	TotalUnits       float64   `json:"total_units"`
	NAVPerUnit       float64   `json:"nav_per_unit"`
	RecordedAt       time.Time `json:"recorded_at"`
}

// NewsArticle represents a crawled market headline.
type NewsArticle struct {
	ContentHash    string    `json:"content_hash"`
	Title          string    `json:"title"`
	Source         string    `json:"source"`
	URL            string    `json:"url"`
	SentimentScore float64   `json:"sentiment_score"`
	Polarity       string    `json:"polarity"` // BULLISH, BEARISH, NEUTRAL
	KeyPhrases     []string  `json:"key_phrases"`
	PublishedAt    time.Time `json:"published_at"`
	IngestedAt     time.Time `json:"ingested_at"`
}

// ScreenedAsset represents a dynamically screened crypto pair.
type ScreenedAsset struct {
	Symbol          string    `json:"symbol"`
	Price           float64   `json:"price"`
	Volume24h       float64   `json:"volume_24h"`
	BidAskSpreadBps float64   `json:"bid_ask_spread_bps"`
	Status          string    `json:"status"` // ACTIVE, FROZEN, DISQUALIFIED
	RejectionReason string    `json:"rejection_reason,omitempty"`
	ScreenedAt      time.Time `json:"screened_at"`
}

// ThreeTierAllocation contains live balances and target ratios.
type ThreeTierAllocation struct {
	TotalEquity            float64 `json:"total_equity"`
	Tier1Cash              float64 `json:"tier1_cash"`
	Tier1TargetPct         float64 `json:"tier1_target_pct"`
	Tier2Core              float64 `json:"tier2_core"`
	Tier2TargetPct         float64 `json:"tier2_target_pct"`
	Tier3Tactical          float64 `json:"tier3_tactical"`
	Tier3TargetPct         float64 `json:"tier3_target_pct"`
	AvailableForWithdrawal float64 `json:"available_for_withdrawal"`
}
