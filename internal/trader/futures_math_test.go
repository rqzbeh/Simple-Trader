package trader

import (
	"math"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
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

// TestCalculateFuturesPnLSignsAndFlat locks the sign convention and the
// flat-exit rule required by spec 012 US5 (SC-001): entry == exit must
// produce exactly 0 (not a signed epsilon), LONG gains when price rises,
// SHORT gains when price falls.
func TestCalculateFuturesPnLSignsAndFlat(t *testing.T) {
	const entry = 100.0
	const quantity = 10.0
	const leverage = 8

	cases := []struct {
		name      string
		exit      float64
		direction Direction
		wantPnL   float64
		wantROI   float64
	}{
		{"long win", 101.0, DirectionLong, 10.0, 8.0},
		{"long loss", 99.0, DirectionLong, -10.0, -8.0},
		{"short win", 99.0, DirectionShort, 10.0, 8.0},
		{"short loss", 101.0, DirectionShort, -10.0, -8.0},
		{"flat long", 100.0, DirectionLong, 0.0, 0.0},
		{"flat short", 100.0, DirectionShort, 0.0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pnl, roi, err := CalculateFuturesPnL(entry, tc.exit, quantity, leverage, tc.direction)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(pnl-tc.wantPnL) > 1e-9 || math.Abs(roi-tc.wantROI) > 1e-9 {
				t.Errorf("got (pnl=%.10f, roi=%.10f), want (pnl=%.10f, roi=%.10f)", pnl, roi, tc.wantPnL, tc.wantROI)
			}
		})
	}
}

// --- Spec 012 US2: evidence-based exits (FR-002/003/004/006/007) ---

// TestATRStopDistance locks the ATR-based stop model: sl = SLAtrMult x ATR
// behind the swing low (LONG) / swing high (SHORT) with an ATR offset, then
// clamped to [min%, max%] of price. FR-002.
func TestATRStopDistance(t *testing.T) {
	prof := config.DefaultCryptoProfile() // SLAtrMult 1.5, offset 0.25, clamp [0.6, 2.5]

	// entry 100, swing low 99.0, ATR 1.5
	// raw = 99.0 - 0.25*1.5 = 98.625 -> 1.375% below entry
	// raw within clamp -> slPct = 1.375
	sl, slPct := CalculateATRStop(100, 99.0, 0, 1.5, DirectionLong, prof)
	if math.Abs(sl-98.625) > 1e-9 {
		t.Errorf("long stop = %.6f, want 98.625", sl)
	}
	if math.Abs(slPct-1.375) > 1e-9 {
		t.Errorf("slPct = %.6f, want 1.375", slPct)
	}

	// short: swing high 101 -> raw = 101 + 0.375 = 101.375
	sl, _ = CalculateATRStop(100, 0, 101.0, 1.5, DirectionShort, prof)
	if math.Abs(sl-101.375) > 1e-9 {
		t.Errorf("short stop = %.6f, want 101.375", sl)
	}

	// clamp floor: ATR tiny -> raw ~ entry -> below min 0.6% -> slPct = 0.6
	sl, slPct = CalculateATRStop(100, 99.95, 0, 0.01, DirectionLong, prof)
	if math.Abs(slPct-0.6) > 1e-9 {
		t.Errorf("floor clamp slPct = %.6f, want 0.6", slPct)
	}
	if math.Abs(sl-100*0.994) > 1e-9 {
		t.Errorf("floor stop = %.6f, want %.6f", sl, 100*0.994)
	}

	// clamp ceiling: ATR huge -> beyond max 2.5% -> slPct = 2.5
	_, slPct = CalculateATRStop(100, 96.0, 0, 2.0, DirectionLong, prof)
	if math.Abs(slPct-2.5) > 1e-9 {
		t.Errorf("ceiling clamp slPct = %.6f, want 2.5", slPct)
	}

	// zero ATR (unknown measurement) -> fallback to min stop, never zero
	sl, slPct = CalculateATRStop(100, 0, 0, 0, DirectionLong, prof)
	if sl <= 0 {
		t.Errorf("zero ATR fallback: sl=%.4f, want > 0", sl)
	}
	if slPct < prof.SLMinPct || slPct > prof.SLMaxPct {
		t.Errorf("zero ATR fallback slPct = %.4f, want inside [%v, %v]", slPct, prof.SLMinPct, prof.SLMaxPct)
	}
}

