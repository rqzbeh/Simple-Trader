package trader

import (
	"testing"
)

func TestThreeTierAllocationInit(t *testing.T) {
	cfg := AllocatorConfig{
		TotalCapital:       100000.0,
		Tier1TargetPct:     0.15,
		CoreTargetPct:      0.45,
		AlphaTargetPct:     0.40,
		MaxRiskPerTradePct: 0.02,
	}

	alloc := NewAllocator(cfg)
	tier1Cash := alloc.GetTier1CashReserve()
	if tier1Cash != 15000.0 {
		t.Fatalf("expected Tier 1 Cash to be 15000.0, got %.2f", tier1Cash)
	}

	core, alpha := alloc.GetAvailableBuckets()
	if core != 45000.0 {
		t.Fatalf("expected Core bucket to be 45000.0, got %.2f", core)
	}
	if alpha != 40000.0 {
		t.Fatalf("expected Alpha bucket to be 40000.0, got %.2f", alpha)
	}
}

func TestTier1WithdrawalFulfillment(t *testing.T) {
	cfg := AllocatorConfig{
		TotalCapital:   100000.0,
		Tier1TargetPct: 0.15, // $15,000 in cash
		CoreTargetPct:  0.45,
		AlphaTargetPct: 0.40,
	}
	alloc := NewAllocator(cfg)

	// Withdrawal of $5,000 (within $15,000 buffer) should succeed instantly
	err := alloc.DebitTier1Cash(5000.0)
	if err != nil {
		t.Fatalf("expected withdrawal of 5000 to succeed against 15000 buffer: %v", err)
	}

	if alloc.GetTier1CashReserve() != 10000.0 {
		t.Fatalf("expected remaining Tier 1 Cash to be 10000.0, got %.2f", alloc.GetTier1CashReserve())
	}

	// Attempt withdrawal of $12,000 (exceeds $10,000 remaining) should be rejected
	err = alloc.DebitTier1Cash(12000.0)
	if err == nil {
		t.Fatalf("expected withdrawal exceeding Tier 1 cash to fail")
	}
}

func TestProfitSweepingToTier1(t *testing.T) {
	cfg := AllocatorConfig{
		TotalCapital:   100000.0,
		Tier1TargetPct: 0.15,
	}
	alloc := NewAllocator(cfg)

	// Tactical swing trade wins $2,500 profit; sweep into Tier 1 cash
	alloc.SweepProfitToTier1(2500.0)

	expectedCash := 15000.0 + 2500.0
	if alloc.GetTier1CashReserve() != expectedCash {
		t.Fatalf("expected Tier 1 cash after profit sweep to be %.2f, got %.2f", expectedCash, alloc.GetTier1CashReserve())
	}
}
