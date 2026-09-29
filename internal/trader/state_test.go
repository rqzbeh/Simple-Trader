package trader

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
)

func evidenceReq() ai.DecisionRequest {
	return ai.DecisionRequest{
		Symbol: "BTC/USDT",
		Quote:  cache.TickerQuote{Symbol: "BTC/USDT", Price: 64000, Change24h: 1.7},
		IndicatorSnap: cache.IndicatorSnapshot{
			RSI: 62, SuperTrend: "BULL", VWAP: 63000, VolumeRatio: 3.1,
			ConfluenceScore: 0.41, Regime: "normal", NATR: 1.9, OBI: 0.2,
		},
		NewsHeadlines: []string{"US SEC follows CFTC in staff guidance for crypto", "BlackRock ETF inflows hit weekly record"},
		CatalystEvents: []ai.CatalystEventInput{{
			Headline: "US SEC follows CFTC in staff guidance for crypto", StoryCount: 4,
			FusedScore: 0.62, Freshness: 0.9, Sources: []string{"cointelegraph", "reuters"},
		}},
	}
}

// TestStateFeedsRealEvidence: the state Jev sees must carry the REAL
// evidence the criteria reference (spec-018 FR-501) — headline TEXTS,
// supertrend, trend, hydrated gates, catalyst cluster meta, policy.
func TestStateFeedsRealEvidence(t *testing.T) {
	pre := map[string]string{"weekend_gap": "clear", "event_blackout": "clear", "polarization": "clear"}
	st, err := BuildEntryState("BTC/USDT", "2026-09-29T12:00:00Z", evidenceReq(), pre)
	if err != nil {
		t.Fatalf("BuildEntryState failed: %v", err)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, want := range []string{
		"US SEC follows CFTC in staff guidance", // headline TEXT, not a count
		"supertrend", "bull",
		"above_vwap",
		"volume_spike",
		"weekend_gap",
		"clear",
		"story_count",
		"fused_sentiment",
		"trading_policy",
		"session_hour_utc",
		"price",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("state JSON missing real evidence %q", want)
		}
	}
	if st.SessionHourUTC != 12 {
		t.Errorf("session hour = %d, want 12", st.SessionHourUTC)
	}
	if st.Sentiment != nil {
		t.Errorf("sentiment must be omitted when not provided, got %+v", st.Sentiment)
	}
	if st.TradingPolicy == "" {
		t.Errorf("trading policy must be present")
	}
}

func TestBuildEntryStateExplicitErrors(t *testing.T) {
	if _, err := BuildEntryState("", "2026-09-29T12:00:00Z", evidenceReq(), nil); err == nil || !strings.Contains(err.Error(), "component=") {
		t.Fatalf("missing symbol must error explicitly: %v", err)
	}
	req := evidenceReq()
	req.IndicatorSnap = cache.IndicatorSnapshot{}
	if _, err := BuildEntryState("BTC", "2026-09-29T12:00:00Z", req, nil); err == nil || !strings.Contains(err.Error(), "missing indicator") {
		t.Fatalf("empty indicators must error: %v", err)
	}
	req = evidenceReq()
	req.Quote = cache.TickerQuote{}
	if _, err := BuildEntryState("BTC", "2026-09-29T12:00:00Z", req, nil); err == nil || !strings.Contains(err.Error(), "price") {
		t.Fatalf("missing price must error: %v", err)
	}
}

func TestSemanticBuckets(t *testing.T) {
	req := evidenceReq()
	b := semanticBuckets(req)
	if b["supertrend"] != "bull" || b["trend"] != "above_vwap" || b["rsi_zone"] != "bullish_momentum" ||
		b["confluence_band"] != "decent" || b["volume_spike"] != "true" {
		t.Errorf("buckets wrong: %v", b)
	}
	if got := rsiZone(25); got != "oversold" {
		t.Errorf("rsiZone(25)=%q", got)
	}
	if got := confluenceBand(0); got != "" {
		t.Errorf("unknown confluence must be omitted, got %q", got)
	}
}
