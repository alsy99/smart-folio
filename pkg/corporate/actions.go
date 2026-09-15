package corporate

import (
	"time"

	"aperture/pkg/asof"
	"aperture/pkg/marketclock"
)

// Action is an NSE cash corporate action. Only announcements known at the
// bar may affect research or fills.
type Action struct {
	Symbol      string
	Kind        string // dividend, split, bonus, demerger
	AnnouncedAt time.Time
	ExDate      time.Time
	Note        string
}

func ist(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, marketclock.Location())
}

var catalog = []Action{
	{
		Symbol: "ITC", Kind: "demerger",
		AnnouncedAt: ist(2023, 8, 1, 9, 0),
		ExDate:      ist(2024, 1, 1, 9, 15),
		Note:        "Hotels demerger completed; card is historical, not a live filing.",
	},
	{
		Symbol: "HDFCBANK", Kind: "merger",
		AnnouncedAt: ist(2022, 4, 4, 9, 0),
		ExDate:      ist(2023, 7, 1, 9, 15),
		Note:        "HDFC Ltd merger digestion; liability franchise rebuild.",
	},
}

// At returns actions for symbol whose announcement is known as-of the bar.
func At(symbol string, bar time.Time) []Action {
	var out []Action
	for _, a := range catalog {
		if a.Symbol != symbol {
			continue
		}
		if asof.KnownAt(a.AnnouncedAt, bar) {
			out = append(out, a)
		}
	}
	return out
}

// Effective is true when the action has gone ex as-of the bar.
func (a Action) Effective(bar time.Time) bool {
	return asof.KnownAt(a.AnnouncedAt, bar) && asof.KnownAt(a.ExDate, bar)
}

func Notes(symbol string, bar time.Time) string {
	acts := At(symbol, bar)
	if len(acts) == 0 {
		return ""
	}
	s := "Corporate actions as-of bar:"
	for _, a := range acts {
		s += " " + a.Kind + " announced " + a.AnnouncedAt.Format("2006-01-02")
		if a.Effective(bar) {
			s += " (ex " + a.ExDate.Format("2006-01-02") + ")"
		} else {
			s += " (not yet ex)"
		}
		s += "."
	}
	return s
}
