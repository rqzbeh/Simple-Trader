package trader

import "math"

// Entry gate rules (spec 012 US1, research R1). The gate runs after the AI
// returns a direction and before the signal is persisted: a rejected
// candidate is written to entry_filter_log instead of becoming a trade.
//
// Rule enum (contracts/api.md §3):
//
//	NO_TREND      price on the wrong side of VWAP / SuperTrend disagreement
//	NO_VOLUME     catalyst volume < 2.5x 20-period average
//	CHASE_BLOCKED price beyond +2 sigma of the recent band in trade direction
//	OI_TRAP       price move contradicted by open-interest delta
//	LIQ_BUFFER    distance-to-liquidation / stop < configured safety floor
const (
	// VolumeRatioMin: catalyst candle volume must exceed 2.5x the 20-period mean.
	VolumeRatioMin = 2.5
	// ChaseSigmaLimit: block entries when price is beyond 2 sigma of the
	// Bollinger mid (sigma ~= (upper-lower)/4 for a 2-sigma band).
	ChaseSigmaLimit = 2.0
	// OIDeltaTrapPct: open-interest delta threshold that flags a squeeze.
	OIDeltaTrapPct = 1.5
	// DefaultMaintenanceMarginPct used for the liquidation-buffer check when
	// the exchange-specific rate is unknown.
	DefaultMaintenanceMarginPct = 0.005
)

// EntryGateInput carries every measurement the gate needs. Zero values mean
// "unknown" and cause the corresponding rule to be skipped (never blocked),
// so a missing data feed degrades to permissive instead of deadlocking
// signal generation.
type EntryGateInput struct {
	Symbol    string
	Direction string // "LONG" or "SHORT"

	Price      float64 // candidate entry price
	VWAP       float64 // session vwap; 0 = unknown
	UpperBand  float64 // bollinger upper; 0 = unknown
	LowerBand  float64 // bollinger lower; 0 = unknown
	MidBand    float64 // bollinger mid; 0 = unknown
	SuperTrend string  // "BULL" | "BEAR" | ""

	VolumeRatio float64  // last/avg20; 0 = unknown
	OIDeltaPct  *float64 // 15m OI change %; nil = unknown

	SlPct                float64 // stop distance in % of price (prospective)
	Leverage             int     // prospective leverage
	LiqBufferMin         float64 // required (liq distance / sl distance)
	MaintenanceMarginPct float64 // maintenance margin rate; 0 => DefaultMaintenanceMarginPct
}

// EntryGateDecision is the outcome: Allowed=false means the candidate must be
// logged with Rule and not traded.
type EntryGateDecision struct {
	Allowed bool
	Rule    string
	Detail  map[string]interface{}
}

// EvaluateEntryGate applies the rule chain in fixed order and returns the
// first violated rule (or Allowed=true).
func EvaluateEntryGate(in EntryGateInput) EntryGateDecision {
	long := in.Direction == "LONG"

	// Rule 1: trend alignment — trade only with the trend.
	if in.VWAP > 0 && in.SuperTrend != "" {
		priceAboveVWAP := in.Price > in.VWAP
		stBull := in.SuperTrend == "BULL"
		trendOK := (long && priceAboveVWAP && stBull) || (!long && !priceAboveVWAP && !stBull)
		if !trendOK {
			return EntryGateDecision{
				Allowed: false,
				Rule:    "NO_TREND",
				Detail: map[string]interface{}{
					"price":      in.Price,
					"vwap":       in.VWAP,
					"supertrend": in.SuperTrend,
					"direction":  in.Direction,
				},
			}
		}
	}

	// Rule 2: volume expansion around the catalyst.
	if in.VolumeRatio > 0 && in.VolumeRatio < VolumeRatioMin {
		return EntryGateDecision{
			Allowed: false,
			Rule:    "NO_VOLUME",
			Detail: map[string]interface{}{
				"volume_ratio": round4(in.VolumeRatio),
				"required":     VolumeRatioMin,
			},
		}
	}

	// Rule 3: anti-chase — price already extended beyond 2 sigma.
	if in.UpperBand > 0 && in.LowerBand > 0 && in.MidBand > 0 {
		sigma := (in.UpperBand - in.LowerBand) / 4.0
		if sigma > 0 {
			z := (in.Price - in.MidBand) / sigma
			blocked := (long && z > ChaseSigmaLimit) || (!long && -z > ChaseSigmaLimit)
			if blocked {
				return EntryGateDecision{
					Allowed: false,
					Rule:    "CHASE_BLOCKED",
					Detail: map[string]interface{}{
						"z_score":   round4(z),
						"limit":     ChaseSigmaLimit,
						"direction": in.Direction,
					},
				}
			}
		}
	}

	// Rule 4: OI trap — price move without (or against) participation.
	if in.OIDeltaPct != nil {
		oi := *in.OIDeltaPct
		trapped := (long && oi < -OIDeltaTrapPct) || (!long && oi > OIDeltaTrapPct)
		if trapped {
			return EntryGateDecision{
				Allowed: false,
				Rule:    "OI_TRAP",
				Detail: map[string]interface{}{
					"oi_delta_pct": round4(oi),
					"threshold":    OIDeltaTrapPct,
					"direction":    in.Direction,
				},
			}
		}
	}

	// Rule 5: liquidation buffer — stop must sit far from liquidation.
	if in.Leverage >= 1 && in.SlPct > 0 && in.LiqBufferMin > 0 {
		mmr := in.MaintenanceMarginPct
		if mmr <= 0 {
			mmr = DefaultMaintenanceMarginPct
		}
		liqDistancePct := 100.0/float64(in.Leverage) - mmr*100.0
		if liqDistancePct > 0 {
			buffer := liqDistancePct / in.SlPct
			if buffer < in.LiqBufferMin {
				return EntryGateDecision{
					Allowed: false,
					Rule:    "LIQ_BUFFER",
					Detail: map[string]interface{}{
						"buffer":       round4(buffer),
						"required":     in.LiqBufferMin,
						"leverage":     in.Leverage,
						"sl_pct":       round4(in.SlPct),
						"liq_dist_pct": round4(liqDistancePct),
					},
				}
			}
		}
	}

	return EntryGateDecision{Allowed: true}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
