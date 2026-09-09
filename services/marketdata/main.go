package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	commonv1 "aperture/go/gen/common/v1"
	marketdatav1 "aperture/go/gen/marketdata/v1"
	"aperture/pkg/prices"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

type server struct {
	marketdatav1.UnimplementedMarketDataServiceServer
}

func (s *server) ListUniverse(ctx context.Context, _ *marketdatav1.ListUniverseRequest) (*marketdatav1.ListUniverseResponse, error) {
	var out []*commonv1.Instrument
	for _, i := range universe.All() {
		out = append(out, &commonv1.Instrument{
			Symbol: i.Symbol, Name: i.Name, Exchange: i.Exchange, Sector: i.Sector,
			Currency: i.Currency, IsBenchmark: i.IsBenchmark,
		})
	}
	return &marketdatav1.ListUniverseResponse{Instruments: out}, nil
}

func (s *server) GetQuotes(ctx context.Context, req *marketdatav1.GetQuotesRequest) (*marketdatav1.GetQuotesResponse, error) {
	now := time.Now()
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range universe.All() {
			syms = append(syms, i.Symbol)
		}
	}
	var out []*commonv1.Quote
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

func (s *server) GetBars(ctx context.Context, req *marketdatav1.GetBarsRequest) (*marketdatav1.GetBarsResponse, error) {
	now := time.Now()
	interval := req.Interval
	if interval == "" {
		interval = "15m"
	}
	count := int(req.Count)
	if count <= 0 {
		count = 40
	}
	raw := prices.Bars(req.Symbol, interval, count, now)
	var out []*commonv1.Bar
	for _, b := range raw {
		out = append(out, &commonv1.Bar{
			Symbol: req.Symbol, Interval: interval, TsUnixMs: b.Ts.UnixMilli(),
			Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return &marketdatav1.GetBarsResponse{Bars: out}, nil
}

func main() {
	addr := os.Getenv("MARKETDATA_BIND")
	if addr == "" {
		addr = ":9081"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	s := grpc.NewServer()
	marketdatav1.RegisterMarketDataServiceServer(s, &server{})
	log.Printf("marketdata listening on %s", addr)
	log.Fatal(s.Serve(lis))
}
