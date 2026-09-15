package trading

import (
	"context"

	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
)

// BookPrint is one published daily snapshot of the paper book.
type BookPrint struct {
	Equity, Peak, Cash, Turnover float64
	Fills                        int
	CoreFills, SatelliteFills    int
	Core, Satellite              float64 // market value by sleeve
	Halted                       bool
	Last                         map[string]float64
	BenchStart                   map[string]float64
}

func (s *Service) ResetTickStats() {
	s.mu.Lock()
	s.lastTurnover = 0
	s.lastFills = 0
	s.lastCoreFills, s.lastSatFills = 0, 0
	s.mu.Unlock()
}

func copyFloatMap(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Print marks the book at Now() and returns the daily publish fields.
func (s *Service) Print(ctx context.Context) (BookPrint, error) {
	quotes, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{})
	if err != nil {
		return BookPrint{}, err
	}
	last := map[string]float64{}
	for _, q := range quotes.Quotes {
		last[q.Symbol] = q.Last
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateMarksLocked(last)
	snap := s.snapshotLocked()
	ddHalt := costs.BookHalted(snap.Equity, snap.Peak)
	core := 0.0
	for sym, q := range s.coreQty {
		core += q * last[sym]
	}
	return BookPrint{
		Equity:         snap.Equity,
		Peak:           snap.Peak,
		Cash:           snap.Cash,
		Turnover:       s.lastTurnover,
		Fills:          s.lastFills,
		CoreFills:      s.lastCoreFills,
		SatelliteFills: s.lastSatFills,
		Core:           core,
		Satellite:      snap.Gross - core,
		Halted:         broker.Halted(snap) || ddHalt,
		Last:           last,
		BenchStart:     copyFloatMap(s.benchStart),
	}, nil
}
