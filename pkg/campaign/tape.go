package campaign

import (
	"context"
	"fmt"
	"sort"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/marketclock"

	"google.golang.org/grpc"
)

// BarTape is a market-data client over the checked-in daily bars. It is
// as-of: at time t a session's bar is visible only once that session has
// closed (15:30 IST), so a 09:15 quote is the previous close and a 15:30
// tick fills at the day's close — a market-on-close paper fill. No wall
// clock, no broker: the same bytes for every stranger who clones the SHA.
type BarTape struct {
	Bars *Bars
	Now  func() time.Time
}

func (t BarTape) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// visibleThrough is the latest IST date whose close is known at now.
func visibleThrough(now time.Time) string {
	n := now.In(marketclock.Location())
	closeAt := time.Date(n.Year(), n.Month(), n.Day(), 15, 30, 0, 0, n.Location())
	if n.Before(closeAt) {
		n = n.AddDate(0, 0, -1)
	}
	return n.Format("2006-01-02")
}

// visible returns symbol's bars with date ≤ visibleThrough(now).
func (t BarTape) visible(symbol string, now time.Time) []DailyBar {
	if t.Bars == nil {
		return nil
	}
	s := t.Bars.Series[symbol]
	cut := visibleThrough(now)
	n := sort.Search(len(s), func(i int) bool { return s[i].Date > cut })
	return s[:n]
}

func (t BarTape) ListUniverse(context.Context, *marketdatav1.ListUniverseRequest, ...grpc.CallOption) (*marketdatav1.ListUniverseResponse, error) {
	var out []*commonv1.Instrument
	for _, i := range TapeInstruments() {
		out = append(out, &commonv1.Instrument{
			Symbol: i.Symbol, Name: i.Name, Exchange: i.Exchange, Sector: i.Sector,
			Currency: i.Currency, IsBenchmark: i.IsBenchmark,
		})
	}
	return &marketdatav1.ListUniverseResponse{Instruments: out}, nil
}

func (t BarTape) GetQuotes(_ context.Context, req *marketdatav1.GetQuotesRequest, _ ...grpc.CallOption) (*marketdatav1.GetQuotesResponse, error) {
	now := t.now()
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range TapeInstruments() {
			syms = append(syms, i.Symbol)
		}
	}
	out := make([]*commonv1.Quote, 0, len(syms))
	for _, s := range syms {
		if t.Bars == nil || t.Bars.Series[s] == nil {
			continue // not on the tape (proxy benchmarks): no quote, not an error
		}
		v := t.visible(s, now)
		if len(v) == 0 {
			return nil, fmt.Errorf("campaign tape: no %s bar visible at %s", s, now.Format(time.RFC3339))
		}
		last := v[len(v)-1]
		chg := 0.0
		if len(v) > 1 && v[len(v)-2].Close > 0 {
			chg = (last.Close/v[len(v)-2].Close - 1) * 100
		}
		out = append(out, &commonv1.Quote{
			Symbol: s, Last: last.Close, ChangePct: chg,
			TsUnixMs: barClose(last.Date).UnixMilli(), Currency: "INR",
		})
	}
	return &marketdatav1.GetQuotesResponse{Quotes: out}, nil
}

// GetBars serves daily bars for every interval: the shipped roster is 1d,
// and the campaign's evidence is daily closes — nothing finer exists here.
func (t BarTape) GetBars(_ context.Context, req *marketdatav1.GetBarsRequest, _ ...grpc.CallOption) (*marketdatav1.GetBarsResponse, error) {
	now := t.now()
	count := int(req.Count)
	if count <= 0 {
		count = 40
	}
	v := t.visible(req.Symbol, now)
	if len(v) > count {
		v = v[len(v)-count:]
	}
	out := make([]*commonv1.Bar, 0, len(v))
	for _, b := range v {
		out = append(out, &commonv1.Bar{
			Symbol: req.Symbol, Interval: "1d", TsUnixMs: barClose(b.Date).UnixMilli(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return &marketdatav1.GetBarsResponse{Bars: out}, nil
}

func barClose(date string) time.Time {
	loc := marketclock.Location()
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 15, 30, 0, 0, loc)
}
