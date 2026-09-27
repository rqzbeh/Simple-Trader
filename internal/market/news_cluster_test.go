package market

import (
	"math"
	"testing"
	"time"
)

// --- Spec 012 US3: news fusion (FR-008/009/010/011) ---

func TestNormalizeHeadline(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bitcoin Hits NEW Record High!!!", "bitcoin hits new record high"},
		{"  Ethereum (ETH) Soars 5% — CoinDesk", "ethereum eth soars 5 coindesk"},
		{"BREAKING: Fed cuts rates by 50bps", "breaking fed cuts rates by 50bps"},
	}
	for _, tc := range cases {
		got := NormalizeHeadline(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeHeadline(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Normalization must be idempotent (clustering compares normalized forms).
	once := NormalizeHeadline("Bitcoin Hits NEW Record High!!!")
	twice := NormalizeHeadline(NormalizeHeadline("Bitcoin Hits NEW Record High!!!"))
	if once != twice {
		t.Errorf("not idempotent: %q != %q", once, twice)
	}
}

func TestTrigramJaccard(t *testing.T) {
	// Identical normalized titles -> 1.0
	if j := TrigramJaccard("bitcoin hits new record high", "bitcoin hits new record high"); j < 0.99 {
		t.Errorf("identical titles jaccard = %.3f, want ~1.0", j)
	}
	// Syndicated retitle (outlet suffix appended) -> >= 0.82 (threshold)
	same := TrigramJaccard(
		"bitcoin etf inflows surge to record 1 billion",
		"bitcoin etf inflows surge to record 1 billion coindesk",
	)
	if same < 0.82 {
		t.Errorf("near-identical story jaccard = %.3f, want >= 0.82", same)
	}
	// Different stories -> well below threshold
	diff := TrigramJaccard(
		"bitcoin etf inflows surge to record 1 billion",
		"sec approves spot ethereum etf application",
	)
	if diff >= 0.82 {
		t.Errorf("different stories jaccard = %.3f, want < 0.82", diff)
	}
}

func TestSourceTierWeights(t *testing.T) {
	cases := map[string]float64{
		"YahooFinance":  1.0, // primary wire / mainstream finance
		"CoinDesk":      0.6, // crypto press
		"CoinTelegraph": 0.6,
		"Decrypt":       0.6,
		"WhaleAlerts":   0.2, // scraper / aggregator
		"OnChainWhales": 0.2,
	}
	for src, want := range cases {
		if got := SourceTierWeight(src); got != want {
			t.Errorf("SourceTierWeight(%q) = %.1f, want %.1f", src, got, want)
		}
	}
	// Unknown source degrades to the lowest tier, never 0 (a zero-weighted
	// source would silently vanish from the fused score).
	if got := SourceTierWeight("SomeRandomBlog"); got != 0.2 {
		t.Errorf("unknown source tier = %.1f, want 0.2", got)
	}
}

func TestFreshnessWeightDecay(t *testing.T) {
	// Half-life 15m (crypto): w = exp(-ln2 * age / 15m)
	w0 := FreshnessWeight(0, 15)
	if math.Abs(w0-1.0) > 1e-9 {
		t.Errorf("age 0 weight = %.4f, want 1.0", w0)
	}
	w1 := FreshnessWeight(15*time.Minute, 15)
	if math.Abs(w1-0.5) > 1e-9 {
		t.Errorf("age=half-life weight = %.4f, want 0.5", w1)
	}
	w2 := FreshnessWeight(20*time.Minute, 15) // 20m < 2x15m: decaying, not discarded
	if w2 >= w1 || w2 <= 0 || w2 >= 0.5 {
		t.Errorf("age 20m weight = %.4f, want in (0, 0.5)", w2)
	}
	// Discard past 2x half-life (research R5): weight must be 0.
	wPast := FreshnessWeight(31*time.Minute, 15)
	if wPast != 0 {
		t.Errorf("past 2x half-life weight = %.4f, want 0 (discarded)", wPast)
	}
	// Commodity half-life 60m: still fresh at 31m.
	if got := FreshnessWeight(31*time.Minute, 60); got <= 0 {
		t.Errorf("commodity 31m weight = %.4f, want > 0", got)
	}
	// Zero/invalid half-life: neutral (never divide by zero).
	if got := FreshnessWeight(10*time.Minute, 0); got != 1.0 {
		t.Errorf("zero half-life weight = %.4f, want 1.0", got)
	}
}

func TestPolarizationScore(t *testing.T) {
	// One-sided coverage: no contradiction -> 0 (no veto).
	if p := PolarizationScore(8, 0); p != 0 {
		t.Errorf("one-sided P = %.3f, want 0", p)
	}
	// Near-balanced coverage: P -> 1 (veto at > 0.40).
	if p := PolarizationScore(5, 5); p < 0.99 {
		t.Errorf("balanced P = %.3f, want ~1.0", p)
	}
	// 60/40 split -> P = 1 - 0.2/1 = 0.8 > 0.40 -> veto territory.
	if p := PolarizationScore(6, 4); p <= 0.40 {
		t.Errorf("60/40 P = %.3f, want > 0.40 (veto)", p)
	}
	// 80/20 split -> P = 0.4 -> exactly at the threshold, not above it.
	if p := PolarizationScore(8, 2); p > 0.40 {
		t.Errorf("80/20 P = %.3f, want <= 0.40", p)
	}
	// No coverage at all: neutral, never NaN.
	if p := PolarizationScore(0, 0); p != 0 {
		t.Errorf("empty P = %.3f, want 0", p)
	}
	// Veto rule helper (FR-011).
	if PolarizationVetoed(0.41) != true {
		t.Errorf("P=0.41 must veto")
	}
	if PolarizationVetoed(0.39) != false {
		t.Errorf("P=0.39 must not veto")
	}
}

func TestClustererSyndicatedDuplicates(t *testing.T) {
	// FR-008: five near-identical headlines within the 45m window merge into
	// ONE cluster with story_count = 5 (research R5: kills the 6x-duplicate-
	// long failure).
	c := NewClusterer(15 * time.Minute)
	now := time.Now()

	headlines := []struct {
		src, title string
		sent       float64
	}{
		{"YahooFinance", "Bitcoin Hits Record High as ETF Inflows Surge Past $1B", 0.7},
		{"CoinDesk", "Bitcoin hits record high as ETF inflows surge past $1b", 0.7},
		{"CoinTelegraph", "BITCOIN HITS RECORD HIGH AS ETF INFLOWS SURGE PAST $1B!", 0.6},
		{"Decrypt", "bitcoin hits record high as etf inflows surge past $1b - coindesk", 0.7},
		{"WhaleAlerts", "(Bitcoin) Hits Record High as ETF Inflows Surge Past $1B", 0.5},
	}
	for i, h := range headlines {
		c.Ingest(h.src, h.title, h.sent, now.Add(time.Duration(i)*time.Minute))
	}

	clusters := c.Clusters(now)
	if len(clusters) != 1 {
		t.Fatalf("clusters = %d, want 1 (syndicated duplicates merge)", len(clusters))
	}
	if clusters[0].StoryCount != 5 {
		t.Errorf("story_count = %d, want 5", clusters[0].StoryCount)
	}
	if clusters[0].Headline == "" {
		t.Errorf("cluster must keep a representative headline")
	}
	if len(clusters[0].Sources) != 5 {
		t.Errorf("sources = %v, want 5 entries", clusters[0].Sources)
	}
}

func TestClustererDistinctStoriesStaySeparate(t *testing.T) {
	c := NewClusterer(15 * time.Minute)
	now := time.Now()
	c.Ingest("CoinDesk", "Bitcoin Hits Record High as ETF Inflows Surge", 0.7, now)
	c.Ingest("CoinTelegraph", "SEC Approves Spot Ethereum ETF Application", 0.8, now.Add(time.Minute))
	c.Ingest("Decrypt", "Ethereum Staking Yields Fall Below 3%", -0.3, now.Add(2*time.Minute))

	if got := len(c.Clusters(now)); got != 3 {
		t.Errorf("clusters = %d, want 3 (distinct stories separate)", got)
	}
}

func TestClustererMergeWindowExpires(t *testing.T) {
	// FR-008: rolling 45-minute window — a headline older than the window
	// starts a NEW cluster, it does not merge with the ancient one.
	// Long half-life (360m) so the 30m/2x-half-life prune cannot mask the
	// 45-minute merge-window rule under test.
	c := NewClusterer(180 * time.Minute)
	now := time.Now()
	c.Ingest("CoinDesk", "Bitcoin Hits Record High as ETF Inflows Surge", 0.7, now)
	// Same title 46 minutes later: outside the 45m merge window.
	c.Ingest("CoinDesk", "Bitcoin Hits Record High as ETF Inflows Surge", 0.7, now.Add(46*time.Minute))

	if got := len(c.Clusters(now.Add(46 * time.Minute))); got != 2 {
		t.Errorf("clusters = %d, want 2 (46m apart exceeds 45m window)", got)
	}
	// Inside the window (10m apart) merges instead.
	c2 := NewClusterer(180 * time.Minute)
	c2.Ingest("CoinDesk", "Bitcoin Hits Record High as ETF Inflows Surge", 0.7, now)
	c2.Ingest("CoinDesk", "Bitcoin Hits Record High as ETF Inflows Surge", 0.7, now.Add(10*time.Minute))
	if got := len(c2.Clusters(now.Add(10 * time.Minute))); got != 1 {
		t.Errorf("clusters = %d, want 1 (10m apart merges within 45m window)", got)
	}
}

func TestClustererPrunePastHalfLife(t *testing.T) {
	c := NewClusterer(15 * time.Minute)
	now := time.Now()
	c.Ingest("CoinDesk", "Bitcoin Hits Record High", 0.7, now)

	// Fresh within 2x half-life (30m).
	if got := len(c.Clusters(now.Add(25 * time.Minute))); got != 1 {
		t.Errorf("clusters at 25m = %d, want 1", got)
	}
	// Discarded past 2x half-life (30m).
	if got := len(c.Clusters(now.Add(31 * time.Minute))); got != 0 {
		t.Errorf("clusters at 31m = %d, want 0 (past 2x half-life discarded)", got)
	}
}

func TestClustererFusedSentimentIsTierWeighted(t *testing.T) {
	// FR-010: tier weights 1.0/0.6/0.2 scale each source's contribution.
	c := NewClusterer(15 * time.Minute)
	now := time.Now()
	c.Ingest("YahooFinance", "Bitcoin Hits Record High", 1.0, now)                 // tier 1.0
	c.Ingest("CoinDesk", "BITCOIN HITS RECORD HIGH!!!", 1.0, now.Add(time.Second)) // tier 0.6

	clusters := c.Clusters(now)
	if len(clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(clusters))
	}
	// Weighted mean of (1.0 x 1.0) and (0.6 x 1.0) / 1.6 = 1.0.
	if math.Abs(clusters[0].FusedSentiment-1.0) > 1e-9 {
		t.Errorf("fused sentiment = %.3f, want 1.0", clusters[0].FusedSentiment)
	}
	if math.Abs(clusters[0].TierWeightSum-1.6) > 1e-9 {
		t.Errorf("tier weight sum = %.3f, want 1.6", clusters[0].TierWeightSum)
	}
}
