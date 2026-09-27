package trader

import (
	"fmt"

	"github.com/rqzbeh/simple-trader/internal/market"
)

// FeedState is one feeder's value-or-error (FR-014: data, never agency).
type FeedState struct {
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// StateObject is the single per-cycle context payload fed to the decision
// core. Feeders contribute fields; nothing in here decides.
type StateObject struct {
	Symbol        string                 `json:"symbol"`
	Timestamp     string                 `json:"timestamp"`
	Indicators    map[string]float64     `json:"indicators"`
	News          FeedState               `json:"news"`
	LSTMStats     FeedState               `json:"lstm_stats"`
	ATR           map[string]float64     `json:"atr_levels"`
	Gates         map[string]string      `json:"gates_as_fields"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

// BuildState assembles the state object. Empty candles ⇒ explicit error
// naming the missing input (no fabricated state).
func BuildState(symbol, ts string, indicators map[string]float64, atrSL, atrTP float64, news market.NewsSentimentReport, lstm FeedState, gates map[string]string) (StateObject, error) {
	if symbol == "" {
		return StateObject{}, fmt.Errorf("component=state-builder: missing symbol")
	}
	if len(indicators) == 0 {
		return StateObject{}, fmt.Errorf("component=state-builder cycle=%s: missing indicator snapshot", symbol)
	}
	if atrSL <= 0 || atrTP <= 0 {
		return StateObject{}, fmt.Errorf("component=state-builder cycle=%s: missing ATR levels", symbol)
	}
	return StateObject{
		Symbol:     symbol,
		Timestamp:  ts,
		Indicators: indicators,
		News: FeedState{Value: map[string]interface{}{
			"polarity": string(news.Polarity), "score": news.Score, "headlines": news.HeadlineCount,
		}},
		LSTMStats: lstm,
		ATR:       map[string]float64{"stop_loss": atrSL, "take_profit": atrTP},
		Gates:     gates,
	}, nil
}
