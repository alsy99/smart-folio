package marketdata

import (
	"context"
	"log/slog"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/config"
	"aperture/pkg/indstocks"
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
	now  func() time.Time
	live *indstocks.Client
}

func New() *Service {
	c := indstocks.Shared()
	if c != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := c.EnsureScrips(ctx); err != nil {
			slog.Warn("indstocks scrips", "err", err)
		} else if err := c.Profile(ctx); err != nil {
			slog.Warn("indstocks profile", "err", err)
		} else {
			slog.Info("indstocks quotes", "mode", "live", "scrips", c.ScripCount())
		}
	} else {
		slog.Info("indstocks quotes", "mode", "mock")
	}
	return &Service{now: time.Now, live: c}
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

func (s *Service) GetQuotes(ctx context.Context, req *marketdatav1.GetQuotesRequest) (*marketdatav1.GetQuotesResponse, error) {
	now := s.now()
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range universe.All() {
			syms = append(syms, i.Symbol)
		}
	}
	live := map[string]indstocks.Quote{}
	if s.live != nil {
		if qs, err := s.live.Quotes(ctx, syms); err != nil {
			slog.Warn("indstocks quotes", "err", err)
		} else {
			live = qs
		}
	}
	out := make([]*commonv1.Quote, 0, len(syms))
	for _, sym := range syms {
		q := &commonv1.Quote{Symbol: sym, TsUnixMs: now.UnixMilli(), Currency: "INR"}
		if lq, ok := live[sym]; ok && lq.LivePrice > 0 {
			q.Last = lq.LivePrice
			q.ChangePct = lq.DayChangePct
		} else {
			q.Last = prices.Last(sym, now)
			q.ChangePct = prices.ChangePct(sym, now)
		}
		out = append(out, q)
	}
	return &marketdatav1.GetQuotesResponse{Quotes: out}, nil
}

func (s *Service) GetBars(ctx context.Context, req *marketdatav1.GetBarsRequest) (*marketdatav1.GetBarsResponse, error) {
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
	if s.live != nil {
		bars, err := s.live.Bars(ctx, req.Symbol, interval, count, now)
		if err != nil {
			slog.Warn("indstocks bars", "symbol", req.Symbol, "err", err)
			raw = nil
		} else if len(bars) > 0 {
			raw = bars
		} else {
			raw = nil
		}
	}
	out := make([]*commonv1.Bar, 0, len(raw))
	for _, b := range raw {
		out = append(out, &commonv1.Bar{
			Symbol: req.Symbol, Interval: interval, TsUnixMs: b.Ts.UnixMilli(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return &marketdatav1.GetBarsResponse{Bars: out}, nil
}
