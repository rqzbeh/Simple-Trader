package indicators

import (
	"math"
)

// MarketRegime represents the current volatility state of the market.
type MarketRegime string

const (
	RegimeLowVolMeanReversion MarketRegime = "LOW_VOL_CONSOLIDATION"
	RegimeNormalTrending      MarketRegime = "NORMAL_TRENDING"
	RegimeHighVolChop         MarketRegime = "HIGH_VOL_CHOP"
)

// RegimeClassifier classifies market conditions based on normalized ATR and historical volatility ratio.
type RegimeClassifier struct {
	atrPeriod int
	smaPeriod int
}

// NewRegimeClassifier creates a new regime classifier instance.
func NewRegimeClassifier(atrPeriod, smaPeriod int) *RegimeClassifier {
	if atrPeriod <= 0 {
		atrPeriod = 14
	}
	if smaPeriod <= 0 {
		smaPeriod = 50
	}
	return &RegimeClassifier{
		atrPeriod: atrPeriod,
		smaPeriod: smaPeriod,
	}
}

// ClassifyRegime evaluates rolling ATR values and computes VolRatio.
// VolRatio = ATR_14 / SMA(ATR_14, 50)
// - VolRatio < 0.70: LOW_VOL_CONSOLIDATION (favors mean reversion, Bollinger bands)
// - 0.70 <= VolRatio <= 1.30: NORMAL_TRENDING (favors momentum, trend following)
// - VolRatio > 1.30: HIGH_VOL_CHOP (risk reduction / danger zone)
func (rc *RegimeClassifier) ClassifyRegime(currentATR float64, historicalATRs []float64) (MarketRegime, float64) {
	if currentATR <= 0 {
		return RegimeNormalTrending, 1.0
	}

	if len(historicalATRs) == 0 {
		return RegimeNormalTrending, 1.0
	}

	count := len(historicalATRs)
	if count > rc.smaPeriod {
		historicalATRs = historicalATRs[count-rc.smaPeriod:]
		count = rc.smaPeriod
	}

	var sum float64
	for _, val := range historicalATRs {
		sum += val
	}
	meanATR := sum / float64(count)
	if meanATR <= 0 {
		return RegimeNormalTrending, 1.0
	}

	volRatio := currentATR / meanATR
	volRatio = math.Round(volRatio*10000) / 10000

	if volRatio < 0.70 {
		return RegimeLowVolMeanReversion, volRatio
	}
	if volRatio > 1.30 {
		return RegimeHighVolChop, volRatio
	}
	return RegimeNormalTrending, volRatio
}

// CalculateSizingMultiplier computes volatility-adjusted risk scaling factor.
func CalculateSizingMultiplier(regime MarketRegime, volRatio float64) float64 {
	switch regime {
	case RegimeHighVolChop:
		if volRatio <= 0 {
			return 0.5
		}
		mult := 1.0 / volRatio
		if mult < 0.25 {
			mult = 0.25
		}
		return mult
	case RegimeLowVolMeanReversion:
		return 1.10
	default:
		return 1.0
	}
}
