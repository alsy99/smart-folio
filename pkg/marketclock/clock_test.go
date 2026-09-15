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

func TestOverrideForcesOpen(t *testing.T) {
	t.Setenv("MARKET_CLOCK_OVERRIDE", "open")
	sat := time.Date(2026, 9, 5, 12, 0, 0, 0, Location())
	if !IsOpen(sat) {
		t.Fatal("override should open the tape")
	}
}
