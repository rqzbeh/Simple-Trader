package ai

import "github.com/rqzbeh/simple-trader/internal/cache"

// ClientConfig holds configuration settings for the OpenAI-compatible AI engine.
type ClientConfig struct {
	BaseURL         string
	APIKey          string
	ModelID         string
	Temperature     float64
	TimeoutSec      int
	ReasoningEffort string
}

// DecisionRequest bundles market state, indicators, and dynamic weights for LLM inference.
type DecisionRequest struct {
	Symbol        string
	Bucket        string
	Quote         cache.TickerQuote
	IndicatorSnap cache.IndicatorSnapshot
	Weights       map[string]float64
	NewsHeadlines []string
}

// DecisionResponse represents the structured trading decision output by the LLM or heuristic fallback.
type DecisionResponse struct {
	Decision                string  `json:"decision"`                   // "BUY", "SELL", or "HOLD"
	Confidence              float64 `json:"confidence"`                 // 0.0 - 1.0
	Reasoning               string  `json:"reasoning"`                  // LLM analytical justification
	SuggestedStopLossPct    float64 `json:"suggested_stop_loss_pct"`    // e.g. 1.5%
	SuggestedTakeProfitPct  float64 `json:"suggested_take_profit_pct"`  // e.g. 3.0%
	Regime                  string  `json:"regime"`                     // "BULL", "BEAR", or "RANGING"
	EstimatedWinProbability float64 `json:"estimated_win_probability"`  // 0.0 - 1.0
}

// TradeOutcome captures execution and exit results for adaptive weight tuning.
type TradeOutcome struct {
	Symbol          string
	Side            string
	Pnl             float64
	ReturnPct       float64
	SuperTrendTrend string
	RSI             float64
	MACDHistogram   float64
	HoldingDuration int64 // Seconds
}
