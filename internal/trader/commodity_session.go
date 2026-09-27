package trader

import (
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// Commodity session semantics (spec 012 US4, FR-014 / research R8):
//   - event blackout calendar: NFP/CPI/FOMC ±30m, EIA ±15m — no new entries
//   - weekend gap: flat while markets are closed (Fri 16:45 ET → Sun 18:00 ET)
//   - forced flat-before-close: open commodity positions must flatten before
//     the next session edge (whichever comes first: horizon or gap edge)

// ScheduledEvent is one upcoming macro/commodity event relevant to a
// blackout window (name matches a profile BlackoutWindow entry).
type ScheduledEvent struct {
	Name string
	Time time.Time
}

// weekendGapBuffer is the flatten cushion before the gap opens.
const weekendGapBuffer = 15 * time.Minute

// etLocation returns America/New_York with a fixed EST fallback so a missing
// tz database degrades the window by at most one DST hour instead of
// panicking.
func etLocation() *time.Location {
	if loc, err := time.LoadLocation("America/New_York"); err == nil {
		return loc
	}
	return time.FixedZone("EST", -5*60*60)
}

// WeekendGapOpen reports whether now sits inside the weekend market gap:
// Friday 16:45 ET (FX close) through Sunday 18:00 ET (open).
func WeekendGapOpen(now time.Time) bool {
	et := now.In(etLocation())
	weekday := et.Weekday()
	hour, min := et.Hour(), et.Minute()
	t := hour*60 + min

	switch weekday {
	case time.Friday:
		return t >= 16*60+45
	case time.Saturday:
		return true
	case time.Sunday:
		return t < 18*60
	default:
		return false
	}
}

// InBlackoutWindow checks `now` against the profile's event blackout
// calendar (FR-014): blocked when now falls within some event's
// [event - BeforeMin, event + AfterMin] window and that window names a
// configured blackout. Returns the blocking event's name ("" when clear).
func InBlackoutWindow(prof config.RiskProfile, events []ScheduledEvent, now time.Time) (blocked bool, name string) {
	for _, ev := range events {
		for _, w := range prof.BlackoutWindows {
			if !strings.EqualFold(w.Name, ev.Name) {
				continue
			}
			start := ev.Time.Add(-time.Duration(w.BeforeMin) * time.Minute)
			end := ev.Time.Add(time.Duration(w.AfterMin) * time.Minute)
			if !now.Before(start) && !now.After(end) {
				return true, w.Name
			}
		}
	}
	return false, ""
}

// nextWeekendGapStart returns the first moment the weekend gap opens after
// `after` (Fri 16:45 ET). If `after` is already inside the gap it returns
// `after` itself (the position must flatten immediately).
func nextWeekendGapStart(after time.Time) time.Time {
	loc := etLocation()
	if WeekendGapOpen(after) {
		return after
	}
	// Days until the next Friday (0=Sunday..6=Saturday in Go's Weekday).
	et := after.In(loc)
	daysAhead := (int(time.Friday) - int(et.Weekday()) + 7) % 7
	friday := et.AddDate(0, 0, daysAhead)
	gapStart := time.Date(friday.Year(), friday.Month(), friday.Day(), 16, 45, 0, 0, loc)
	if !gapStart.After(after) {
		gapStart = gapStart.AddDate(0, 0, 7)
	}
	return gapStart
}

// FlattenDeadline is the forced flat-before-close moment for an open
// commodity position (FR-014): the earlier of the profile horizon and the
// next weekend-gap edge minus a 15m buffer. Non-weekendFlat profiles only
// use the horizon.
func FlattenDeadline(createdAt time.Time, prof config.RiskProfile) time.Time {
	deadline := createdAt.Add(time.Duration(prof.HorizonMin) * time.Minute)
	if !prof.WeekendFlat {
		return deadline
	}
	gapEdge := nextWeekendGapStart(createdAt).Add(-weekendGapBuffer)
	if gapEdge.Before(deadline) {
		return gapEdge
	}
	return deadline
}

// MustFlatten reports whether an open position must be closed now: at/after
// its flatten deadline, or immediately when the weekend gap is already open
// (a position discovered mid-gap closes at once, never rides the closure).
func MustFlatten(now, createdAt time.Time, prof config.RiskProfile) bool {
	if now.Before(FlattenDeadline(createdAt, prof)) {
		return false
	}
	if prof.WeekendFlat && WeekendGapOpen(now) {
		return true
	}
	// Past the horizon (or the gap-edge deadline): flat.
	return true
}
