package market

import (
	"context"
	"fmt"
	"testing"
)

type mockMarketStatsProvider struct {
	stats map[string]struct {
		price     float64
		volume24h float64
		spreadBps float64
		err       error
	}
}

func (m *mockMarketStatsProvider) Get24hStats(symbol string) (float64, float64, float64, error) {
	s, ok := m.stats[symbol]
	if !ok {
		return 0, 0, 0, fmt.Errorf("symbol not found")
	}
	return s.price, s.volume24h, s.spreadBps, s.err
}

func TestDynamicCryptoScreenerEvaluation(t *testing.T) {
	cfg := ScreenerConfig{
		Min24hVolume: 50000000.0, // $50M
		MaxSpreadBps: 10.0,        // 10 bps
		CandidatePairs: []string{
			"BTC/USD", // Qualified (high volume, tight spread)
			"LOWVOL/USD", // Disqualified (volume < $50M)
			"WIDESPREAD/USD", // Disqualified (spread > 10 bps)
		},
	}

	provider := &mockMarketStatsProvider{
		stats: map[string]struct {
			price     float64
			volume24h float64
			spreadBps float64
			err       error
		}{
			"BTC/USD": {
				price:     65000.0,
				volume24h: 1200000000.0,
				spreadBps: 1.2,
			},
			"LOWVOL/USD": {
				price:     2.5,
				volume24h: 15000000.0, // $15M < $50M
				spreadBps: 4.0,
			},
			"WIDESPREAD/USD": {
				price:     10.0,
				volume24h: 80000000.0, // $80M > $50M
				spreadBps: 18.5,       // 18.5 bps > 10 bps
			},
		},
	}

	screener := NewDynamicCryptoScreener(cfg, provider, nil, nil)
	ctx := context.Background()

	evaluated := screener.RunScreeningCycle(ctx)
	if len(evaluated) != 3 {
		t.Fatalf("expected 3 assets evaluated, got %d", len(evaluated))
	}

	for _, asset := range evaluated {
		switch asset.Symbol {
		case "BTC/USD":
			if asset.Status != "ACTIVE" {
				t.Errorf("expected BTC/USD to be ACTIVE, got %s", asset.Status)
			}
		case "LOWVOL/USD":
			if asset.Status != "DISQUALIFIED" {
				t.Errorf("expected LOWVOL/USD to be DISQUALIFIED, got %s", asset.Status)
			}
			if asset.RejectionReason != "volume below $50M threshold" {
				t.Errorf("unexpected rejection reason: %s", asset.RejectionReason)
			}
		case "WIDESPREAD/USD":
			if asset.Status != "DISQUALIFIED" {
				t.Errorf("expected WIDESPREAD/USD to be DISQUALIFIED, got %s", asset.Status)
			}
			if asset.RejectionReason != "spread exceeds 10 bps limit" {
				t.Errorf("unexpected rejection reason: %s", asset.RejectionReason)
			}
		}
	}

	universe := screener.GetActiveUniverse()
	if len(universe) != 1 || universe[0] != "BTC/USD" {
		t.Fatalf("expected active universe [BTC/USD], got %v", universe)
	}
}
