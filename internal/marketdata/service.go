package marketdata

import (
	"context"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/config"
	"aperture/pkg/prices"
	"aperture/pkg/universe"
)

type Config struct {
	Bind string
}

func LoadConfig() Config {
	return Config{Bind: config.String("MARKETDATA_BIND", ":9081")}
}

type Service struct {
	marketdatav1.UnimplementedMarketDataServiceServer
	now func() time.Time
}

func New() *Service {
	return &Service{now: time.Now}
}

func (s *Service) ListUniverse(_ context.Context, _ *marketdatav1.ListUniverseRequest) (*marketdatav1.ListUniverseResponse, error) {
	var out []*commonv1.Instrument
	for _, i := range universe.All() {
		out = append(out, &commonv1.Instrument{
			Symbol: i.Symbol, Name: i.Name, Exchange: i.Exchange, Sector: i.Sector,
			Currency: i.Currency, IsBenchmark: i.IsBenchmark,
		})
	}
	return &marketdatav1.ListUniverseResponse{Instruments: out}, nil
}

func (s *Service) GetQuotes(_ context.Context, req *marketdatav1.GetQuotesRequest) (*marketdatav1.GetQuotesResponse, error) {
	now := s.now()
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range universe.All() {
			syms = append(syms, i.Symbol)
		}
	}
	out := make([]*commonv1.Quote, 0, len(syms))
	for _, sym := range syms {
		out = append(out, &commonv1.Quote{
			Symbol:    sym,
			Last:      prices.Last(sym, now),
			ChangePct: prices.ChangePct(sym, now),
			TsUnixMs:  now.UnixMilli(),
			Currency:  "INR",
		})
	}
	return &marketdatav1.GetQuotesResponse{Quotes: out}, nil
}

func (s *Service) GetBars(_ context.Context, req *marketdatav1.GetBarsRequest) (*marketdatav1.GetBarsResponse, error) {
	now := s.now()
	interval := req.Interval
	if interval == "" {
		interval = "15m"
	}
	count := int(req.Count)
	if count <= 0 {
		count = 40
	}
	raw := prices.Bars(req.Symbol, interval, count, now)
	out := make([]*commonv1.Bar, 0, len(raw))
	for _, b := range raw {
		out = append(out, &commonv1.Bar{
			Symbol: req.Symbol, Interval: interval, TsUnixMs: b.Ts.UnixMilli(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return &marketdatav1.GetBarsResponse{Bars: out}, nil
}
