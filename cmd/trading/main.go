package main

import (
	"log/slog"
	"os"

	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/trading"
	"aperture/pkg/grpcx"
	"aperture/pkg/llm"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	ctx, stop := serve.Context()
	defer stop()
	cfg := trading.LoadConfig()
	ln := learningv1.NewLearningServiceClient(grpcx.MustDial(ctx, cfg.LearningAddr))
	svc := trading.New(trading.Deps{
		MarketData: marketdatav1.NewMarketDataServiceClient(grpcx.MustDial(ctx, cfg.MarketdataAddr)),
		Learning:   ln,
		Sentiment:  sentimentv1.NewSentimentServiceClient(grpcx.MustDial(ctx, cfg.SentimentAddr)),
		LLM:        llm.FromEnv(),
		Log:        slog.Default(),
		Cfg:        cfg,
	})
	if wresp, err := ln.GetWeights(ctx, &learningv1.GetWeightsRequest{}); err == nil {
		svc.SeedWeights(wresp.Weights)
	}
	go svc.Loop(ctx)
	if cfg.Autostart {
		go svc.Autostart(ctx)
	}
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		tradingv1.RegisterTradingServiceServer(s, svc)
	}); err != nil {
		slog.Error("trading", "err", err)
		os.Exit(1)
	}
}
