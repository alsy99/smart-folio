package backtest

import (
	"time"

	"aperture/pkg/prices"
)

// Tape is where the lab gets its daily closes. The walk-forward engine is
// tape-agnostic; the roster is not. Only a real tape (INDstocks daily
// history or an NSE bhavcopy import) can promote a variant — promotion on
// the mock tape is a plumbing check and never reaches data/roster/.
type Tape interface {
	// Name is recorded on the report and the roster snapshot.
	Name() string
	// Days returns the trading days covered by the tape for [now-years, now].
	Days(years int, now time.Time) ([]time.Time, error)
	// Closes returns one close per day, aligned to days (forward-filled on gaps).
	Closes(symbol string, days []time.Time) ([]float64, error)
}

// TapeMock is the deterministic sine-wave tape used by tests and by a desk
// booted without an INDstocks token.
const TapeMock = "mock"

// IsMockTape reports whether a tape name carries no real market evidence.
func IsMockTape(name string) bool {
	return name == "" || name == TapeMock
}

// MockTape is prices.Last on weekdays. It never promotes.
type MockTape struct{}

func (MockTape) Name() string { return TapeMock }

func (MockTape) Days(years int, now time.Time) ([]time.Time, error) {
	return tradingDays(now, years), nil
}

func (MockTape) Closes(symbol string, days []time.Time) ([]float64, error) {
	out := make([]float64, len(days))
	for i, d := range days {
		out[i] = prices.Last(symbol, d)
	}
	return out, nil
}