// TestStagedTargetsFromATR locks the staged exit model (research R4, plan G2):
// TP1 = TP1AtrMult x ATR (0.75x = +0.5R at the decay checkpoint level),
// TP2 = TP2AtrMult x ATR, partial close fraction carried from the profile.
func TestStagedTargetsFromATR(t *testing.T) {
	prof := config.DefaultCryptoProfile() // tp1 0.75, tp2 2.3, frac 0.6

	tp1, tp2, frac := CalculateStagedTargets(100, 1.5, DirectionLong, prof)
	if math.Abs(tp1-101.125) > 1e-9 {
		t.Errorf("tp1 = %.6f, want 101.125", tp1)
	}
	if math.Abs(tp2-103.45) > 1e-9 {
		t.Errorf("tp2 = %.6f, want 103.45", tp2)
	}
	if math.Abs(frac-0.6) > 1e-9 {
		t.Errorf("frac = %.6f, want 0.6", frac)
	}

	// short: targets below entry
	tp1, tp2, _ = CalculateStagedTargets(100, 1.5, DirectionShort, prof)
	if math.Abs(tp1-98.875) > 1e-9 || math.Abs(tp2-96.55) > 1e-9 {
		t.Errorf("short targets = %.4f/%.4f, want 98.875/96.55", tp1, tp2)
	}

	// zero ATR -> fallback: TP1 = entry + 1R at min stop (never zero/flat)
	tp1, tp2, _ = CalculateStagedTargets(100, 0, DirectionLong, prof)
	if tp1 <= 100 || tp2 <= tp1 {
		t.Errorf("zero ATR fallback: tp1=%.4f tp2=%.4f, want tp1>entry, tp2>tp1", tp1, tp2)
	}
}

// TestTPBandsInsideEmpiricalMFE validates the TP1 band reachability gate
// (FR-003, SC-002): the empirical favorable-movement (MFE) distribution over
// the holding horizon must put TP1 hit rate inside [40%, 80%].
func TestTPBandsInsideEmpiricalMFE(t *testing.T) {
	cases := []struct {
		name       string
		mfePct     []float64 // favorable excursion % per historical trade window
		tp1Pct     float64
		wantInBand bool
	}{
		{"too easy (94% hit)", []float64{1.0, 1.2, 1.1, 0.9, 1.3, 1.05, 0.95, 1.15, 1.25, 1.0, 1.1, 1.2, 0.85, 1.0, 1.3, 1.1, 1.05, 0.9, 1.15, 1.2}, 0.9, false},
		{"in band (60% hit)", []float64{1.5, 0.5, 1.2, 0.3, 1.8, 0.7, 1.0, 0.4, 2.0, 0.6, 1.3, 0.2, 1.1, 0.8, 1.6, 0.45, 1.4, 0.35, 1.9, 0.55}, 1.0, true},
		{"too hard (10% hit)", []float64{0.2, 0.1, 0.3, 0.15, 0.25, 0.05, 0.4, 0.2, 0.1, 0.3, 0.2, 0.15, 0.5, 0.1, 0.25, 0.35, 0.2, 0.1, 0.3, 0.15}, 1.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hitRate := TP1HitRateFromMFE(tc.mfePct, tc.tp1Pct)
			inBand := hitRate >= 0.40 && hitRate <= 0.80
			if inBand != tc.wantInBand {
				t.Errorf("hit rate = %.2f, inBand = %v, want %v", hitRate, inBand, tc.wantInBand)
			}
		})
	}
}

// TestVolTargetLeverage locks the vol-target leverage formula (FR-006):
// leverage = min(cap, TargetHourlyVolPct / NATR%), never below 1.
func TestVolTargetLeverage(t *testing.T) {
	prof := config.DefaultCryptoProfile() // target 3.5%

	// NATR 0.7% -> 3.5/0.7 = 5x
	if got := CalculateVolTargetLeverage(0.7, 8, prof); got != 5 {
		t.Errorf("leverage = %d, want 5", got)
	}
	// NATR 0.2% -> 17.5 -> capped at 8
	if got := CalculateVolTargetLeverage(0.2, 8, prof); got != 8 {
		t.Errorf("leverage = %d, want cap 8", got)
	}
	// NATR 3.5% -> 1x floor
	if got := CalculateVolTargetLeverage(3.5, 8, prof); got != 1 {
		t.Errorf("leverage = %d, want 1", got)
	}
	// unknown NATR -> floor 1, never 0
	if got := CalculateVolTargetLeverage(0, 8, prof); got != 1 {
		t.Errorf("leverage = %d, want 1 on unknown NATR", got)
	}
}

