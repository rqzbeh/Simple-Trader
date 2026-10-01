package ai

import "github.com/rqzbeh/simple-trader/internal/cache"

// ClientConfig holds configuration settings for the OpenAI-compatible AI engine.
type ClientConfig struct {
	BaseURL            string
	APIKey             string
	ModelID            string
	Temperature        float64
	TimeoutSec         int
	ReasoningEffort    string
	DefaultLeverage    int
	MinStopLossPct     float64
	MaxStopLossPct     float64
	MinTakeProfitPct   float64
	MaxTakeProfitPct   float64
	MinRiskRewardRatio float64
}

// NewsSentimentInput carries the pre-computed NLP sentiment packet for the
// headlines attached to a DecisionRequest, so the model reasons from scored
// evidence instead of raw text alone.
type NewsSentimentInput struct {
	Score         float64  `json:"score"`                 // -1.0 (bearish) to +1.0 (bullish)
	Polarity      string   `json:"polarity"`              // BULLISH, BEARISH, NEUTRAL
	HeadlineCount int      `json:"headline_count"`        // Headlines scored
	BullishCount  int      `json:"bullish_count"`         // Headlines leaning bullish
	BearishCount  int      `json:"bearish_count"`         // Headlines leaning bearish
	KeyPhrases    []string `json:"key_phrases,omitempty"` // Trigger phrases detected
}

// DecisionRequest bundles market state, indicators, and dynamic weights for LLM inference.
type DecisionRequest struct {
	Symbol         string
	Bucket         string
	Quote          cache.TickerQuote
	IndicatorSnap  cache.IndicatorSnapshot
	Weights        map[string]float64
	NewsHeadlines  []string
	NewsSentiment  *NewsSentimentInput
	CatalystEvents []CatalystEventInput // clustered events (US3 T036)
	HorizonMinutes int                  // profile holding horizon for the bucket (US4 T040)
	// Gates: real outcomes of the pre-AI entry guards that already ran
	// (spec-018 FR-501) — hydrated state, never an empty placeholder.
	Gates map[string]string
}

// CatalystEventInput is the clustered Catalyst Event payload sent to the
// model instead of a flat list of duplicate headlines (spec 012 US3, T036).
type CatalystEventInput struct {
	Headline   string   `json:"headline"`
	StoryCount int      `json:"story_count"`
	FusedScore float64  `json:"fused_score"`
	Freshness  float64  `json:"freshness"`
	Sources    []string `json:"sources,omitempty"`
}

// DecisionResponse represents the structured trading decision output by the LLM or heuristic fallback.
type DecisionResponse struct {
	Evidence                []string `json:"evidence,omitempty"`
	Decision                string   `json:"decision"`                  // "BUY", "SELL", or "HOLD"
	Confidence              float64  `json:"confidence"`                // 0.0 - 1.0
	Reasoning               string   `json:"reasoning"`                 // LLM analytical justification
	Catalyst                string   `json:"catalyst,omitempty"`        // Primary news catalyst headline or source
	Leverage                int      `json:"leverage,omitempty"`        // Isolated leverage factor (1x - 10x)
	AllocationPct           float64  `json:"allocation_pct,omitempty"`  // Suggested % of available alpha fund (e.g. 2.0%)
	SuggestedStopLossPct    float64  `json:"suggested_stop_loss_pct"`   // e.g. 1.5%
	SuggestedTakeProfitPct  float64  `json:"suggested_take_profit_pct"` // e.g. 3.0%
	Regime                  string   `json:"regime"`                    // "BULL", "BEAR", or "RANGING"
	EstimatedWinProbability float64  `json:"estimated_win_probability"` // 0.0 - 1.0
	// GateRejected is set when an entry gate vetoed a BUY/SELL decision
	// (spec 012 US1). Non-empty rule name; the signal was NOT persisted.
	GateRejected string `json:"gate_rejected,omitempty"`
	// GateRejectedDetail carries the rule metrics for the audit/SSE payload.
	GateRejectedDetail map[string]interface{} `json:"gate_rejected_detail,omitempty"`

	// Dynamic trade timeframe (spec-016)
	Timeframe             string             `json:"timeframe,omitempty"`
	TimeframeDistribution map[string]float64 `json:"timeframe_distribution,omitempty"`
	TimeframeConfidence   float64            `json:"timeframe_confidence,omitempty"`

	// Managed trade parameters (spec-015)
	ParameterModes         []byte `json:"parameter_modes,omitempty"`
	ParameterValues        []byte `json:"parameter_values,omitempty"`
	ParameterDistributions []byte `json:"parameter_distributions,omitempty"`
	ParameterClamps        []byte `json:"parameter_clamps,omitempty"`
}

// TradeOutcome captures execution and exit results for adaptive weight tuning.
// Indicator fields come from the decision-time snapshot recorded with the
// signal (spec 012 US7, FR-022). HasSnapshot=false means no recording exists
// (legacy rows) and the outcome MUST NOT move any statistic (FR-023).
type TradeOutcome struct {
	Symbol          string
	Side            string
	Pnl             float64
	ReturnPct       float64
	HasSnapshot     bool
	SuperTrendTrend string
	RSI             float64
	MACDHistogram   float64
	CMF             float64
	KaufmanER       float64
	OBI             float64
	Divergence      string
	HoldingDuration int64 // Seconds
}
