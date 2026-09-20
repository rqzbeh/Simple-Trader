package trader_test

import (
	"math"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/trader"
)

func TestHalfKellyCriterion(t *testing.T) {
	cfg := trader.DefaultKellyConfig()

	// 1. Unfavorable odds: Win probability 30%, Payoff 1.0 -> negative edge
	// Kelly f* = (0.30 * 1 - 0.70) / 1 = -0.40 -> clamps to minRiskPct (0.005 / 0.5%)
	riskNeg := trader.CalculateHalfKelly(cfg, 0.30, 1.0)
	if riskNeg != cfg.MinRiskPct {
		t.Errorf("expected negative edge to clamp to min risk %f, got %f", cfg.MinRiskPct, riskNeg)
	}

	// 2. Strong edge: Win probability 65%, Payoff 2.0 (2:1 reward:risk)
	// Kelly f* = (0.65 * 2 - 0.35) / 2 = (1.30 - 0.35) / 2 = 0.475
	// Half-Kelly = 0.5 * 0.475 = 0.2375 (23.75%)
	// MUST clamp to maxRiskPct (0.02 / 2.0%) to prevent catastrophic drawdown (FR-007)
	riskHigh := trader.CalculateHalfKelly(cfg, 0.65, 2.0)
	if riskHigh != cfg.MaxRiskPct {
		t.Errorf("expected high edge to clamp to max risk ceiling %f, got %f", cfg.MaxRiskPct, riskHigh)
	}

	// 3. Moderate edge: Win probability 52%, Payoff 1.5
	// Kelly f* = (0.52 * 1.5 - 0.48) / 1.5 = (0.78 - 0.48) / 1.5 = 0.20
	// Half-Kelly = 0.5 * 0.20 = 0.10 (10%) -> clamps to max 0.02
	riskMod := trader.CalculateHalfKelly(cfg, 0.52, 1.5)
	if riskMod != 0.02 {
		t.Errorf("expected risk clamped to max 0.02, got %f", riskMod)
	}

	// 4. Slim positive edge: Win probability 51%, Payoff 1.02
	// f* = (0.51 * 1.02 - 0.49) / 1.02 = (0.5202 - 0.49) / 1.02 = 0.0296
	// Half-Kelly = 0.5 * 0.0296 = 0.0148 (1.48%) -> inside [0.005, 0.02]
	riskSlim := trader.CalculateHalfKelly(cfg, 0.51, 1.02)
	if math.Abs(riskSlim-0.0148) > 0.001 {
		t.Errorf("expected slim risk ~0.0148, got %f", riskSlim)
	}
}

func TestAllocatorWithHalfKelly(t *testing.T) {
	allocator := trader.NewAllocator(trader.AllocatorConfig{
		TotalCapital:       100000.0,
		CoreTargetPct:      0.60,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	})

	// Sizing BTC at $60,000, SL at $59,000 (SL distance $1000), TP at $62,000, Win Prob 55%
	btcSize := allocator.CalculateKellyPositionSize("ALPHA", 60000.0, 59000.0, 62000.0, 0.55)
	if btcSize <= 0 {
		t.Fatalf("expected positive BTC size from Half-Kelly, got %f", btcSize)
	}

	// Verify maximum dollar risk does not exceed 2% of total capital ($2,000)
	slDistance := 1000.0
	actualRisk := btcSize * slDistance
	if actualRisk > 2000.0+1e-4 {
		t.Errorf("actual risk %f exceeded maximum allowed 2%% risk of 2000.0", actualRisk)
	}
}
