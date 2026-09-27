package market

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"sync"
	"time"
)

// News clustering / freshness / polarization (spec 012 US3, research R5):
// near-identical syndicated headlines merge into one Catalyst Event so a
// story picked up by six feeds becomes ONE signal, weighted by source
// authority and discounted by age.

const (
	// ClusterJaccardThreshold: token 3-gram Jaccard at or above this value
	// merges two titles (research R5, validated on syndicated feeds).
	ClusterJaccardThreshold = 0.82
	// ClusterMergeWindow: rolling window in which near-identical titles
	// merge into one cluster (FR-008).
	ClusterMergeWindow = 45 * time.Minute
)

// NormalizeHeadline lowercases, strips punctuation and collapses whitespace.
// Must be idempotent — clustering compares normalized forms.
func NormalizeHeadline(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	prevSpace := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenTrigrams(normalized string) map[string]struct{} {
	tokens := strings.Fields(normalized)
	out := make(map[string]struct{}, len(tokens))
	for i := 0; i+2 < len(tokens); i++ {
		out[tokens[i]+" "+tokens[i+1]+" "+tokens[i+2]] = struct{}{}
	}
	// Very short titles have no trigram; fall back to the token itself so
	// short syndicated heads still cluster instead of never matching.
	if len(out) == 0 && len(tokens) > 0 {
		for _, t := range tokens {
			out[t] = struct{}{}
		}
	}
	return out
}

// TrigramJaccard is the token 3-gram Jaccard similarity of two titles
// (research R5: >= 0.82 means "same story"). Inputs are normalized first.
func TrigramJaccard(a, b string) float64 {
	ta := tokenTrigrams(NormalizeHeadline(a))
	tb := tokenTrigrams(NormalizeHeadline(b))
	if len(ta) == 0 && len(tb) == 0 {
		return 1.0
	}
	inter := 0
	for t := range ta {
		if _, ok := tb[t]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 1.0
	}
	return float64(inter) / float64(union)
}

// SourceTierWeight maps a feed to its authority tier (FR-010):
// 1.0 primary wires / mainstream finance, 0.6 crypto press, 0.2 scrapers.
// Unknown sources degrade to the lowest tier — never 0, which would make
// them vanish from the fused score silently.
func SourceTierWeight(source string) float64 {
	switch strings.ToLower(source) {
	case "reuters", "bloomberg", "wsj", "ft", "yahoo", "yahoo finance", "yahoofinance":
		return 1.0
	case "coindesk", "cointelegraph", "decrypt", "the block", "coingecko":
		return 0.6
	default:
		return 0.2 // scrapers, Google News aggregators, social feeds
	}
}

// FreshnessWeight is the exponential half-life decay of a cluster (FR-009):
// w = exp(-ln2 * age / halfLifeMinutes), discarded (0) past 2x half-life
// (research R5). An invalid half-life returns neutral 1.0 — never divides
// by zero, never discards a live story by accident.
func FreshnessWeight(age time.Duration, halfLifeMinutes float64) float64 {
	if halfLifeMinutes <= 0 {
		return 1.0
	}
	if age < 0 {
		age = 0
	}
	ageMin := age.Minutes()
	if ageMin > 2*halfLifeMinutes {
		return 0
	}
	return math.Exp(-math.Ln2 * ageMin / halfLifeMinutes)
}

// PolarizationScore measures contradictory coverage balance (FR-011):
// 0 = one-sided coverage (tradeable), 1 = perfectly balanced bullish vs
// bearish coverage (contradictory, untradeable). Returns 0 for no coverage
// instead of NaN.
func PolarizationScore(bullCount, bearCount int) float64 {
	total := bullCount + bearCount
	if total <= 0 {
		return 0
	}
	diff := math.Abs(float64(bullCount - bearCount))
	return 1 - diff/float64(total)
}

// PolarizationVetoed is the FR-011 veto rule: balanced contradictory
// coverage above 0.40 means no trade.
func PolarizationVetoed(p float64) bool {
	return p > 0.40
}

// NewsCluster is one Catalyst Event (research R5 / data-model §1).
type NewsCluster struct {
	ID             string    `json:"id"` // fingerprint: stable hash of the normalized head title
	Fingerprint    string    `json:"fingerprint"`
	Headline       string    `json:"headline"` // representative (first) raw title
	Sources        []string  `json:"sources"`
	Headlines      []string  `json:"headlines"`
	StoryCount     int       `json:"story_count"`
	FusedSentiment float64   `json:"fused_sentiment"` // source-tier-weighted mean of member sentiment
	Polarization   float64   `json:"polarization"`    // 0..1, veto above 0.40
	FreshWeight    float64   `json:"fresh_weight"`    // recomputed per evaluation from age
	TierWeightSum  float64   `json:"tier_weight_sum"`
	HalfLifeMin    float64   `json:"half_life_minutes"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`

	memberSentiments []float64 // per-headline sentiment for polarization recompute
}

// effectivePolarization recomputes balance from member sentiments on every
// ingest so late-arriving contradictory coverage tightens the veto.
func (c *NewsCluster) effectivePolarization() float64 {
	bull, bear := 0, 0
	for _, s := range c.memberSentiments {
		switch {
		case s > 0.05:
			bull++
		case s < -0.05:
			bear++
		}
	}
	return PolarizationScore(bull, bear)
}

// Clusterer maintains the rolling 45-minute merge window (FR-008). In-memory
// for the single-process backend: clusters only live for max(window,
// 2x half-life) minutes, so cross-restart persistence buys nothing a Redis
// copy would not (research noted Redis; deferred as an implementation detail
// that does not change observable behavior).
type Clusterer struct {
	mu       sync.Mutex
	window   time.Duration
	halfLife time.Duration
	jaccard  float64
	clusters []*NewsCluster
}

// NewClusterer builds a clusterer whose clusters expire past 2x halfLife.
// The merge window is the fixed 45-minute FR-008 value.
func NewClusterer(halfLife time.Duration) *Clusterer {
	if halfLife <= 0 {
		halfLife = 15 * time.Minute
	}
	return &Clusterer{
		window:   ClusterMergeWindow,
		halfLife: halfLife,
		jaccard:  ClusterJaccardThreshold,
		clusters: make([]*NewsCluster, 0, 8),
	}
}

// Ingest folds one headline into the matching open cluster (token 3-gram
// Jaccard >= 0.82 within the 45m window) or opens a new cluster. Returns
// the cluster the headline landed in.
func (c *Clusterer) Ingest(source, title string, sentiment float64, now time.Time) *NewsCluster {
	c.mu.Lock()
	defer c.mu.Unlock()

	norm := NormalizeHeadline(title)
	if norm == "" {
		return nil
	}

	var target *NewsCluster
	for _, cl := range c.clusters {
		if now.Sub(cl.LastSeen) > c.window {
			continue // outside the rolling merge window
		}
		for _, existing := range cl.Headlines {
			if TrigramJaccard(norm, existing) >= c.jaccard {
				target = cl
				break
			}
		}
	}

	if target == nil {
		tier := SourceTierWeight(source)
		fingerprint := sha256.Sum256([]byte(norm))
		target = &NewsCluster{
			ID:               hex.EncodeToString(fingerprint[:8]),
			Fingerprint:      hex.EncodeToString(fingerprint[:16]),
			Headline:         title,
			Sources:          []string{source},
			Headlines:        []string{title},
			StoryCount:       1,
			FusedSentiment:   sentiment,
			TierWeightSum:    tier,
			memberSentiments: []float64{sentiment},
			HalfLifeMin:      c.halfLife.Minutes(),
			FirstSeen:        now,
			LastSeen:         now,
		}
		target.Polarization = target.effectivePolarization()
		c.clusters = append(c.clusters, target)
		return target
	}

	// Merge: tier-weighted fused sentiment (FR-010), story count, sources.
	target.Sources = append(target.Sources, source)
	target.Headlines = append(target.Headlines, title)
	target.memberSentiments = append(target.memberSentiments, sentiment)
	tier := SourceTierWeight(source)
	target.TierWeightSum += tier
	target.FusedSentiment = (target.FusedSentiment*(target.TierWeightSum-tier) + sentiment*tier) / target.TierWeightSum
	target.StoryCount++
	target.Polarization = target.effectivePolarization()
	if now.After(target.LastSeen) {
		target.LastSeen = now
	}
	if now.Before(target.FirstSeen) {
		target.FirstSeen = now
	}
	return target
}

// Clusters returns live clusters pruned past 2x half-life (FR-009 discard),
// freshest first.
func (c *Clusterer) Clusters(now time.Time) []*NewsCluster {
	c.mu.Lock()
	defer c.mu.Unlock()

	live := c.clusters[:0]
	for _, cl := range c.clusters {
		if now.Sub(cl.LastSeen) <= 2*c.halfLife {
			cl.FreshWeight = FreshnessWeight(now.Sub(cl.LastSeen), c.halfLife.Minutes())
			live = append(live, cl)
		}
	}
	c.clusters = live

	out := make([]*NewsCluster, len(live))
	copy(out, live)
	return out
}
