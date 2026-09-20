package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Candle represents an OHLCV bar.
type Candle struct {
	ID        int64     `json:"id"`
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	OpenTime  time.Time `json:"open_time"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// NewsItem represents a financial news article.
type NewsItem struct {
	ID             int64     `json:"id"`
	Source         string    `json:"source"`
	Headline       string    `json:"headline"`
	Summary        string    `json:"summary"`
	URL            string    `json:"url"`
	PublishedAt    time.Time `json:"published_at"`
	AssetTag       string    `json:"asset_tag"`
	SentimentScore float32   `json:"sentiment_score"`
	Hash           string    `json:"hash"`
}

// IndicatorWeight represents the dynamic weight for an indicator.
type IndicatorWeight struct {
	ID            int       `json:"id"`
	Symbol        string    `json:"symbol"`
	Regime        string    `json:"regime"`
	IndicatorName string    `json:"indicator_name"`
	Weight        float64   `json:"weight"`
	WinCount      int       `json:"win_count"`
	LossCount     int       `json:"loss_count"`
	CumulativePnL float64   `json:"cumulative_pnl"`
	LastUpdated   time.Time `json:"last_updated"`
}

// Signal represents a trade recommendation.
type Signal struct {
	ID                int64           `json:"id"`
	Symbol            string          `json:"symbol"`
	Side              string          `json:"side"`   // 'BUY' or 'SELL'
	Bucket            string          `json:"bucket"` // 'CORE' or 'ALPHA'
	EntryPrice        float64         `json:"entry_price"`
	StopLoss          float64         `json:"stop_loss"`
	TakeProfit        float64         `json:"take_profit"`
	Confidence        float32         `json:"confidence"`
	ConfluenceScore   float32         `json:"confluence_score"`
	AIReasoning       string          `json:"ai_reasoning"`
	IndicatorSnapshot json.RawMessage `json:"indicator_snapshot"`
	Status            string          `json:"status"` // 'OPEN', 'EXECUTED', 'CANCELLED', 'EXPIRED'
	CreatedAt         time.Time       `json:"created_at"`
}

// Validate ensures signal parameters are valid.
func (s *Signal) Validate() error {
	if s.Symbol == "" {
		return errors.New("symbol is required")
	}
	if s.Side != "BUY" && s.Side != "SELL" {
		return fmt.Errorf("invalid side '%s', must be 'BUY' or 'SELL'", s.Side)
	}
	if s.Bucket != "CORE" && s.Bucket != "ALPHA" {
		return fmt.Errorf("invalid bucket '%s', must be 'CORE' or 'ALPHA'", s.Bucket)
	}
	if s.EntryPrice <= 0 || s.StopLoss <= 0 || s.TakeProfit <= 0 {
		return errors.New("entry, stop-loss, and take-profit must be greater than zero")
	}
	return nil
}

// Trade represents an executed order.
type Trade struct {
	ID           int64      `json:"id"`
	SignalID     *int64     `json:"signal_id"`
	Symbol       string     `json:"symbol"`
	Side         string     `json:"side"`   // 'BUY' or 'SELL'
	Bucket       string     `json:"bucket"` // 'CORE' or 'ALPHA'
	PositionSize float64    `json:"position_size"`
	EntryPrice   float64    `json:"entry_price"`
	EntryTime    time.Time  `json:"entry_time"`
	ExitPrice    float64    `json:"exit_price"`
	ExitTime     *time.Time `json:"exit_time"`
	StopLoss     float64    `json:"stop_loss"`
	TakeProfit   float64    `json:"take_profit"`
	RealizedPnL  float64    `json:"realized_pnl"`
	ReturnPct    float32    `json:"return_pct"`
	ExitReason   string     `json:"exit_reason"`
	RootCause    string     `json:"root_cause"`
	Status       string     `json:"status"` // 'OPEN', 'CLOSED'
	ExecutionFee     float64    `json:"execution_fee"`
	SlippagePaid     float64    `json:"slippage_paid"`
	Leverage         int        `json:"leverage"`
	LiquidationPrice float64    `json:"liquidation_price"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CalculatePnL computes realized profit/loss and return percentage.
func (t *Trade) CalculatePnL() (float64, float32) {
	if t.ExitPrice == 0 || t.EntryPrice == 0 {
		return 0, 0
	}
	var diff float64
	if t.Side == "BUY" {
		diff = t.ExitPrice - t.EntryPrice
	} else {
		diff = t.EntryPrice - t.ExitPrice
	}
	grossPnL := diff * t.PositionSize
	netPnL := grossPnL - t.ExecutionFee
	lev := t.Leverage
	if lev < 1 {
		lev = 1
	}
	margin := (t.EntryPrice * t.PositionSize) / float64(lev)
	retPct := float32((netPnL / margin) * 100)
	return netPnL, retPct
}

// FineTuneRecord holds exportable pairs for OpenAI model fine-tuning.
type FineTuneRecord struct {
	ID                int64     `json:"id"`
	TradeID           int64     `json:"trade_id"`
	Symbol            string    `json:"symbol"`
	PromptSystem      string    `json:"prompt_system"`
	PromptUser        string    `json:"prompt_user"`
	AssistantResponse string    `json:"assistant_response"`
	TradeOutcome      string    `json:"trade_outcome"` // 'WIN' or 'LOSS'
	Exported          bool      `json:"exported"`
	CreatedAt         time.Time `json:"created_at"`
}
