package cache

import (
	"encoding/json"
	"fmt"
)

// TickerKey generates the Redis key for a market quote ticker.
func TickerKey(symbol string) string {
	return fmt.Sprintf("ticker:%s", symbol)
}

// CandleKey generates the Redis key for historical candles.
func CandleKey(symbol, timeframe string) string {
	return fmt.Sprintf("candles:%s:%s", symbol, timeframe)
}

// WeightsKey generates the Redis key for dynamic indicator weights.
func WeightsKey(symbol, regime string) string {
	return fmt.Sprintf("weights:%s:%s", symbol, regime)
}

// PubSubChannels defines the standard pub/sub communication channels.
const (
	ChannelMarketTicks = "pubsub:market_ticks"
	ChannelSignals     = "pubsub:signals"
	ChannelTrades      = "pubsub:trades"
)

// TickerQuote represents the in-memory cached market quote.
type TickerQuote struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change_24h"`
	High24h   float64 `json:"high_24h"`
	Low24h    float64 `json:"low_24h"`
	Volume    float64 `json:"volume"`
	UpdatedAt int64   `json:"updated_at"`
}

func (t *TickerQuote) Marshal() ([]byte, error) {
	return json.Marshal(t)
}

func (t *TickerQuote) Unmarshal(data []byte) error {
	return json.Unmarshal(data, t)
}

// IndicatorSnapshot represents the cached multi-indicator state.
type IndicatorSnapshot struct {
	Symbol          string  `json:"symbol"`
	RSI             float64 `json:"rsi"`
	MACD            float64 `json:"macd"`
	Signal          float64 `json:"signal"`
	Histogram       float64 `json:"histogram"`
	UpperBand       float64 `json:"upper_band"`
	MiddleBand      float64 `json:"middle_band"`
	LowerBand       float64 `json:"lower_band"`
	SuperTrend      string  `json:"super_trend"` // "BULL" or "BEAR"
	ConfluenceScore float64 `json:"confluence_score"`
	Regime          string  `json:"regime"`
	UpdatedAt       int64   `json:"updated_at"`
}

func (s *IndicatorSnapshot) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func (s *IndicatorSnapshot) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}
