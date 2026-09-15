package policy

import (
	"sort"
	"time"

	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
)

// Calendar is the tape's session dates (NIFTY50 candle dates, YYYY-MM-DD,
// sorted). The rebalance session is the last session of the calendar
// month or quarter on this calendar — not the last civil weekday, which
// may be an exchange holiday. Past the last known session (the live edge)
// it falls back to the civil weekday rule and says so via Known.
type Calendar []string

func CalendarFrom(dates []string) Calendar {
	seen := map[string]bool{}
	out := make(Calendar, 0, len(dates))
	for _, d := range dates {
		if len(d) != 10 || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func period(date string, r ips.Rebalance) string {
	if len(date) < 7 {
		return date
	}
	if r == ips.RebalanceQuarterly {
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			return date[:7]
		}
		q := (int(t.Month())-1)/3 + 1
		return t.Format("2006") + "-Q" + string(rune('0'+q))
	}
	return date[:7]
}

// Known reports whether the calendar can decide date without falling
// back to civil weekdays: it needs a session strictly after date.
func (c Calendar) Known(date string) bool {
	return len(c) > 0 && c[len(c)-1] > date
}

// IsRebalanceSession: date is a session and the next session on the
// calendar is in a different period. Past the calendar's edge, date is a
// weekday and the next civil weekday is in a different period.
func (c Calendar) IsRebalanceSession(date string, r ips.Rebalance) bool {
	i := sort.SearchStrings(c, date)
	if i < len(c) && c[i] == date {
		if i+1 < len(c) {
			return period(c[i+1], r) != period(date, r)
		}
		return civilLast(date, r)
	}
	if c.Known(date) {
		return false // a known non-session (holiday) never rebalances
	}
	return civilLast(date, r)
}

// civilLast: date is a weekday whose next weekday is in a new period.
func civilLast(date string, r ips.Rebalance) bool {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	next := t.AddDate(0, 0, 1)
	for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		next = next.AddDate(0, 0, 1)
	}
	return period(next.Format("2006-01-02"), r) != period(date, r)
}

// NextRebalance is the first rebalance session on or after date.
func (c Calendar) NextRebalance(date string, r ips.Rebalance) string {
	i := sort.SearchStrings(c, date)
	for ; i < len(c); i++ {
		if c.IsRebalanceSession(c[i], r) {
			return c[i]
		}
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	for k := 0; k < 120; k++ {
		d := t.AddDate(0, 0, k).Format("2006-01-02")
		if c.IsRebalanceSession(d, r) {
			return d
		}
	}
	return ""
}

// SessionDate is the IST date of a mark.
func SessionDate(t time.Time) string { return marketclock.SessionDate(t) }
