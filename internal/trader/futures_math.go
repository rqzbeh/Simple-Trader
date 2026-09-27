package trader

import (
	"errors"
	"math"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// Direction represents futures trade direction.
type Direction string

const (
	DirectionLong  Direction = "LONG"
	DirectionShort Direction = "SHORT"
)

// FuturesContractSpec encapsulates calculations for isolated margin futures contracts.
type FuturesContractSpec struct {
	Direction             Direction `json:"direction"`
	EntryPrice            float64   `json:"entry_price"`
	StopLoss              float64   `json:"stop_loss"`
	TakeProfit1           float64   `json:"take_profit_1"`
	TakeProfit2           *float64  `json:"take_profit_2,omitempty"`
	Leverage              int       `json:"leverage"`
	MaintenanceMarginRate float64   `json:"maintenance_margin_rate"` // e.g. 0.005 for 0.5%
}

// CalculateLiquidationPrice computes the isolated margin liquidation price.
// Long:  Entry * (1 - 1/L + MMR)
// Short: Entry * (1 + 1/L - MMR)
func CalculateLiquidationPrice(entry float64, leverage int, direction Direction, mmr float64) (float64, error) {
	if entry <= 0 {
		return 0, errors.New("entry price must be positive")
	}
	if leverage < 1 || leverage > 125 {
		return 0, errors.New("leverage must be between 1x and 125x")
	}
	if mmr < 0 || mmr >= 1.0 {
		mmr = 0.005 // default 0.5%
	}

	levFactor := 1.0 / float64(leverage)

	switch direction {
	case DirectionLong:
		liqPrice := entry * (1.0 - levFactor + mmr)
		if liqPrice < 0 {
			liqPrice = 0
		}
		return liqPrice, nil
	case DirectionShort:
		liqPrice := entry * (1.0 + levFactor - mmr)
		return liqPrice, nil
	default:
		return 0, errors.New("invalid direction: must be LONG or SHORT")
	}
}

// CalculateFuturesPnL computes net realized PnL and return on isolated margin (ROI %).
// Long PnL:  Quantity * (ExitPrice - EntryPrice)
// Short PnL: Quantity * (EntryPrice - ExitPrice)
// Margin:    (Quantity * EntryPrice) / Leverage
// ROI %:     (PnL / Margin) * 100 = +/- ((Exit - Entry)/Entry) * Leverage * 100
func CalculateFuturesPnL(entry, exit float64, quantity float64, leverage int, direction Direction) (pnlUSD, roiPct float64, err error) {
	if entry <= 0 || exit <= 0 || quantity <= 0 {
		return 0, 0, errors.New("entry, exit prices, and quantity must be positive")
	}
	if leverage < 1 {
		return 0, 0, errors.New("leverage must be at least 1")
	}

	marginUSD := (quantity * entry) / float64(leverage)
	if marginUSD <= 0 {
		return 0, 0, errors.New("calculated margin must be positive")
	}

	switch direction {
	case DirectionLong:
		pnlUSD = quantity * (exit - entry)
	case DirectionShort:
		pnlUSD = quantity * (entry - exit)
	default:
		return 0, 0, errors.New("direction must be LONG or SHORT")
	}

	roiPct = (pnlUSD / marginUSD) * 100.0
	return pnlUSD, roiPct, nil
}

// CalculateRiskRewardRatio calculates the R:R ratio based on Entry, Stop Loss, and Take Profit.
// R:R = |TP - Entry| / |Entry - SL|
func CalculateRiskRewardRatio(entry, stopLoss, takeProfit float64, direction Direction) (float64, error) {
	if entry <= 0 || stopLoss <= 0 || takeProfit <= 0 {
		return 0, errors.New("all price levels must be positive")
	}

	switch direction {
	case DirectionLong:
		if stopLoss >= entry {
			return 0, errors.New("long stop loss must be strictly below entry price")
		}
		if takeProfit <= entry {
			return 0, errors.New("long take profit must be strictly above entry price")
		}
	case DirectionShort:
		if stopLoss <= entry {
			return 0, errors.New("short stop loss must be strictly above entry price")
		}
		if takeProfit >= entry {
			return 0, errors.New("short take profit must be strictly below entry price")
		}
	default:
		return 0, errors.New("invalid trade direction")
	}

	riskDistance := math.Abs(entry - stopLoss)
	rewardDistance := math.Abs(takeProfit - entry)

	if riskDistance == 0 {
		return 0, errors.New("risk distance cannot be zero")
	}

	rrRatio := rewardDistance / riskDistance
	return rrRatio, nil
}

// CalculatePositionSizing computes contract quantity ensuring max risk <= maxRiskPct (e.g. 2.0% of portfolio equity).
// Risk Amount = Equity * maxRiskPct
// Risk Per Coin = |Entry - SL|
// Quantity = Risk Amount / Risk Per Coin
// Position Value = Quantity * Entry
// Margin Required = Position Value / Leverage
// Returns quantity, marginRequired, riskAmountUSD, or error if constraints violated.
func CalculatePositionSizing(
	equity float64,
	maxRiskPct float64, // e.g. 0.02 for 2.0%
	availableAlphaCapital float64,
	entry float64,
	stopLoss float64,
	leverage int,
) (quantity, marginRequired, riskAmountUSD float64, err error) {
	if equity <= 0 || availableAlphaCapital <= 0 {
		return 0, 0, 0, errors.New("equity and available capital must be positive")
	}
	if maxRiskPct <= 0 || maxRiskPct > 0.10 {
		maxRiskPct = 0.02 // default 2.0% safety cap, max 10%
	}
	if leverage < 1 {
		leverage = 1
	}

	riskDistance := math.Abs(entry - stopLoss)
	if riskDistance <= 0 {
		return 0, 0, 0, errors.New("stop loss cannot be equal to entry price")
	}

	riskAmountUSD = equity * maxRiskPct
	quantity = riskAmountUSD / riskDistance
	positionValue := quantity * entry
	marginRequired = positionValue / float64(leverage)

	// Enforce margin does not exceed available Tier 2 Tactical Alpha
	if marginRequired > availableAlphaCapital {
		// Scale down quantity to fit available margin
		marginRequired = availableAlphaCapital
		positionValue = marginRequired * float64(leverage)
		quantity = positionValue / entry
		riskAmountUSD = quantity * riskDistance
	}

	return quantity, marginRequired, riskAmountUSD, nil
}

// --- Spec 012 US2: evidence-based exits (FR-002/003/005/006/007) ---

// CalculateATRStop derives the structural stop-loss price from measured
// volatility (FR-002): the stop sits SLAtrMult x ATR behind the 15m swing
// (swing low for LONG, swing high for SHORT) plus an SLSwingOffset x ATR
// buffer, then clamps to [SLMinPct, SLMaxPct] of the entry price. A missing
// ATR measurement falls back to the profile's minimum stop — never zero.
// Returns the stop price and the stop distance as % of entry (for sizing,
// the entry gate and audit columns).
func CalculateATRStop(entry, swingLow, swingHigh, atr float64, dir Direction, prof config.RiskProfile) (stopPrice, slPct float64) {
	var raw float64
	switch dir {
	case DirectionLong:
		anchor := swingLow
		if anchor <= 0 && atr > 0 {
			// No swing known: pure ATR below entry. atr is ABSOLUTE (price
			// units) — anchor = entry - mult x atr.
			anchor = entry - prof.SLAtrMult*atr
		}
		if anchor > 0 && atr > 0 {
			raw = anchor - prof.SLSwingOffset*atr
		} else {
			raw = anchor
		}
	default:
		anchor := swingHigh
		if anchor <= 0 && atr > 0 {
			anchor = entry + prof.SLAtrMult*atr
		}
		if anchor > 0 && atr > 0 {
			raw = anchor + prof.SLSwingOffset*atr
		} else {
			raw = anchor
		}
	}

	pct := math.Abs(entry-raw) / entry * 100.0
	if pct < prof.SLMinPct {
		pct = prof.SLMinPct
	}
	if pct > prof.SLMaxPct {
		pct = prof.SLMaxPct
	}

	if dir == DirectionLong {
		stopPrice = entry * (1.0 - pct/100.0)
	} else {
		stopPrice = entry * (1.0 + pct/100.0)
	}
	return stopPrice, pct
}

// CalculateStagedTargets returns the staged take-profit prices from measured
// volatility (research R4, plan G2): TP1 = TP1AtrMult x ATR, TP2 = TP2AtrMult
// x ATR beyond entry, with TP1CloseFrac from the profile. The hard R:R floor
// is intentionally dropped: staged exits measure reachability, not a fixed
// ratio. A missing ATR falls back to TP1 = entry + 1R at the min stop.
func CalculateStagedTargets(entry, atr float64, dir Direction, prof config.RiskProfile) (tp1, tp2, closeFrac float64) {
	closeFrac = prof.TP1CloseFrac

	if atr <= 0 {
		// Fallback: TP1 = 1R at the profile's minimum stop distance, TP2 = 2R.
		// Callers with a wider effective stop can stretch TP1 themselves.
		riskDist := entry * prof.SLMinPct / 100.0
		if dir == DirectionLong {
			return entry + riskDist, entry + 2*riskDist, closeFrac
		}
		return entry - riskDist, entry - 2*riskDist, closeFrac
	}

	if dir == DirectionLong {
		return entry + prof.TP1AtrMult*atr, entry + prof.TP2AtrMult*atr, closeFrac
	}
	return entry - prof.TP1AtrMult*atr, entry - prof.TP2AtrMult*atr, closeFrac
}

// TP1HitRateFromMFE computes the historical hit probability of a TP1 target:
// the share of recorded favorable-excursion windows whose maximum favorable
// excursion (MFE) reached the target. FR-003 uses this to validate the TP1
// band: target must land with hit rate in [40%, 80%].
func TP1HitRateFromMFE(mfePct []float64, tp1Pct float64) float64 {
	if len(mfePct) == 0 {
		return 0
	}
	hits := 0
	for _, mfe := range mfePct {
		if mfe >= tp1Pct {
			hits++
		}
	}
	return float64(hits) / float64(len(mfePct))
}

// CalculateVolTargetLeverage derives per-asset leverage from a
// target-volatility formula (FR-006): leverage = min(cap,
// TargetHourlyVolPct / NATR%), floored at 1x. natrPct is the normalized ATR
// as a percent of price (e.g. 0.7 = 0.7%).
func CalculateVolTargetLeverage(natrPct float64, cap int, prof config.RiskProfile) int {
	if natrPct <= 0 || prof.TargetHourlyVolPct <= 0 {
		return 1
	}
	raw := prof.TargetHourlyVolPct / natrPct
	if cap < 1 {
		cap = 1
	}
	lev := int(math.Floor(raw))
	if lev < 1 {
		lev = 1
	}
	if lev > cap {
		lev = cap
	}
	return lev
}

// LiquidationBufferOK verifies the liquidation-buffer invariant (FR-006):
// distance-to-liquidation / stop-distance must be >= LiqBufferMin, else the
// trade risks stop-then-liquidation whipsaw and must not ship.
func LiquidationBufferOK(entry, slPct float64, leverage int, dir Direction, prof config.RiskProfile) bool {
	liq, err := CalculateLiquidationPrice(entry, leverage, dir, 0.005)
	if err != nil {
		return false
	}
	liqDist := math.Abs(entry-liq) / entry * 100.0
	slDist := slPct
	if slDist <= 0 {
		return false
	}
	return liqDist/slDist >= prof.LiqBufferMin
}

// CalculatePositionSizingWithSlippage sizes by fixed-fractional risk with a
// slippage buffer (FR-007): risk per coin = |entry - SL| + slippagePct x
// entry, so the realized risk stays inside the budget even when the stop
// fills through slippage. Tier budget stays a cap (not a fixed slot) — same
// clamp semantics as CalculatePositionSizing.
func CalculatePositionSizingWithSlippage(
	equity float64,
	maxRiskPct float64,
	availableAlphaCapital float64,
	entry float64,
	stopLoss float64,
	leverage int,
) (quantity, marginRequired, riskAmountUSD float64, err error) {
	// 10 bps default slippage buffer on the stop fill.
	return CalculatePositionSizingWithSlippageRate(equity, maxRiskPct, availableAlphaCapital, entry, stopLoss, leverage, 0.001)
}

// CalculatePositionSizingWithSlippageRate is the parameterized core of
// CalculatePositionSizingWithSlippage (slippagePct as fraction of entry).
func CalculatePositionSizingWithSlippageRate(
	equity float64,
	maxRiskPct float64,
	availableAlphaCapital float64,
	entry float64,
	stopLoss float64,
	leverage int,
	slippagePct float64,
) (quantity, marginRequired, riskAmountUSD float64, err error) {
	if entry <= 0 || stopLoss <= 0 {
		return 0, 0, 0, errors.New("entry and stop loss must be positive")
	}
	baseDist := math.Abs(entry - stopLoss)
	adjustedSL := stopLoss
	if entry > stopLoss {
		adjustedSL = entry - baseDist - slippagePct*entry
	} else {
		adjustedSL = entry + baseDist + slippagePct*entry
	}
	return CalculatePositionSizing(equity, maxRiskPct, availableAlphaCapital, entry, adjustedSL, leverage)
}

// BreakevenStopPrice returns the break-even stop after the TP1 partial fill:
// entry price moved past round-trip taker fees (research R4: "entry+fees").
// feeRate is the round-trip taker fee as a fraction (e.g. 0.001 = 2 x 5 bps).
func BreakevenStopPrice(entry float64, dir Direction, feeRate float64) float64 {
	if feeRate < 0 {
		feeRate = 0.001
	}
	if dir == DirectionLong {
		return entry * (1.0 + feeRate)
	}
	return entry * (1.0 - feeRate)
}

// EvaluateDecay advances the time-decay state machine for one ACTIVE signal
// (FR-005, research R5): before the breakeven checkpoint the trade is
// untouched; at/after the breakeven checkpoint a trade that has not reached
// +0.5R is protected (stop to break-even); at/after the flat checkpoint a
// still-unprofitable trade is closed before the full horizon; the hard
// horizon always closes. currentState is the persisted decay_state.
// Returns the new state and the action the reconciler must take.
func EvaluateDecay(ageMin float64, rMultiple float64, currentState string, prof config.RiskProfile) (newState, action string) {
	// Hard horizon: always a time exit, whatever the PnL.
	if prof.HorizonMin > 0 && ageMin >= float64(prof.HorizonMin) {
		return "CLOSED", "TIME_EXIT"
	}

	// Flat checkpoint: still unprofitable -> close (never ride a dead trade
	// to the horizon bell).
	if prof.DecayFlatAtMin > 0 && ageMin >= float64(prof.DecayFlatAtMin) && rMultiple <= 0 {
		return "CLOSED", "CLOSE"
	}

	// Breakeven checkpoint: protect a laggard once.
	if prof.DecayBreakevenAtMin > 0 && ageMin >= float64(prof.DecayBreakevenAtMin) {
		if currentState == "NONE" && rMultiple < 0.5 {
			return "BREAKEVEN", "BREAKEVEN"
		}
		if currentState == "BREAKEVEN" {
			return "BREAKEVEN", "" // already protected: no repeated move
		}
	}

	return currentState, ""
}
