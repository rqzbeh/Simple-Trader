package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// RiskProfile bundles the evidence-based exit/entry/sizing parameters for one
// asset class (spec FR-019: all values config-driven with documented defaults).
// Defaults come from docs/RESEARCH-trading-system-optimization.md (R2/R3/R4/R8).
type RiskProfile struct {
	Name string `json:"profile"`

	// Holding horizon
	HorizonMin          int `json:"horizon_min"`            // hard time-exit minutes
	HorizonMax          int `json:"horizon_max"`            // max lifetime (commodity multi-hour horizon)
	DecayBreakevenAtMin int `json:"decay_breakeven_at_min"` // move SL to entry+fees if PnL < +0.5R
	DecayFlatAtMin      int `json:"decay_flat_at_min"`      // close if PnL <= 0

	// Volatility-based stops/targets (multiples of 1h ATR)
	SLAtrMult     float64 `json:"sl_atr_mult"`
	SLSwingOffset float64 `json:"sl_swing_offset"` // ATR buffer behind 15m swing
	SLMinPct      float64 `json:"sl_min_pct"`      // guardrail clamp
	SLMaxPct      float64 `json:"sl_max_pct"`      // guardrail clamp
	TP1AtrMult    float64 `json:"tp1_atr_mult"`
	TP2AtrMult    float64 `json:"tp2_atr_mult"`
	TP1CloseFrac  float64 `json:"tp1_close_fraction"` // partial close at TP1

	// Leverage / sizing
	TargetHourlyVolPct float64 `json:"target_hourly_vol_pct"` // vol-target input for leverage
	LiqBufferMin       float64 `json:"liq_buffer_min"`        // liq_distance / sl >= this
	RiskPerTradePct    float64 `json:"risk_per_trade_pct"`    // fixed-fractional risk budget

	// News fusion
	FreshnessHalfLifeMin float64 `json:"freshness_half_life_min"`

	// Commodities-only semantics
	WeekendFlat     bool             `json:"weekend_flat"`
	BlackoutWindows []BlackoutWindow `json:"blackout_calendar"`
}

// BlackoutWindow is a scheduled no-entry window relative to a named event
// (e.g. NFP ±30m, EIA ±15m). Applied by the commodity session guard (US4).
type BlackoutWindow struct {
	Name      string `json:"name"`
	BeforeMin int    `json:"before_min"`
	AfterMin  int    `json:"after_min"`
}

// DefaultCryptoProfile: 1h intraday futures (research R2/R3/R4).
func DefaultCryptoProfile() RiskProfile {
	return RiskProfile{
		Name:                 "CRYPTO",
		HorizonMin:           60,
		HorizonMax:           60,
		DecayBreakevenAtMin:  30,
		DecayFlatAtMin:       40,
		SLAtrMult:            2.0,
		SLSwingOffset:        0.25,
		SLMinPct:             0.6,
		SLMaxPct:             2.5,
		TP1AtrMult:           1.0,
		TP2AtrMult:           1.8,
		TP1CloseFrac:         0.6,
		TargetHourlyVolPct:   3.5,
		LiqBufferMin:         4.0,
		RiskPerTradePct:      0.015,
		FreshnessHalfLifeMin: 15,
		WeekendFlat:          false,
		BlackoutWindows:      []BlackoutWindow{},
	}
}

// DefaultCommodityProfile: longer horizon, wider stops, report blackouts,
// weekend-gap guard (research R8 / data-model.md §3).
func DefaultCommodityProfile() RiskProfile {
	return RiskProfile{
		Name:                 "COMMODITY",
		HorizonMin:           240,
		HorizonMax:           720,
		DecayBreakevenAtMin:  120,
		DecayFlatAtMin:       180,
		SLAtrMult:            2.5,
		SLSwingOffset:        0.25,
		SLMinPct:             0.6,
		SLMaxPct:             4.0,
		TP1AtrMult:           1.2,
		TP2AtrMult:           2.2,
		TP1CloseFrac:         0.6,
		TargetHourlyVolPct:   1.2,
		LiqBufferMin:         4.0,
		RiskPerTradePct:      0.010,
		FreshnessHalfLifeMin: 60,
		WeekendFlat:          true,
		BlackoutWindows: []BlackoutWindow{
			{Name: "NFP", BeforeMin: 30, AfterMin: 30},
			{Name: "CPI", BeforeMin: 30, AfterMin: 30},
			{Name: "FOMC", BeforeMin: 30, AfterMin: 30},
			{Name: "EIA", BeforeMin: 15, AfterMin: 15},
		},
	}
}

// DefaultRiskProfiles returns the seeded profile set by asset class.
func DefaultRiskProfiles() map[string]RiskProfile {
	c := DefaultCryptoProfile()
	m := DefaultCommodityProfile()
	return map[string]RiskProfile{c.Name: c, m.Name: m}
}

// GetRiskProfile resolves a profile by name (case-insensitive).
// Unknown names fall back to the crypto profile; callers treat that as
// "default intraday behavior".
func GetRiskProfile(name string) RiskProfile {
	key := strings.ToUpper(strings.TrimSpace(name))
	if p, ok := LoadRiskProfiles()[key]; ok {
		return p
	}
	return DefaultCryptoProfile()
}

var (
	profilesOnce  sync.Once
	profilesCache map[string]RiskProfile
)

