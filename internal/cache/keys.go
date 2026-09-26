package cache

import (
	"encoding/json"
	"fmt"
)

// TickerKey generates the Redis key for a market quote ticker.
func TickerKey(symbol string) string {
	return fmt.Sprintf("ticker:%s", symbol)
}

// IndicatorKey generates the Redis key for an indicator snapshot.
func IndicatorKey(symbol string) string {
	return fmt.Sprintf("indicators:%s", symbol)
}

// CandleKey generates the Redis key for historical candles.
func CandleKey(symbol, timeframe string) string {
	return fmt.Sprintf("candles:%s:%s", symbol, timeframe)
}

// WeightsKey generates the Redis key for dynamic indicator weights.
func WeightsKey(symbol, regime string) string {
	return fmt.Sprintf("weights:%s:%s", symbol, regime)
}

// SessionKey generates the Redis key for active admin user session tokens.
func SessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

// RateLimitKey generates the Redis key for sliding-window login attempts per IP/username.
func RateLimitKey(identifier string) string {
	return fmt.Sprintf("ratelimit:login:%s", identifier)
}

// SignalQueueKey generates the Redis key for asynchronous signal distribution.
func SignalQueueKey() string {
	return "queue:signals:futures"
}

// TelegramQueueKey generates the Redis key for asynchronous telegram notifications.
func TelegramQueueKey() string {
	return "queue:telegram:notifications"
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
	Change24h float64 `json:"change24h"`
	High24h   float64 `json:"high24h"`
	Low24h    float64 `json:"low24h"`
	Volume    float64 `json:"volume"`
	UpdatedAt int64   `json:"updated_at"`
}

func (t TickerQuote) MarshalJSON() ([]byte, error) {
	type Alias TickerQuote
	return json.Marshal(&struct {
		Alias
		Change24hSnake float64 `json:"change_24h"`
		High24hSnake   float64 `json:"high_24h"`
		Low24hSnake    float64 `json:"low_24h"`
	}{
		Alias:          Alias(t),
		Change24hSnake: t.Change24h,
		High24hSnake:   t.High24h,
		Low24hSnake:    t.Low24h,
	})
}

func (t *TickerQuote) UnmarshalJSON(data []byte) error {
	type Alias TickerQuote
	aux := struct {
		*Alias
		Change24hSnake *float64 `json:"change_24h"`
		High24hSnake   *float64 `json:"high_24h"`
		Low24hSnake    *float64 `json:"low_24h"`
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if t.Change24h == 0 && aux.Change24hSnake != nil {
		t.Change24h = *aux.Change24hSnake
	}
	if t.High24h == 0 && aux.High24hSnake != nil {
		t.High24h = *aux.High24hSnake
	}
	if t.Low24h == 0 && aux.Low24hSnake != nil {
		t.Low24h = *aux.Low24hSnake
	}
	return nil
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
	OBI             float64 `json:"obi"`
	CVD             float64 `json:"cvd"`
	Divergence      string  `json:"divergence"`
	VolRatio        float64 `json:"vol_ratio"`
	GarmanKlass     float64 `json:"garman_klass"`
	Parkinson       float64 `json:"parkinson"`
	KaufmanER       float64 `json:"kaufman_er"`
	CMF             float64 `json:"cmf"`
	NATR            float64 `json:"natr"`
	VWAP            float64 `json:"vwap"`
	VolumeRatio     float64 `json:"volume_ratio"`
	UpdatedAt       int64   `json:"updated_at"`
}

func (s *IndicatorSnapshot) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func (s *IndicatorSnapshot) Unmarshal(data []byte) error {
	return json.Unmarshal(data, s)
}
