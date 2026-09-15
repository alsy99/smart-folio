package main

import (
	"log/slog"
	"os"

	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/internal/marketdata"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	slog.SetDefault(slog.Default().With("svc", "marketdata"))
	ctx, stop := serve.Context()
	defer stop()
	cfg := marketdata.LoadConfig()
	svc := marketdata.New()
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		marketdatav1.RegisterMarketDataServiceServer(s, svc)
	}); err != nil {
		slog.Error("marketdata", "err", err)
		os.Exit(1)
	}
}
