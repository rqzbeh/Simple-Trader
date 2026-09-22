package trader

import (
	"math"
)

// FrictionModel defines exchange fees, spread, and liquidity impact slippage parameters.
type FrictionModel struct {
	MakerFeeRate   float64 // e.g. 0.0002 (0.02%)
	TakerFeeRate   float64 // e.g. 0.0005 (0.05%)
	ImpactFactor   float64 // quadratic market impact coefficient (default: 0.05)
	MaxSlippagePct float64 // dynamic slippage cap percentage (e.g. 0.05 for 5%)
}

// NewFrictionModel initializes friction settings with dynamic parameters.
func NewFrictionModel(makerFee, takerFee, impactFactor, maxSlippagePct float64) FrictionModel {
	if makerFee <= 0 {
		makerFee = 0.0002
	}
	if takerFee <= 0 {
		takerFee = 0.0005
	}
	if impactFactor <= 0 {
		impactFactor = 0.05
	}
	if maxSlippagePct <= 0 {
		maxSlippagePct = 0.05
	}
	return FrictionModel{
		MakerFeeRate:   makerFee,
		TakerFeeRate:   takerFee,
		ImpactFactor:   impactFactor,
		MaxSlippagePct: maxSlippagePct,
	}
}

// DefaultFrictionModel returns standard institutional exchange friction settings.
func DefaultFrictionModel() FrictionModel {
	return NewFrictionModel(0.0002, 0.0005, 0.05, 0.05)
}

// ExecutionQuote provides detailed execution pricing, spread, slippage, and fee breakdown.
type ExecutionQuote struct {
	BasePrice      float64 `json:"base_price"`
	EffectivePrice float64 `json:"effective_price"`
	HalfSpread     float64 `json:"half_spread"`
	Slippage       float64 `json:"slippage"`
	FeeRate        float64 `json:"fee_rate"`
	TotalFee       float64 `json:"total_fee"`
	TotalGrossCost float64 `json:"total_gross_cost"`
	TotalNetCost   float64 `json:"total_net_cost"`
}

// CalculateExecution calculates realistic execution price with spread, quadratic slippage, and fees.
// symbolSpreadPct: typical spread as a percentage of price (e.g. 0.0001 for 1 bp)
// availableDepthQty: available top-of-book depth in asset units (e.g. 100 BTC or 500 Gold oz)
func (f *FrictionModel) CalculateExecution(
	side string,
	quantity float64,
	midPrice float64,
	symbolSpreadPct float64,
	availableDepthQty float64,
	isTaker bool,
) ExecutionQuote {
	if symbolSpreadPct <= 0 {
		symbolSpreadPct = 0.0002 // default 2 bps spread
	}
	if availableDepthQty <= 0 {
		availableDepthQty = 100.0 // default reasonable depth
	}

	halfSpread := midPrice * (symbolSpreadPct / 2.0)

	// Quadratic market impact (slippage)
	// Slippage = midPrice * ImpactFactor * (quantity / availableDepth)^2
	liquidityRatio := quantity / availableDepthQty
	slippage := midPrice * f.ImpactFactor * math.Pow(liquidityRatio, 2)
	// Cap slippage to at most MaxSlippagePct of price to avoid extreme synthetic anomalies
	capPct := f.MaxSlippagePct
	if capPct <= 0 {
		capPct = 0.05
	}
	maxSlippage := midPrice * capPct
	if slippage > maxSlippage {
		slippage = maxSlippage
	}

	var effectivePrice float64
	if side == "BUY" {
		effectivePrice = midPrice + halfSpread + slippage
	} else {
		effectivePrice = midPrice - halfSpread - slippage
		if effectivePrice <= 0 {
			effectivePrice = midPrice * 0.01 // minimum guard
		}
	}

	feeRate := f.TakerFeeRate
	if !isTaker {
		feeRate = f.MakerFeeRate
	}

	grossValue := quantity * effectivePrice
	feeAmount := grossValue * feeRate

	var netValue float64
	if side == "BUY" {
		netValue = grossValue + feeAmount
	} else {
		netValue = grossValue - feeAmount
	}

	return ExecutionQuote{
		BasePrice:      midPrice,
		EffectivePrice: effectivePrice,
		HalfSpread:     halfSpread,
		Slippage:       slippage,
		FeeRate:        feeRate,
		TotalFee:       feeAmount,
		TotalGrossCost: grossValue,
		TotalNetCost:   netValue,
	}
}
