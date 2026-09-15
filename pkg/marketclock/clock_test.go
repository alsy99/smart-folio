package marketclock

import (
	"testing"
	"time"
)

func TestWeekendClosedWithoutOverride(t *testing.T) {
	t.Setenv("MARKET_CLOCK_OVERRIDE", "")
	sat := time.Date(2026, 9, 5, 12, 0, 0, 0, Location())
	if IsOpen(sat) {
		t.Fatal("Saturday should be closed")
	}
	if SessionStatus(sat) != "weekend" {
		t.Fatalf("status %s", SessionStatus(sat))
	}
}

func TestWeekdaySessionHours(t *testing.T) {
	t.Setenv("MARKET_CLOCK_OVERRIDE", "")
	open := time.Date(2026, 9, 9, 10, 0, 0, 0, Location())
	closed := time.Date(2026, 9, 9, 22, 0, 0, 0, Location())
	if !IsOpen(open) {
		t.Fatal("Wednesday 10:00 IST should be open")
	}
	if IsOpen(closed) {
		t.Fatal("Wednesday 22:00 IST should be closed")
	}
	if SessionStatus(closed) != "closed" {
		t.Fatalf("status %s", SessionStatus(closed))
	}
}

func TestSessionsHeldCountsFullCashSessions(t *testing.T) {
	loc := Location()
	mon1525 := time.Date(2026, 9, 14, 15, 25, 0, 0, loc)
	cases := []struct {
		at   time.Time
		want int
	}{
		{mon1525, 0},
		{mon1525.Add(5 * time.Minute), 0},              // Monday close is not a new session
		{time.Date(2026, 9, 15, 15, 25, 0, 0, loc), 0}, // Tuesday before the close
		{time.Date(2026, 9, 15, 15, 30, 0, 0, loc), 1}, // Tuesday close
		{time.Date(2026, 9, 17, 15, 25, 0, 0, loc), 2}, // Thursday before close: Tue, Wed
		{time.Date(2026, 9, 17, 15, 30, 0, 0, loc), 3},
		{time.Date(2026, 9, 21, 9, 0, 0, 0, loc), 4}, // next Monday morning: Tue–Fri, weekend skipped
	}
	for _, c := range cases {
		if got := SessionsHeld(mon1525, c.at); got != c.want {
			t.Fatalf("SessionsHeld(Mon 15:25, %s) = %d, want %d", c.at.Format("Mon 15:04"), got, c.want)
		}
	}
	if SessionDate(mon1525) != "2026-09-14" {
		t.Fatalf("session date %s", SessionDate(mon1525))
	}
}

func TestOverrideForcesOpen(t *testing.T) {
	t.Setenv("MARKET_CLOCK_OVERRIDE", "open")
	sat := time.Date(2026, 9, 5, 12, 0, 0, 0, Location())
	if !IsOpen(sat) {
		t.Fatal("override should open the tape")
	}
}
