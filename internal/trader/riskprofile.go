package trader

import (
	"fmt"
	"log"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// EffectiveProfile resolves and validates the risk profile for an asset class.
// Invalid profiles never reach the sizing/exit math: they fail loudly so a bad
// config cannot silently distort stops or horizons (constitution VIII).
func EffectiveProfile(name string) (config.RiskProfile, error) {
	p := config.GetRiskProfile(name)
	if err := p.Validate(); err != nil {
		return config.RiskProfile{}, fmt.Errorf("effective profile: %w", err)
	}
	return p, nil
}

// EffectiveProfileWithTimeframe resolves the base risk profile and overrides
// HorizonMin, DecayBreakevenAtMin, and DecayFlatAtMin from the TimeframeProfile
// if timeframe is non-empty and known (spec-016 FR-205/FR-206).
func EffectiveProfileWithTimeframe(name, timeframe string) (config.RiskProfile, error) {
	p, err := EffectiveProfile(name)
	if err != nil {
		return config.RiskProfile{}, err
	}
	if timeframe != "" {
		if tfProf, ok := config.GetTimeframeProfile(timeframe); ok {
			p.HorizonMin = tfProf.HorizonMin
			p.DecayBreakevenAtMin = tfProf.BEOffsetMin
			p.DecayFlatAtMin = tfProf.FlatOffsetMin
		}
	}
	return p, nil
}

// ValidateRiskProfiles checks the full seeded set at startup so operators learn
// about a bad configuration before the first signal is generated.
func ValidateRiskProfiles() error {
	for name, p := range config.LoadRiskProfiles() {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("risk profile %q: %w", name, err)
		}
		log.Printf("[riskprofile] %s ok: horizon=%dm decay=%dm/%dm sl=%.1fATR tp1=%.1fATR risk=%.2f%%",
			name, p.HorizonMin, p.DecayBreakevenAtMin, p.DecayFlatAtMin,
			p.SLAtrMult, p.TP1AtrMult, p.RiskPerTradePct*100)
	}
	return nil
}
