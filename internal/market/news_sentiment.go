package market

import (
	"math"
	"strings"
)

// SentimentPolarity classifies aggregated news sentiment.
type SentimentPolarity string

const (
	PolarityBullish SentimentPolarity = "BULLISH"
	PolarityBearish SentimentPolarity = "BEARISH"
	PolarityNeutral SentimentPolarity = "NEUTRAL"
)

// NewsSentimentReport captures the quantitative NLP score of market headlines.
type NewsSentimentReport struct {
	Score      float64           `json:"score"`      // -1.0 (extremely bearish) to +1.0 (extremely bullish)
	Polarity   SentimentPolarity `json:"polarity"`   // BULLISH, BEARISH, NEUTRAL
	HeadlineCount int            `json:"headline_count"`
	KeyPhrases []string          `json:"key_phrases"`
}

var bullishTerms = []string{
	"rate cut", "inflation cool", "inflation drops", "etf approved", "etf inflow",
	"record high", "bullish", "stimulus", "accumulate", "reserve currency",
	"breakout", "rally", "easing", "dovish", "liquidity surge", "halving",
	// Whale & Institutional accumulation signals
	"whale buy", "whale accumulation", "whale alert", "massive transfer to cold",
	"institutional accumulation", "spot etf inflow", "billionaire buy", "treasury reserve",
	"strategic bitcoin reserve", "whale loading", "holding supply",
	// Politician & High-Profile Insiders
	"trump crypto", "trump backing", "trump endorsement", "world liberty financial",
	"trump son", "barron trump", "eric trump", "pelosi buy", "congressional buy",
	"senator buy", "pro-crypto legislation", "insider accumulation",
}

var bearishTerms = []string{
	"rate hike", "hawkish", "inflation surges", "war", "recession", "insolvency",
	"bank run", "sec lawsuit", "sanction", "selloff", "crash", "bearish",
	"liquidation cascade", "hack", "stolen", "contagion", "downgrade",
	// Whale dump & Manipulation signals
	"whale dump", "whale sell", "transfer to exchange", "whale liquidation",
	"market manipulation", "pump and dump", "rug pull", "wash trading",
	// Politician & Regulatory enforcement signals
	"insider dump", "insider selling", "politician sell", "pelosi sell",
	"congressional disclosure dump", "sec subpoena", "sec investigation",
	"fraud charges", "crypto crackdown", "subpoena", "anti-crypto",
}

// AnalyzeNewsSentiment evaluates financial headlines using a quantitative financial lexicon.
// Produces a normalized polarity score in [-1.0, 1.0].
func AnalyzeNewsSentiment(headlines []string) NewsSentimentReport {
	if len(headlines) == 0 {
		return NewsSentimentReport{
			Score:         0.0,
			Polarity:      PolarityNeutral,
			HeadlineCount: 0,
			KeyPhrases:    []string{},
		}
	}

	var rawScore float64
	var keyPhrases []string

	for _, raw := range headlines {
		line := strings.ToLower(raw)
		for _, b := range bullishTerms {
			if strings.Contains(line, b) {
				rawScore += 1.0
				keyPhrases = append(keyPhrases, b)
			}
		}
		for _, b := range bearishTerms {
			if strings.Contains(line, b) {
				rawScore -= 1.0
				keyPhrases = append(keyPhrases, b)
			}
		}
	}

	// Normalize by number of headlines with hyperbolic tangent compression
	denom := math.Max(1.0, float64(len(headlines))*0.5)
	normalized := math.Tanh(rawScore / denom)
	normalized = math.Round(normalized*1000) / 1000

	var polarity SentimentPolarity
	if normalized >= 0.20 {
		polarity = PolarityBullish
	} else if normalized <= -0.20 {
		polarity = PolarityBearish
	} else {
		polarity = PolarityNeutral
	}

	if keyPhrases == nil {
		keyPhrases = []string{}
	}

	return NewsSentimentReport{
		Score:         normalized,
		Polarity:      polarity,
		HeadlineCount: len(headlines),
		KeyPhrases:    keyPhrases,
	}
}
