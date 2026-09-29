package trader

import (
	"fmt"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
)

// FeedState is one feeder's value-or-error (FR-014: data, never agency).
type FeedState struct {
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// CatalystFact is the real cluster meta handed to the core (spec-018 FR-501):
// headline, syndicated story count, fused sentiment, freshness, sources — all
// computed upstream by the clusterer, never invented here.
type CatalystFact struct {
	Headline       string   `json:"headline"`
	StoryCount     int      `json:"story_count"`
	FusedSentiment float64  `json:"fused_sentiment"`
	Freshness      float64  `json:"freshness"`
	Sources        []string `json:"sources,omitempty"`
}

// StateObject is the single per-cycle context payload fed to the decision
// core. Feeders contribute fields; nothing in here decides. Absent evidence
// is OMITTED (omitempty) — never fabricated (spec-018 FR-505).
type StateObject struct {
	Symbol         string                 `json:"symbol"`
	Timestamp      string                 `json:"timestamp"`
	SessionHourUTC int                    `json:"session_hour_utc"`
	Price          float64                `json:"price,omitempty"`
	Change24hPct   float64                `json:"change_24h_pct,omitempty"`
	Indicators     map[string]float64     `json:"indicators"`
	Semantic       map[string]string      `json:"semantic,omitempty"`
	Headlines      []string               `json:"headlines,omitempty"`
	Catalysts      []CatalystFact         `json:"catalysts,omitempty"`
	Sentiment      *ai.NewsSentimentInput `json:"sentiment,omitempty"`
	Gates          map[string]string      `json:"gates_as_fields"`
	TradingPolicy  string                 `json:"trading_policy,omitempty"`
}

// TradingPolicyText states the entry rules as data (docs: put policy in
// state). It references only fields that exist in this state.
const TradingPolicyText = "News-catalyst-first: an asset-relevant `headlines` story or a `catalysts` cluster must exist and agree with `semantic` and `indicators` before any entry. Mechanical gates (trend, volume, chase, OI, liquidation buffer, slot caps, risk caps) are enforced by code after this judgment and reject marginal setups. NO_TRADE is composed by code from the `edge` answer — never choose it in `direction`."

// BuildEntryState assembles the evidence-complete state (spec-018 FR-501)
// from data the pipeline ALREADY holds on the decision request. Hard failures
// name the missing input; unknown optional fields stay omitted.
func BuildEntryState(symbol, ts string, req ai.DecisionRequest, preGates map[string]string) (StateObject, error) {
	if symbol == "" {
		return StateObject{}, fmt.Errorf("component=state-builder: missing symbol")
	}
	indicators := snapToMap(req.IndicatorSnap)
	if snapshotEmpty(req.IndicatorSnap, indicators) {
		return StateObject{}, fmt.Errorf("component=state-builder cycle=%s: missing indicator snapshot", symbol)
	}
	if req.Quote.Price <= 0 {
		return StateObject{}, fmt.Errorf("component=state-builder cycle=%s: missing real price", symbol)
	}
	// Real numeric extras — only when actually measured (>0).
	if req.IndicatorSnap.NATR > 0 {
		indicators["natr"] = req.IndicatorSnap.NATR
	}
	if req.IndicatorSnap.VolumeRatio > 0 {
		indicators["volume_ratio"] = req.IndicatorSnap.VolumeRatio
	}
	if req.IndicatorSnap.VWAP > 0 {
		indicators["vwap"] = req.IndicatorSnap.VWAP
	}

	semantic := semanticBuckets(req)

	sessionHour := 0
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		sessionHour = t.UTC().Hour()
	}

	headlines := req.NewsHeadlines
	if len(headlines) > 10 {
		headlines = headlines[:10]
	}

	catalysts := make([]CatalystFact, 0, len(req.CatalystEvents))
	for i, e := range req.CatalystEvents {
		if i >= 10 {
			break
		}
		catalysts = append(catalysts, CatalystFact{
			Headline:       e.Headline,
			StoryCount:     e.StoryCount,
			FusedSentiment: e.FusedScore,
			Freshness:      e.Freshness,
			Sources:        e.Sources,
		})
	}

	gates := make(map[string]string, len(preGates))
	for k, v := range preGates {
		gates[k] = v
	}

	return StateObject{
		Symbol:         symbol,
		Timestamp:      ts,
		SessionHourUTC: sessionHour,
		Price:          req.Quote.Price,
		Change24hPct:   req.Quote.Change24h,
		Indicators:     indicators,
		Semantic:       semantic,
		Headlines:      headlines,
		Catalysts:      catalysts,
		Sentiment:      req.NewsSentiment,
		Gates:          gates,
		TradingPolicy:  TradingPolicyText,
	}, nil
}

// snapshotEmpty reports an all-zero snapshot (never measured) — all
// flattened values zero and no SuperTrend/regime means no real data.
func snapshotEmpty(snap cache.IndicatorSnapshot, flat map[string]float64) bool {
	for _, v := range flat {
		if v != 0 {
			return false
		}
	}
	return snap.SuperTrend == "" && snap.Regime == ""
}

// semanticBuckets precomputes categorical facts in Go — Jev is not a
// calculator (docs: model-jaggedness/jev-1.13). Empty bucket ⇒ omitted.
func semanticBuckets(req ai.DecisionRequest) map[string]string {
	snap := req.IndicatorSnap
	out := map[string]string{}
	if t := strings.ToLower(snap.SuperTrend); t == "bull" || t == "bear" {
		out["supertrend"] = t
	}
	if snap.VWAP > 0 {
		if req.Quote.Price > snap.VWAP {
			out["trend"] = "above_vwap"
		} else {
			out["trend"] = "below_vwap"
		}
	}
	if z := rsiZone(snap.RSI); z != "" {
		out["rsi_zone"] = z
	}
	if b := confluenceBand(snap.ConfluenceScore); b != "" {
		out["confluence_band"] = b
	}
	if snap.VolumeRatio > 0 {
		if snap.VolumeRatio >= VolumeRatioMin {
			out["volume_spike"] = "true"
		} else {
			out["volume_spike"] = "false"
		}
	}
	if snap.Regime != "" {
		out["regime"] = snap.Regime
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// rsiZone maps RSI to a categorical zone (no numeric clauses for the model).
func rsiZone(rsi float64) string {
	switch {
	case rsi <= 0:
		return ""
	case rsi >= 70:
		return "overbought"
	case rsi >= 55:
		return "bullish_momentum"
	case rsi >= 45:
		return "neutral"
	case rsi >= 30:
		return "bearish_momentum"
	default:
		return "oversold"
	}
}

// confluenceBand bands the indicator-confluence score on its REAL observed
// distribution (0.25-0.50 in live markets — docs/RESEARCH-jev-confidence.md).
func confluenceBand(c float64) string {
	switch {
	case c <= 0:
		return ""
	case c >= 0.50:
		return "strong"
	case c >= 0.35:
		return "decent"
	case c >= 0.25:
		return "typical_consolidation"
	default:
		return "weak"
	}
}
