package marketclock

import (
	"os"
	"strings"
	"time"
)

const istOffset = 5*time.Hour + 30*time.Minute

func Location() *time.Location {
	if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
		return loc
	}
	return time.FixedZone("IST", int(istOffset/time.Second))
}

func OverrideOpen() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MARKET_CLOCK_OVERRIDE")))
	return v == "open" || v == "1" || v == "true"
}

func IsOpen(now time.Time) bool {
	if OverrideOpen() {
		return true
	}
	t := now.In(Location())
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	minutes := t.Hour()*60 + t.Minute()
	open := 9*60 + 15
	close := 15*60 + 30
	return minutes >= open && minutes <= close
}

// SessionDate is the IST calendar day a timestamp falls on (YYYY-MM-DD).
func SessionDate(t time.Time) string {
	return t.In(Location()).Format("2006-01-02")
}

// SessionsHeld counts full NSE cash sessions completed between a fill and
// now: weekdays strictly after the fill's IST date whose 15:30 close is at
// or before now. A Monday 15:25 fill has held 0 sessions until Tuesday
// 15:30, 1 session until Wednesday 15:30, and so on. Exchange holidays are
// not modelled, so the count is an upper bound — the book holds at least
// this long.
func SessionsHeld(openedAt, now time.Time) int {
	loc := Location()
	o := openedAt.In(loc)
	n := now.In(loc)
	if !n.After(o) {
		return 0
	}
	day := time.Date(o.Year(), o.Month(), o.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	sessions := 0
	for ; !day.After(n); day = day.AddDate(0, 0, 1) {
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		closeAt := time.Date(day.Year(), day.Month(), day.Day(), 15, 30, 0, 0, loc)
		if closeAt.After(n) {
			break
		}
		sessions++
	}
	return sessions
}

func SessionStatus(now time.Time) string {
	if OverrideOpen() {
		return "open (clock override)"
	}
	if IsOpen(now) {
		return "open"
	}
	t := now.In(Location())
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return "weekend"
	}
	return "closed"
}
