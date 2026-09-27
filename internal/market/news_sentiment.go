package market

// SentimentPolarity classifies aggregated news sentiment.
type SentimentPolarity string

const (
	PolarityBullish  SentimentPolarity = "BULLISH"
	PolarityBearish  SentimentPolarity = "BEARISH"
	PolarityNeutral  SentimentPolarity = "NEUTRAL"
	PolarityMixed    SentimentPolarity = "MIXED"
)

// NewsSentimentReport captures classified news impact (spec-013 v3.0).
// Production values come from the decision core (Jev → 9Router escalation),
// never from a local word list — the lexicon classifier was deleted.
type NewsSentimentReport struct {
	Score         float64           `json:"score"` // -1.0 (bearish) to +1.0 (bullish)
	Polarity      SentimentPolarity `json:"polarity"`
	HeadlineCount int               `json:"headline_count"`
	KeyPhrases    []string          `json:"key_phrases"`
}

// NewsClassifier classifies a set of headlines. Implementations MUST return
// an explicit error on failure (FR-007) — never a neutral/default guess.
type NewsClassifier func(headlines []string) (NewsSentimentReport, error)

// ErrNoClassifier is returned by the default classifier: classification is a
// core decision, not something a fallback can fabricate.
type ClassifierNotConfiguredError struct{}

func (ClassifierNotConfiguredError) Error() string {
	return "news classifier not configured: decision core unavailable (no fallback exists)"
}

// DefaultClassifier fails explicitly until a core-backed classifier is injected.
func DefaultClassifier(headlines []string) (NewsSentimentReport, error) {
	return NewsSentimentReport{}, ClassifierNotConfiguredError{}
}

// marketWideTerms headlines whose catalyst genuinely applies to every
// instrument (macro policy, regulation, war), so stay in every symbol's
// prompt even when symbol itself not named.
var marketWideTerms = []string{
	"federal reserve", " fed ", "rate cut", "rate hike", "inflation", "recession",
	"central bank", "tariff", "war ", "sanction", "sec ", "sec approves", "sec lawsuit",
	"regulation", "etf approval", "etf inflow", "interest rate", "monetary policy", "gdp ",
}

// broadCryptoTerms catalysts move the whole crypto complex.
var broadCryptoTerms = []string{
	"crypto", "altcoin", "stablecoin", "blockchain", "exchange hack", "whale",
	"market crash", "market rally", "liquidation", "token unlock",
}

// HeadlinesForSymbol returns subset of headlines whose catalyst plausibly
// applies to the symbol (term-based scoping only — scoring is core-owned).
func HeadlinesForSymbol(headlines []string, symbol string) []string {
	var out []string
	sym := lower(symbol)
	base, quote := splitPair(sym)
	for _, raw := range headlines {
		line := lower(raw)
		keep := containsTerm(line, sym) || containsTerm(line, base) || containsTerm(line, quote)
		if !keep {
			for _, t := range marketWideTerms {
				if containsTerm(line, t) {
					keep = true
					break
				}
			}
		}
		if !keep {
			for _, t := range broadCryptoTerms {
				if containsTerm(line, t) {
					keep = true
					break
				}
			}
		}
		if keep {
			out = append(out, raw)
		}
	}
	return out
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func containsTerm(line, term string) bool {
	return len(term) > 0 && indexSub(line, term) >= 0
}

func indexSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func splitPair(sym string) (string, string) {
	for _, sep := range []string{"/", "-", "_"} {
		if idx := indexSub(sym, sep); idx >= 0 {
			return sym[:idx], sym[idx+1:]
		}
	}
	return sym, ""
}
