package trader

import (
	"errors"
	"math"
)

// Direction represents futures trade direction.
type Direction string

const (
	DirectionLong  Direction = "LONG"
	DirectionShort Direction = "SHORT"
)

// FuturesContractSpec encapsulates calculations for isolated margin futures contracts.
type FuturesContractSpec struct {
	Direction           Direction `json:"direction"`
	EntryPrice          float64   `json:"entry_price"`
	StopLoss            float64   `json:"stop_loss"`
	TakeProfit1         float64   `json:"take_profit_1"`
	TakeProfit2         *float64  `json:"take_profit_2,omitempty"`
	Leverage            int       `json:"leverage"`
	MaintenanceMarginRate float64 `json:"maintenance_margin_rate"` // e.g. 0.005 for 0.5%
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
	if maxRiskPct <= 0 || maxRiskPct > 0.05 {
		maxRiskPct = 0.02 // default 2.0% safety cap
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
