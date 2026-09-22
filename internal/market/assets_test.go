package market

import (
	"testing"
)

func TestSupportedAssetsClassificationAndInvariants(t *testing.T) {
	assets := GetSupportedAssets()
	if len(assets) == 0 {
		t.Fatal("expected non-empty SupportedAssets")
	}

	seenSymbols := make(map[string]bool)

	for _, a := range assets {
		// 1. Uniqueness
		if seenSymbols[a.Symbol] {
			t.Errorf("duplicate asset symbol found: %s", a.Symbol)
		}
		seenSymbols[a.Symbol] = true

		// 2. Pure Bucketing: must be either CORE or ALPHA
		if a.Bucket != "CORE" && a.Bucket != "ALPHA" {
			t.Errorf("asset %s has invalid bucket: %s (must be CORE or ALPHA)", a.Symbol, a.Bucket)
		}

		// 3. CORE assets must be strictly commodities/precious metals/energy
		if a.Bucket == "CORE" {
			allowedGroups := map[string]bool{
				"GOLD":      true,
				"SILVER":    true,
				"COPPER":    true,
				"PLATINUM":  true,
				"PALLADIUM": true,
				"OIL":       true,
				"ALUMINUM":  true,
			}
			if !allowedGroups[a.ExposureGroup] {
				t.Errorf("CORE asset %s has unapproved commodity exposure group: %s", a.Symbol, a.ExposureGroup)
			}
		}

		// 4. Verification of feed source
		validFeeds := map[string]bool{
			"BINANCE_SPOT":    true,
			"BINANCE_FUTURES": true,
			"BINANCE":         true,
			"KUCOIN":          true,
			"COINEX":          true,
			"YAHOO":           true,
			"TRADINGVIEW":     true,
		}
		if !validFeeds[a.FeedSource] {
			t.Errorf("asset %s has unknown feed source: %s", a.Symbol, a.FeedSource)
		}
	}
}

func TestCommodityExposureGroupsAndBundling(t *testing.T) {
	t.Run("Gold exposure group bundling", func(t *testing.T) {
		goldSymbols := []string{"PAXG/USDT", "XAU/USDT"}
		for _, sym := range goldSymbols {
			grp := GetExposureGroup(sym)
			if grp != "GOLD" {
				t.Errorf("expected %s to have exposure group GOLD, got %s", sym, grp)
			}
		}

		// Pairwise correlation within Gold
		for i := 0; i < len(goldSymbols); i++ {
			for j := 0; j < len(goldSymbols); j++ {
				if !AreCorrelatedCommodities(goldSymbols[i], goldSymbols[j]) {
					t.Errorf("expected %s and %s to be correlated commodities", goldSymbols[i], goldSymbols[j])
				}
			}
		}
	})

	t.Run("Other commodity exposure groups", func(t *testing.T) {
		tests := []struct {
			symbol   string
			expected string
		}{
			{"XAG/USDT", "SILVER"},
			{"COPPER/USDT", "COPPER"},
			{"XPT/USDT", "PLATINUM"},
			{"XPD/USDT", "PALLADIUM"},
			{"OIL/USDT", "OIL"},
			{"ALU/USDT", "ALUMINUM"},
		}

		for _, tc := range tests {
			grp := GetExposureGroup(tc.symbol)
			if grp != tc.expected {
				t.Errorf("expected %s to have exposure group %s, got %s", tc.symbol, tc.expected, grp)
			}
		}
	})

	t.Run("Cross-commodity independence", func(t *testing.T) {
		// Gold and Silver should not correlate
		if AreCorrelatedCommodities("PAXG/USDT", "XAG/USDT") {
			t.Error("PAXG/USDT and XAG/USDT should not be correlated commodities")
		}
		// Gold and Copper should not correlate
		if AreCorrelatedCommodities("PAXG/USDT", "COPPER/USDT") {
			t.Error("PAXG/USDT and COPPER/USDT should not be correlated commodities")
		}
		// Gold and BTC should not correlate
		if AreCorrelatedCommodities("PAXG/USDT", "BTC/USDT") {
			t.Error("PAXG/USDT and BTC/USDT should not be correlated commodities")
		}
	})

	t.Run("GetCorrelatedSymbols returns all instruments in same group", func(t *testing.T) {
		goldCorrelated := GetCorrelatedSymbols("PAXG/USDT")
		if len(goldCorrelated) < 2 {
			t.Errorf("expected at least 2 correlated symbols for PAXG/USDT, got %v", goldCorrelated)
		}

		hasXAU := false
		for _, s := range goldCorrelated {
			if s == "XAU/USDT" {
				hasXAU = true
				break
			}
		}
		if !hasXAU {
			t.Errorf("expected XAU/USDT in correlated symbols of PAXG/USDT, got %v", goldCorrelated)
		}
	})
}

func TestDynamicBucketLookup(t *testing.T) {
	// Commodities must be CORE
	if GetBucket("PAXG/USDT") != "CORE" {
		t.Errorf("expected PAXG/USDT to be CORE, got %s", GetBucket("PAXG/USDT"))
	}
	if GetBucket("COPPER/USDT") != "CORE" {
		t.Errorf("expected COPPER/USDT to be CORE, got %s", GetBucket("COPPER/USDT"))
	}
	if GetBucket("OIL/USDT") != "CORE" {
		t.Errorf("expected OIL/USDT to be CORE, got %s", GetBucket("OIL/USDT"))
	}

	// Crypto must be ALPHA
	if GetBucket("BTC/USDT") != "ALPHA" {
		t.Errorf("expected BTC/USDT to be ALPHA, got %s", GetBucket("BTC/USDT"))
	}
	if GetBucket("ETH/USDT") != "ALPHA" {
		t.Errorf("expected ETH/USDT to be ALPHA, got %s", GetBucket("ETH/USDT"))
	}
	if GetBucket("BNB/USDT") != "ALPHA" {
		t.Errorf("expected BNB/USDT to be ALPHA, got %s", GetBucket("BNB/USDT"))
	}
	if GetBucket("SOL/USDT") != "ALPHA" {
		t.Errorf("expected SOL/USDT to be ALPHA, got %s", GetBucket("SOL/USDT"))
	}
}
