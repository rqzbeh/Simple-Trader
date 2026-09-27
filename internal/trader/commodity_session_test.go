package trader

import (
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// --- Spec 012 US4: commodity session semantics (FR-014) ---

func TestWeekendGapWindow(t *testing.T) {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	cases := []struct {
		name string
		when time.Time
		want bool
	}{
		// Friday 16:45 ET (FX close) onward: gap open.
		{"fri 16:45 ET opens the gap", time.Date(2026, 9, 25, 16, 45, 0, 0, et), true},
		{"fri 17:00 ET inside gap", time.Date(2026, 9, 25, 17, 0, 0, 0, et), true},
		{"sat noon inside gap", time.Date(2026, 9, 26, 12, 0, 0, 0, et), true},
		{"sun 17:59 ET still inside", time.Date(2026, 9, 27, 17, 59, 0, 0, et), true},
		// Just outside the edges.
		{"fri 16:44 ET before open", time.Date(2026, 9, 25, 16, 44, 0, 0, et), false},
		{"sun 18:00 ET gap closed", time.Date(2026, 9, 27, 18, 0, 0, 0, et), false},
		{"mon midday tradable", time.Date(2026, 9, 28, 12, 0, 0, 0, et), false},
		{"wednesday tradable", time.Date(2026, 9, 23, 9, 30, 0, 0, et), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WeekendGapOpen(tc.when); got != tc.want {
				t.Errorf("WeekendGapOpen(%s) = %v, want %v", tc.when, got, tc.want)
			}
		})
	}
}

func TestBlackoutWindows(t *testing.T) {
	prof := config.DefaultCommodityProfile() // NFP/CPI/FOMC ±30m, EIA ±15m
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// NFP at 08:30 ET on Friday 2026-10-02.
	nfp := ScheduledEvent{Name: "NFP", Time: time.Date(2026, 10, 2, 8, 30, 0, 0, et)}

	blocked, name := InBlackoutWindow(prof, []ScheduledEvent{nfp}, nfp.Time)
	if !blocked || name != "NFP" {
		t.Errorf("at NFP: blocked=%v name=%q, want true/NFP", blocked, name)
	}
	blocked, name = InBlackoutWindow(prof, []ScheduledEvent{nfp}, nfp.Time.Add(-25*time.Minute))
	if !blocked {
		t.Errorf("25m before NFP must be inside ±30m window")
	}
	blocked, _ = InBlackoutWindow(prof, []ScheduledEvent{nfp}, nfp.Time.Add(45*time.Minute))
	if blocked {
		t.Errorf("45m after NFP must be OUTSIDE ±30m window")
	}

	// EIA at 10:30 ET Wednesday: ±15m only.
	eia := ScheduledEvent{Name: "EIA", Time: time.Date(2026, 9, 23, 10, 30, 0, 0, et)}
	blocked, _ = InBlackoutWindow(prof, []ScheduledEvent{eia}, eia.Time.Add(-10*time.Minute))
	if !blocked {
		t.Errorf("10m before EIA must be inside ±15m window")
	}
	blocked, _ = InBlackoutWindow(prof, []ScheduledEvent{eia}, eia.Time.Add(-20*time.Minute))
	if blocked {
		t.Errorf("20m before EIA must be OUTSIDE ±15m window")
	}

	// Unknown event name never matches a blackout window.
	blocked, _ = InBlackoutWindow(prof, []ScheduledEvent{{Name: "Random", Time: eia.Time}}, eia.Time)
	if blocked {
		t.Errorf("unmatched event must not block")
	}
}

func TestEventFreshnessInsideBlackout(t *testing.T) {
	// Events already past their After-window never block.
	prof := config.DefaultCommodityProfile()
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	nfp := ScheduledEvent{Name: "NFP", Time: time.Date(2026, 10, 2, 8, 30, 0, 0, et)}
	blocked, _ := InBlackoutWindow(prof, []ScheduledEvent{nfp}, nfp.Time.Add(31*time.Minute))
	if blocked {
		t.Errorf("31m after NFP must be outside the ±30m window")
	}
}

func TestFlattenDeadline(t *testing.T) {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	prof := config.DefaultCommodityProfile() // weekend_flat=true, horizon 240m

	created := time.Date(2026, 9, 25, 10, 0, 0, 0, et) // Friday 10:00 ET
	deadline := FlattenDeadline(created, prof)

	// WeekendFlat profile: the next weekend gap opens Fri 16:45 ET; the
	// flatten deadline must precede it (15m buffer) — NOT the 240m horizon
	// (which would land at 14:00, earlier, so horizon wins).
	if want := created.Add(240 * time.Minute); !deadline.Equal(want) {
		t.Errorf("deadline = %s, want horizon %s (10:00 + 4h = 14:00 < 16:45)", deadline, want)
	}

	// Created Friday 15:00 ET: horizon (19:00) lands inside the gap
	// (opens 16:45) — gap start minus the 15m buffer (16:30) wins.
	created2 := time.Date(2026, 9, 25, 15, 0, 0, 0, et)
	deadline2 := FlattenDeadline(created2, prof)
	if want := time.Date(2026, 9, 25, 16, 30, 0, 0, et); !deadline2.Equal(want) {
		t.Errorf("deadline2 = %s, want %s (gap edge 16:45 minus 15m buffer)", deadline2, want)
	}
	if WeekendGapOpen(deadline2.Add(14 * time.Minute)) {
		t.Errorf("16:44 must still be tradable")
	}
	if !WeekendGapOpen(deadline2.Add(16 * time.Minute)) {
		t.Errorf("16:46 must be inside the gap")
	}

	// Non-weekendFlat profile: only the horizon applies.
	cryptoProf := config.DefaultCryptoProfile()
	if got, want := FlattenDeadline(created, cryptoProf), created.Add(60*time.Minute); !got.Equal(want) {
		t.Errorf("crypto deadline = %s, want %s", got, want)
	}

	// MustFlatten triggers exactly at the deadline, not before.
	if MustFlatten(deadline.Add(-time.Minute), created, prof) {
		t.Errorf("1m before deadline must not force flat")
	}
	if !MustFlatten(deadline, created, prof) {
		t.Errorf("at the deadline must force flat")
	}
}