// LoadRiskProfiles returns the effective profile set: compiled defaults,
// overlaid with RISK_PROFILES_FILE (JSON map keyed by profile name) when set.
// Result is cached after first load (FR-019: retune without code changes).
func LoadRiskProfiles() map[string]RiskProfile {
	profilesOnce.Do(func() {
		profilesCache = DefaultRiskProfiles()
		path := os.Getenv("RISK_PROFILES_FILE")
		if path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("[riskprofile] cannot read %s: %v (using defaults)\n", path, err)
			return
		}
		var overrides map[string]RiskProfile
		if err := json.Unmarshal(data, &overrides); err != nil {
			fmt.Printf("[riskprofile] invalid JSON in %s: %v (using defaults)\n", path, err)
			return
		}
		for name, ov := range overrides {
			base, ok := profilesCache[strings.ToUpper(name)]
			if !ok {
				base = DefaultCryptoProfile()
			}
			mergeProfile(&base, ov)
			if err := base.Validate(); err != nil {
				fmt.Printf("[riskprofile] %v — keeping compiled default\n", err)
				continue
			}
			profilesCache[strings.ToUpper(name)] = base
		}
	})
	return profilesCache
}

// mergeProfile overlays non-zero fields of ov onto base so a config file can
// tune single knobs without restating the whole profile.
func mergeProfile(base *RiskProfile, ov RiskProfile) {
	if ov.Name != "" {
		base.Name = ov.Name
	}
	if ov.HorizonMin > 0 {
		base.HorizonMin = ov.HorizonMin
	}
	if ov.HorizonMax > 0 {
		base.HorizonMax = ov.HorizonMax
	}
	if ov.DecayBreakevenAtMin > 0 {
		base.DecayBreakevenAtMin = ov.DecayBreakevenAtMin
	}
	if ov.DecayFlatAtMin > 0 {
		base.DecayFlatAtMin = ov.DecayFlatAtMin
	}
	if ov.SLAtrMult > 0 {
		base.SLAtrMult = ov.SLAtrMult
	}
	if ov.SLSwingOffset > 0 {
		base.SLSwingOffset = ov.SLSwingOffset
	}
	if ov.SLMinPct > 0 {
		base.SLMinPct = ov.SLMinPct
	}
	if ov.SLMaxPct > 0 {
		base.SLMaxPct = ov.SLMaxPct
	}
	if ov.TP1AtrMult > 0 {
		base.TP1AtrMult = ov.TP1AtrMult
	}
	if ov.TP2AtrMult > 0 {
		base.TP2AtrMult = ov.TP2AtrMult
	}
	if ov.TP1CloseFrac > 0 {
		base.TP1CloseFrac = ov.TP1CloseFrac
	}
	if ov.TargetHourlyVolPct > 0 {
		base.TargetHourlyVolPct = ov.TargetHourlyVolPct
	}
	if ov.LiqBufferMin > 0 {
		base.LiqBufferMin = ov.LiqBufferMin
	}
	if ov.RiskPerTradePct > 0 {
		base.RiskPerTradePct = ov.RiskPerTradePct
	}
	if ov.FreshnessHalfLifeMin > 0 {
		base.FreshnessHalfLifeMin = ov.FreshnessHalfLifeMin
	}
	if ov.WeekendFlat {
		base.WeekendFlat = true
	}
	if len(ov.BlackoutWindows) > 0 {
		base.BlackoutWindows = ov.BlackoutWindows
	}
}

// Validate enforces internal consistency so a bad config can never ship a
// signal whose decay fires after its own horizon or whose stop clamp is
// inverted (constitution VIII: parameters must be well-formed).
func (p RiskProfile) Validate() error {
	var errs []string
	if p.HorizonMin <= 0 {
		errs = append(errs, "horizon_min must be > 0")
	}
	if p.HorizonMax < p.HorizonMin {
		errs = append(errs, "horizon_max must be >= horizon_min")
	}
	if p.DecayBreakevenAtMin <= 0 || p.DecayBreakevenAtMin >= p.DecayFlatAtMin {
		errs = append(errs, "decay checkpoints must satisfy 0 < breakeven < flat")
	}
	if p.HorizonMax > 0 && p.DecayFlatAtMin >= p.HorizonMax {
		errs = append(errs, "decay_flat_at_min must be < horizon_max")
	}
	if p.SLMinPct <= 0 || p.SLMaxPct < p.SLMinPct {
		errs = append(errs, "stop clamps must satisfy 0 < min <= max")
	}
	if p.SLAtrMult <= 0 || p.TP1AtrMult <= 0 {
		errs = append(errs, "ATR multipliers must be > 0")
	}
	if p.TP1CloseFrac <= 0 || p.TP1CloseFrac >= 1 {
		errs = append(errs, "tp1_close_fraction must be in (0,1)")
	}
	if p.RiskPerTradePct <= 0 || p.RiskPerTradePct > 0.05 {
		errs = append(errs, "risk_per_trade_pct must be in (0, 0.05]")
	}
	if p.LiqBufferMin < 1 {
		errs = append(errs, "liq_buffer_min must be >= 1")
	}
	if p.FreshnessHalfLifeMin <= 0 {
		errs = append(errs, "freshness_half_life_min must be > 0")
	}
	if len(errs) > 0 {
		return fmt.Errorf("risk profile %s invalid: %s", p.Name, strings.Join(errs, "; "))
	}
	return nil
}
