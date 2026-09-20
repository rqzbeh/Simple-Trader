package db

import (
	"math"
	"testing"
)

func TestCalculateNAV(t *testing.T) {
	tests := []struct {
		name        string
		equity      float64
		units       float64
		expectedNAV float64
	}{
		{
			name:        "Inception with zero units",
			equity:      0,
			units:       0,
			expectedNAV: 1.0,
		},
		{
			name:        "Par value 100k equity for 100k units",
			equity:      100000.0,
			units:       100000.0,
			expectedNAV: 1.0,
		},
		{
			name:        "10% portfolio gain",
			equity:      110000.0,
			units:       100000.0,
			expectedNAV: 1.10,
		},
		{
			name:        "5% portfolio loss",
			equity:      95000.0,
			units:       100000.0,
			expectedNAV: 0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nav := CalculateNAV(tt.equity, tt.units)
			if math.Abs(nav-tt.expectedNAV) > 1e-4 {
				t.Fatalf("expected NAV %.4f, got %.4f", tt.expectedNAV, nav)
			}
		})
	}
}

func TestInvestorNoDilutionMath(t *testing.T) {
	// Scenario:
	// Alice invests $10,000 at inception (NAV = 1.0) -> gets 10,000 units
	aliceUnits := 10000.0 / 1.0
	totalUnits := aliceUnits
	portfolioEquity := 10000.0

	// Portfolio trades and gains +10% ($1,000 profit) -> Total Equity = $11,000
	portfolioEquity = 11000.0
	navAfterGain := CalculateNAV(portfolioEquity, totalUnits)
	if navAfterGain != 1.10 {
		t.Fatalf("expected NAV 1.10, got %.4f", navAfterGain)
	}

	// Alice's equity should be $11,000 (+10% ROI)
	aliceEquity := aliceUnits * navAfterGain
	if aliceEquity != 11000.0 {
		t.Fatalf("expected Alice equity 11000, got %.2f", aliceEquity)
	}

	// Bob enters the pool with $11,000 deposit at NAV = 1.10
	bobDeposit := 11000.0
	bobUnits := bobDeposit / navAfterGain // 10,000 units
	totalUnits += bobUnits
	portfolioEquity += bobDeposit // Total Equity = $22,000

	newNAV := CalculateNAV(portfolioEquity, totalUnits)
	if newNAV != 1.10 {
		t.Fatalf("expected NAV to remain 1.10 after Bob's deposit, got %.4f", newNAV)
	}

	// Check Alice: 10,000 units * 1.10 = $11,000 (ROI +10%)
	aliceNewEquity := aliceUnits * newNAV
	aliceROI := ((aliceNewEquity - 10000.0) / 10000.0) * 100.0
	if aliceNewEquity != 11000.0 || aliceROI != 10.0 {
		t.Fatalf("Alice's profit was diluted! Equity: %.2f, ROI: %.2f", aliceNewEquity, aliceROI)
	}

	// Check Bob: 10,000 units * 1.10 = $11,000 (ROI 0%)
	bobEquity := bobUnits * newNAV
	bobROI := ((bobEquity - bobDeposit) / bobDeposit) * 100.0
	if bobEquity != 11000.0 || bobROI != 0.0 {
		t.Fatalf("Bob's equity incorrect! Equity: %.2f, ROI: %.2f", bobEquity, bobROI)
	}
}

func TestInvestorWithdrawalMath(t *testing.T) {
	// Investor has 10,000 units at NAV = 1.10 ($11,000 equity)
	units := 10000.0
	nav := 1.10

	// Withdraws $2,200
	withdrawAmount := 2200.0
	unitsRedeemed := withdrawAmount / nav // 2,000 units
	remainingUnits := units - unitsRedeemed // 8,000 units

	remainingEquity := remainingUnits * nav // $8,800
	totalDeposited := 10000.0
	totalWithdrawn := 2200.0

	netProfit := remainingEquity + totalWithdrawn - totalDeposited
	roi := (netProfit / totalDeposited) * 100.0

	if remainingEquity != 8800.0 {
		t.Fatalf("expected remaining equity 8800, got %.2f", remainingEquity)
	}
	if netProfit != 1000.0 {
		t.Fatalf("expected net profit 1000, got %.2f", netProfit)
	}
	if math.Abs(roi-10.0) > 1e-4 {
		t.Fatalf("expected ROI 10%%, got %.2f%%", roi)
	}
}
