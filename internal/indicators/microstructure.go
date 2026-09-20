package indicators

import (
	"math"
)

// OrderBookLevel represents price and available volume at a specific order book tier.
type OrderBookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// CalculateOBI computes Level-N Order Book Imbalance bounded strictly in [-1.0, 1.0].
// Positive OBI (> +0.30) indicates institutional bid accumulation.
// Negative OBI (< -0.30) indicates institutional ask distribution.
func CalculateOBI(bids, asks []OrderBookLevel) float64 {
	var totalBidQty, totalAskQty float64

	for _, b := range bids {
		if b.Quantity > 0 {
			totalBidQty += b.Quantity
		}
	}
	for _, a := range asks {
		if a.Quantity > 0 {
			totalAskQty += a.Quantity
		}
	}

	totalDepth := totalBidQty + totalAskQty
	if totalDepth <= 0 {
		return 0.0
	}

	obi := (totalBidQty - totalAskQty) / totalDepth
	if obi > 1.0 {
		return 1.0
	}
	if obi < -1.0 {
		return -1.0
	}
	return obi
}

// CVDTracker maintains cumulative volume delta state across aggressive trades.
type CVDTracker struct {
	CumulativeDelta float64
	History         []float64
	MaxHistory      int
}

// NewCVDTracker initializes a new Cumulative Volume Delta tracker.
func NewCVDTracker(maxHistory int) *CVDTracker {
	if maxHistory <= 0 {
		maxHistory = 200
	}
	return &CVDTracker{
		CumulativeDelta: 0.0,
		History:         make([]float64, 0, maxHistory),
		MaxHistory:      maxHistory,
	}
}

// Update records aggressive buyer vs seller volume and returns current CVD.
// buyerInitiatedVol: aggressive taker buy volume
// sellerInitiatedVol: aggressive taker sell volume
func (c *CVDTracker) Update(buyerInitiatedVol, sellerInitiatedVol float64) float64 {
	delta := buyerInitiatedVol - sellerInitiatedVol
	c.CumulativeDelta += delta

	c.History = append(c.History, c.CumulativeDelta)
	if len(c.History) > c.MaxHistory {
		c.History = c.History[1:]
	}
	return c.CumulativeDelta
}

// DivergenceType represents order flow absorption or exhaustion signals.
type DivergenceType string

const (
	DivergenceNone              DivergenceType = "NONE"
	DivergenceBullishAbsorption DivergenceType = "BULLISH_ABSORPTION"
	DivergenceBearishExhaustion DivergenceType = "BEARISH_EXHAUSTION"
)

// DetectDivergence evaluates whether price trends diverge from CVD volume flows.
// Bullish Absorption: Price prints lower low, but CVD forms higher low (smart money absorption).
// Bearish Exhaustion: Price prints higher high, but CVD forms lower high (buyers running out of steam).
func DetectDivergence(prices []float64, cvdValues []float64) DivergenceType {
	n := len(prices)
	if n < 4 || len(cvdValues) < 4 || n != len(cvdValues) {
		return DivergenceNone
	}

	pPrev := prices[n-3]
	pCurr := prices[n-1]
	cPrev := cvdValues[n-3]
	cCurr := cvdValues[n-1]

	// Bullish Absorption: Price drops while CVD increases
	if pCurr < pPrev && cCurr > cPrev {
		priceDropPct := (pPrev - pCurr) / pPrev
		if priceDropPct > 0.001 { // at least 10 bps drop
			return DivergenceBullishAbsorption
		}
	}

	// Bearish Exhaustion: Price rises while CVD drops
	if pCurr > pPrev && cCurr < cPrev {
		priceRisePct := (pCurr - pPrev) / pPrev
		if priceRisePct > 0.001 { // at least 10 bps rise
			return DivergenceBearishExhaustion
		}
	}

	return DivergenceNone
}

// MicrostructureState encapsulates combined L2 OBI and CVD metrics.
type MicrostructureState struct {
	OBI        float64        `json:"obi"`
	CVD        float64        `json:"cvd"`
	Divergence DivergenceType `json:"divergence"`
}

// EvaluateMicrostructure combines OBI and CVD into a unified microstructure signal score.
// Returns score in [-1.0, 1.0] where > 0 is bullish and < 0 is bearish.
func EvaluateMicrostructure(obi float64, divergence DivergenceType) float64 {
	score := obi * 0.70 // OBI contributes 70% of base microstructure

	switch divergence {
	case DivergenceBullishAbsorption:
		score += 0.30
	case DivergenceBearishExhaustion:
		score -= 0.30
	}

	return math.Max(-1.0, math.Min(1.0, score))
}
