package trader

import (
	"testing"
)

func fptr(v float64) *float64 { return &v }

// TestEvaluateEntryGateRules walks every rejection rule and the pass-through
// path (spec 012 US1, contracts/api.md §3 rule enum).
func TestEvaluateEntryGateRules(t *testing.T) {
	base := EntryGateInput{
		Symbol:       "BTC/USDT",
		Direction:    "LONG",
		Price:        100.0,
		VWAP:         99.0, // price above VWAP: LONG trend OK
		UpperBand:    101.0,
		LowerBand:    99.0,
		MidBand:      100.0,
		SuperTrend:   "BULL",
		VolumeRatio:  3.0, // >= 2.5
		SlPct:        1.2,
		Leverage:     8,
		LiqBufferMin: 4.0,
	}

	t.Run("all green passes", func(t *testing.T) {
		got := EvaluateEntryGate(base)
		if !got.Allowed {
			t.Fatalf("expected allowed, got rule %s detail %+v", got.Rule, got.Detail)
		}
	})

	t.Run("NO_TREND long below VWAP", func(t *testing.T) {
		in := base
		in.VWAP = 101.0 // price below VWAP
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "NO_TREND" {
			t.Fatalf("want NO_TREND, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("NO_TREND short with bear trend but price above VWAP", func(t *testing.T) {
		in := base
		in.Direction = "SHORT"
		in.SuperTrend = "BEAR" // trend agrees
		in.VWAP = 99.0         // but price above VWAP -> not with-trend for SHORT
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "NO_TREND" {
			t.Fatalf("want NO_TREND, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("NO_VOLUME below 2.5x", func(t *testing.T) {
		in := base
		in.VWAP = 98.0 // clear trend alignment so volume rule is reached
		in.VolumeRatio = 2.0
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "NO_VOLUME" {
			t.Fatalf("want NO_VOLUME, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("CHASE_BLOCKED long beyond +2 sigma", func(t *testing.T) {
		in := base
		in.VWAP = 98.0
		in.VolumeRatio = 4.0
		in.Price = 102.0 // sigma=(101-99)/4=0.5 -> z=4 > 2
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "CHASE_BLOCKED" {
			t.Fatalf("want CHASE_BLOCKED, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("OI_TRAP long with OI collapse", func(t *testing.T) {
		in := base
		in.VWAP = 98.0
		in.VolumeRatio = 4.0
		in.OIDeltaPct = fptr(-2.0)
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "OI_TRAP" {
			t.Fatalf("want OI_TRAP, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("OI unknown is permissive", func(t *testing.T) {
		in := base
		in.VWAP = 98.0
		in.VolumeRatio = 4.0
		in.OIDeltaPct = nil
		got := EvaluateEntryGate(in)
		if !got.Allowed {
			t.Fatalf("nil OI must not block, got rule %s", got.Rule)
		}
	})

	t.Run("LIQ_BUFFER buffer below floor", func(t *testing.T) {
		in := base
		in.VWAP = 98.0
		in.VolumeRatio = 4.0
		in.Leverage = 20 // liq dist ~5% - 0.5% = 4.5%
		in.SlPct = 1.5   // buffer = 4.5/1.5 = 3.0 < 4.0
		in.LiqBufferMin = 4.0
		got := EvaluateEntryGate(in)
		if got.Allowed || got.Rule != "LIQ_BUFFER" {
			t.Fatalf("want LIQ_BUFFER, got allowed=%v rule=%q", got.Allowed, got.Rule)
		}
	})

	t.Run("LIQ_BUFFER satisfied at 8x", func(t *testing.T) {
		in := base
		in.VWAP = 98.0
		in.VolumeRatio = 4.0
		in.Leverage = 8 // liq dist 12.5-0.5=12%; buffer=12/1.2=10 >= 4
		got := EvaluateEntryGate(in)
		if !got.Allowed {
			t.Fatalf("expected allowed, got rule %s", got.Rule)
		}
	})

	t.Run("unknown fields stay permissive", func(t *testing.T) {
		in := EntryGateInput{
			Symbol:    "X/USDT",
			Direction: "LONG",
			Price:     1.0,
			// no VWAP, no bands, no volume, no leverage
		}
		got := EvaluateEntryGate(in)
		if !got.Allowed {
			t.Fatalf("zero-value inputs must be permissive, got rule %s", got.Rule)
		}
	})
}
