package prices

import (
	"context"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

// Tape is a deterministic mock market-data client. Quotes and bars are
// prices.Last / prices.Bars at Now() — the same function a stranger gets
// after cloning the SHA. No INDstocks, no wall clock.
type Tape struct {
	Now func() time.Time
}

func (t Tape) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (t Tape) ListUniverse(context.Context, *marketdatav1.ListUniverseRequest, ...grpc.CallOption) (*marketdatav1.ListUniverseResponse, error) {
	var out []*commonv1.Instrument
	for _, i := range universe.All() {
		out = append(out, &commonv1.Instrument{
			Symbol: i.Symbol, Name: i.Name, Exchange: i.Exchange, Sector: i.Sector,
			Currency: i.Currency, IsBenchmark: i.IsBenchmark,
		})
	}
	return &marketdatav1.ListUniverseResponse{Instruments: out}, nil
}

func (t Tape) GetQuotes(_ context.Context, req *marketdatav1.GetQuotesRequest, _ ...grpc.CallOption) (*marketdatav1.GetQuotesResponse, error) {
	now := t.now()
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range universe.All() {
			syms = append(syms, i.Symbol)
		}
	}
	out := make([]*commonv1.Quote, 0, len(syms))
	for _, s := range syms {
		out = append(out, &commonv1.Quote{
			Symbol:    s,
			Last:      Last(s, now),
			ChangePct: ChangePct(s, now),
			TsUnixMs:  now.UnixMilli(),
			Currency:  "INR",
		})
	}
	return &marketdatav1.GetQuotesResponse{Quotes: out}, nil
}

func (t Tape) GetBars(_ context.Context, req *marketdatav1.GetBarsRequest, _ ...grpc.CallOption) (*marketdatav1.GetBarsResponse, error) {
	now := t.now()
	interval := req.Interval
	if interval == "" {
		interval = "15m"
	}
	count := int(req.Count)
	if count <= 0 {
		count = 40
	}
	raw := Bars(req.Symbol, interval, count, now)
	out := make([]*commonv1.Bar, 0, len(raw))
	for _, b := range raw {
		out = append(out, &commonv1.Bar{
			Symbol: req.Symbol, Interval: interval, TsUnixMs: b.Ts.UnixMilli(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return &marketdatav1.GetBarsResponse{Bars: out}, nil
}
