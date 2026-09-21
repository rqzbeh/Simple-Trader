package ai

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// BetaPosterior maintains the conjugate Beta distribution parameters for an indicator.
type BetaPosterior struct {
	Alpha float64 `json:"alpha"` // Success count + prior
	Beta  float64 `json:"beta"`  // Failure count + prior
}

// NewBetaPosterior initializes a Beta distribution with an uninformative or mild prior.
func NewBetaPosterior(priorAlpha, priorBeta float64) *BetaPosterior {
	if priorAlpha <= 0 {
		priorAlpha = 2.0
	}
	if priorBeta <= 0 {
		priorBeta = 2.0
	}
	return &BetaPosterior{
		Alpha: priorAlpha,
		Beta:  priorBeta,
	}
}

// Update records a win or loss observation with attribution weighting.
func (b *BetaPosterior) Update(win bool, attributionWeight float64) {
	if attributionWeight <= 0 {
		attributionWeight = 1.0
	}
	if win {
		b.Alpha += attributionWeight
	} else {
		b.Beta += attributionWeight
	}
}

// ExpectedValue returns the theoretical mean of the posterior: E[X] = alpha / (alpha + beta).
func (b *BetaPosterior) ExpectedValue() float64 {
	total := b.Alpha + b.Beta
	if total <= 0 {
		return 0.5
	}
	return b.Alpha / total
}

// Sample draws a random variate from Beta(alpha, beta) using Marsaglia-Tsang Gamma sampling.
func (b *BetaPosterior) Sample(r *rand.Rand) float64 {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	x := sampleGamma(b.Alpha, 1.0, r)
	y := sampleGamma(b.Beta, 1.0, r)
	if x+y <= 0 {
		return 0.5
	}
	return x / (x + y)
}

// sampleGamma implements Marsaglia and Tsang (2000) method for Gamma(alpha, 1).
func sampleGamma(alpha, beta float64, r *rand.Rand) float64 {
	if alpha <= 0 {
		return 0
	}
	if alpha < 1.0 {
		// Johnk's generator or Weibull transformation: Gamma(alpha) = Gamma(alpha+1) * U^(1/alpha)
		u := r.Float64()
		for u == 0 {
			u = r.Float64()
		}
		return sampleGamma(alpha+1.0, beta, r) * math.Pow(u, 1.0/alpha)
	}

	d := alpha - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)

	for {
		z := r.NormFloat64()
		v := 1.0 + c*z
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := r.Float64()
		if u < 1.0-0.0331*z*z*z*z {
			return (d * v) / beta
		}
		if math.Log(u) < 0.5*z*z+d-d*v+d*math.Log(v) {
			return (d * v) / beta
		}
	}
}

// ThompsonSampler maintains conjugate Beta distributions across indicators to sample optimal weights.
type ThompsonSampler struct {
	mu         sync.RWMutex
	posteriors map[string]*BetaPosterior
	rng        *rand.Rand
}

// NewThompsonSampler initializes the Thompson Sampling engine.
func NewThompsonSampler(seed int64) *ThompsonSampler {
	var rng *rand.Rand
	if seed != 0 {
		rng = rand.New(rand.NewSource(seed))
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	indicators := []string{"RSI", "MACD", "SUPERTREND", "MICROSTRUCTURE"}
	posteriors := make(map[string]*BetaPosterior)
	for _, ind := range indicators {
		posteriors[ind] = NewBetaPosterior(2.0, 2.0)
	}

	return &ThompsonSampler{
		posteriors: posteriors,
		rng:        rng,
	}
}

// SampleWeights generates dynamic indicator weights using posterior sampling (FR-006).
// Resulting weights are normalized and clamped to [0.20, 3.00x].
func (ts *ThompsonSampler) SampleWeights() map[string]float64 {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	weights := make(map[string]float64)
	for name, post := range ts.posteriors {
		theta := post.Sample(ts.rng) // theta in (0, 1)

		// Base expectation for prior Beta(2, 2) is 0.50.
		// Scale = theta / 0.50
		rawWeight := theta / 0.50

		// Clamp strictly to [0.20, 3.00] as specified in FR-006
		clamped := math.Max(0.20, math.Min(3.00, rawWeight))
		weights[name] = math.Round(clamped*1000) / 1000
	}

	return weights
}

// RecordOutcome updates Beta posteriors based on trade performance attribution.
func (ts *ThompsonSampler) RecordOutcome(outcome TradeOutcome) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	isWin := outcome.Pnl > 0

	// 1. SuperTrend
	if post, exists := ts.posteriors["SUPERTREND"]; exists {
		stAligned := (outcome.Side == "BUY" && outcome.SuperTrendTrend == "BULL") ||
			(outcome.Side == "SELL" && outcome.SuperTrendTrend == "BEAR")
		if stAligned {
			post.Update(isWin, 1.0)
		}
	}

	// 2. MACD
	if post, exists := ts.posteriors["MACD"]; exists {
		macdAligned := (outcome.Side == "BUY" && outcome.MACDHistogram > 0) ||
			(outcome.Side == "SELL" && outcome.MACDHistogram < 0)
		if macdAligned {
			post.Update(isWin, 1.0)
		}
	}

	// 3. RSI
	if post, exists := ts.posteriors["RSI"]; exists {
		rsiAligned := (outcome.Side == "BUY" && outcome.RSI >= 50) ||
			(outcome.Side == "SELL" && outcome.RSI <= 50)
		if rsiAligned {
			post.Update(isWin, 1.0)
		}
	}

	// 4. Microstructure
	if post, exists := ts.posteriors["MICROSTRUCTURE"]; exists {
		post.Update(isWin, 1.0)
	}
}

// GetPosteriorStats returns current Alpha, Beta, expected win rates, and distribution variance.
func (ts *ThompsonSampler) GetPosteriorStats() map[string]map[string]float64 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	stats := make(map[string]map[string]float64)
	for name, post := range ts.posteriors {
		total := post.Alpha + post.Beta
		variance := 0.0
		if total > 0 {
			// Beta distribution variance: Var(X) = (alpha * beta) / ((alpha + beta)^2 * (alpha + beta + 1))
			variance = (post.Alpha * post.Beta) / (math.Pow(total, 2) * (total + 1.0))
		}
		stats[name] = map[string]float64{
			"alpha":    post.Alpha,
			"beta":     post.Beta,
			"mean":     post.ExpectedValue(),
			"variance": variance,
		}
	}
	return stats
}

// UpdatePosteriors batches custom alpha and beta updates (e.g. from real-data GPU ML training).
func (ts *ThompsonSampler) UpdatePosteriors(updates map[string]struct{ Alpha, Beta float64 }) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for name, param := range updates {
		if post, exists := ts.posteriors[name]; exists {
			post.Alpha = param.Alpha
			post.Beta = param.Beta
		} else {
			ts.posteriors[name] = &BetaPosterior{
				Alpha: param.Alpha,
				Beta:  param.Beta,
			}
		}
	}
}