// TestLiquidationBufferInvariant locks the liquidation-buffer rule (FR-006):
// liq_distance / sl_distance must stay >= LiqBufferMin for the trade to ship.
func TestLiquidationBufferInvariant(t *testing.T) {
	prof := config.DefaultCryptoProfile() // liq_buffer_min 4.0

	// 5x long @100: liq = 100*(1-0.2+0.005)=80.5, dist=19.5.
	// sl 0.6% -> sl dist 0.6 -> buffer 32.5 >= 4 -> OK
	if ok := LiquidationBufferOK(100, 0.6, 5, DirectionLong, prof); !ok {
		t.Errorf("buffer 32.5 should pass >= 4.0")
	}
	// sl 2.5% wide at 5x -> dist 2.5 -> buffer 7.8 >= 4 -> still OK
	if ok := LiquidationBufferOK(100, 2.5, 5, DirectionLong, prof); !ok {
		t.Errorf("buffer 7.8 should pass >= 4.0")
	}
	// 20x long @100: liq dist = 5 - 0.5 = ~4.95... sl 2.5% -> buffer ~1.98 < 4 -> FAIL
	if ok := LiquidationBufferOK(100, 2.5, 20, DirectionLong, prof); ok {
		t.Errorf("buffer ~1.98 must fail < 4.0")
	}
}

// TestSizingSlippageBuffer locks the slippage-adjusted sizing (FR-007):
// risk per coin = |entry - SL| + 0.1% slippage buffer of entry.
func TestSizingSlippageBuffer(t *testing.T) {
	equity := 100000.0
	entry := 100.0
	sl := 99.0 // 1.0% distance
	// risk 1500 -> base qty = 1500/1.0 = 1500 coins
	// with 0.1% buffer: riskDist = 1.0 + 0.1 = 1.1 -> qty = 1363.63
	qty, _, _, err := CalculatePositionSizingWithSlippage(equity, 0.015, 40000.0, entry, sl, 5)
	if err != nil {
		t.Fatalf("sizing failed: %v", err)
	}
	want := 1500.0 / 1.1
	if math.Abs(qty-want) > 0.01 {
		t.Errorf("qty = %.4f, want %.4f (slippage-adjusted)", qty, want)
	}
	// always smaller than un-adjusted sizing
	baseQty, _, _, _ := CalculatePositionSizing(equity, 0.015, 40000.0, entry, sl, 5)
	if qty >= baseQty {
		t.Errorf("slippage-adjusted qty %.4f must be < base %.4f", qty, baseQty)
	}
}

// TestDecayStateMachine locks the decay checkpoint semantics (FR-005):
// before breakeven checkpoint -> NONE; at/after breakeven when PnL < +0.5R
// -> BREAKEVEN (protect); at/after flat checkpoint when PnL <= 0 -> CLOSED.
func TestDecayStateMachine(t *testing.T) {
	prof := config.DefaultCryptoProfile() // BE 30m, flat 40m

	cases := []struct {
		name       string
		ageMin     float64
		rMultiple  float64 // pnl / risk distance
		wantState  string
		wantAction string // "", "BREAKEVEN", "CLOSE"
	}{
		{"young trade untouched", 10, 0.0, "NONE", ""},
		{"young but winning untouched", 20, 1.2, "NONE", ""},
		{"at BE checkpoint below +0.5R -> protect", 30, 0.2, "BREAKEVEN", "BREAKEVEN"},
		{"past BE checkpoint already protected stays", 35, 0.3, "BREAKEVEN", "BREAKEVEN"},
		{"at BE checkpoint already at +0.5R no move", 30, 0.6, "NONE", ""},
		{"at flat checkpoint flat -> close", 40, 0.0, "CLOSED", "CLOSE"},
		{"at flat checkpoint slightly negative -> close", 41, -0.1, "CLOSED", "CLOSE"},
		{"at flat checkpoint profitable -> no close", 40, 0.5, "NONE", ""},
		{"hard horizon always closes", 60, 2.0, "CLOSED", "TIME_EXIT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, action := EvaluateDecay(tc.ageMin, tc.rMultiple, "NONE", prof)
			if state != tc.wantState {
				t.Errorf("state = %q, want %q", state, tc.wantState)
			}
			if action != tc.wantAction {
				t.Errorf("action = %q, want %q", action, tc.wantAction)
			}
		})
	}

	// BREAKEVEN stop price: entry + fees for long (exit = entry * (1 + fee)).
	be := BreakevenStopPrice(100.0, DirectionLong, 0.0005)
	if math.Abs(be-100.05) > 1e-9 {
		t.Errorf("long BE stop = %.6f, want 100.05", be)
	}
	be = BreakevenStopPrice(100.0, DirectionShort, 0.0005)
	if math.Abs(be-99.95) > 1e-9 {
		t.Errorf("short BE stop = %.6f, want 99.95", be)
	}
}
