package trader

import (
	"math"
	"testing"
)

func TestCalculateLiquidationPrice(t *testing.T) {
	entry := 60000.0
	leverage := 10
	mmr := 0.005 // 0.5%

	// Long liquidation: Entry * (1 - 1/L + MMR)
	// 60000 * (1 - 0.1 + 0.005) = 60000 * 0.905 = 54300
	longLiq, err := CalculateLiquidationPrice(entry, leverage, DirectionLong, mmr)
	if err != nil {
		t.Fatalf("unexpected error for long liq: %v", err)
	}
	expectedLongLiq := 54300.0
	if math.Abs(longLiq-expectedLongLiq) > 0.01 {
		t.Errorf("long liq price mismatch: expected %.2f, got %.2f", expectedLongLiq, longLiq)
	}

	// Short liquidation: Entry * (1 + 1/L - MMR)
	// 60000 * (1 + 0.1 - 0.005) = 60000 * 1.095 = 65700
	shortLiq, err := CalculateLiquidationPrice(entry, leverage, DirectionShort, mmr)
	if err != nil {
		t.Fatalf("unexpected error for short liq: %v", err)
	}
	expectedShortLiq := 65700.0
	if math.Abs(shortLiq-expectedShortLiq) > 0.01 {
		t.Errorf("short liq price mismatch: expected %.2f, got %.2f", expectedShortLiq, shortLiq)
	}
}

func TestCalculateFuturesPnL(t *testing.T) {
	entry := 50000.0
	quantity := 2.0 // position value = 100,000 USD
	leverage := 5   // isolated margin = 20,000 USD

	// Test 1: Profitable Long
	// Exit = 52000 (+4% price move -> +20% ROI)
	pnl, roi, err := CalculateFuturesPnL(entry, 52000.0, quantity, leverage, DirectionLong)
	if err != nil {
		t.Fatalf("CalculateFuturesPnL failed: %v", err)
	}
	expectedPnL := 4000.0
	expectedROI := 20.0
	if math.Abs(pnl-expectedPnL) > 0.01 || math.Abs(roi-expectedROI) > 0.01 {
		t.Errorf("long pnl/roi mismatch: got (%.2f, %.2f%%), expected (%.2f, %.2f%%)", pnl, roi, expectedPnL, expectedROI)
	}

	// Test 2: Profitable Short
	// Exit = 48000 (-4% price move -> +20% ROI)
	pnlShort, roiShort, err := CalculateFuturesPnL(entry, 48000.0, quantity, leverage, DirectionShort)
	if err != nil {
		t.Fatalf("CalculateFuturesPnL short failed: %v", err)
	}
	if math.Abs(pnlShort-expectedPnL) > 0.01 || math.Abs(roiShort-expectedROI) > 0.01 {
		t.Errorf("short pnl/roi mismatch: got (%.2f, %.2f%%), expected (%.2f, %.2f%%)", pnlShort, roiShort, expectedPnL, expectedROI)
	}

	// Test 3: Losing Short
	// Exit = 51000 (+2% price move -> -10% ROI)
	pnlLoss, roiLoss, err := CalculateFuturesPnL(entry, 51000.0, quantity, leverage, DirectionShort)
	if err != nil {
		t.Fatalf("CalculateFuturesPnL short loss failed: %v", err)
	}
	if math.Abs(pnlLoss - -2000.0) > 0.01 || math.Abs(roiLoss - -10.0) > 0.01 {
		t.Errorf("short loss mismatch: got (%.2f, %.2f%%), expected (-2000.00, -10.00%%)", pnlLoss, roiLoss)
	}
}

func TestCalculateRiskRewardRatio(t *testing.T) {
	// Long trade: Entry 100, Stop 95, TP 110 -> Risk = 5, Reward = 10 -> R:R = 2.0
	rrLong, err := CalculateRiskRewardRatio(100.0, 95.0, 110.0, DirectionLong)
	if err != nil {
		t.Fatalf("unexpected error on long R:R: %v", err)
	}
	if math.Abs(rrLong-2.0) > 0.001 {
		t.Errorf("expected R:R 2.0, got %.3f", rrLong)
	}

	// Short trade: Entry 100, Stop 105, TP 90 -> Risk = 5, Reward = 10 -> R:R = 2.0
	rrShort, err := CalculateRiskRewardRatio(100.0, 105.0, 90.0, DirectionShort)
	if err != nil {
		t.Fatalf("unexpected error on short R:R: %v", err)
	}
	if math.Abs(rrShort-2.0) > 0.001 {
		t.Errorf("expected R:R 2.0, got %.3f", rrShort)
	}

	// Invalid Short trade: TP higher than entry
	_, err = CalculateRiskRewardRatio(100.0, 105.0, 102.0, DirectionShort)
	if err == nil {
		t.Errorf("expected error for invalid short TP, got nil")
	}
}

func TestCalculatePositionSizing(t *testing.T) {
	equity := 100000.0
	maxRiskPct := 0.02 // 2.0% -> $2,000 max risk
	availableCapital := 40000.0
	entry := 50000.0
	stopLoss := 48000.0 // distance = 2,000 per coin
	leverage := 5

	qty, marginReq, riskUSD, err := CalculatePositionSizing(
		equity, maxRiskPct, availableCapital, entry, stopLoss, leverage,
	)
	if err != nil {
		t.Fatalf("unexpected error sizing position: %v", err)
	}

	// Risk USD should be $2,000
	if math.Abs(riskUSD-2000.0) > 0.01 {
		t.Errorf("expected risk USD 2000, got %.2f", riskUSD)
	}

	// Qty = 2000 / 2000 = 1.0 BTC
	if math.Abs(qty-1.0) > 0.001 {
		t.Errorf("expected qty 1.0, got %.3f", qty)
	}

	// Position value = 1.0 * 50,000 = 50,000. Margin at 5x = 10,000 USD
	if math.Abs(marginReq-10000.0) > 0.01 {
		t.Errorf("expected margin 10000, got %.2f", marginReq)
	}
}
