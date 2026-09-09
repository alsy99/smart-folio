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
