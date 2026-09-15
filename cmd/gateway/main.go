package main

import (
	"log/slog"
	"os"

	advisorv1 "aperture/gen/advisor/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	policyv1 "aperture/gen/policy/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/gateway"
	"aperture/pkg/grpcx"
	"aperture/pkg/serve"
)

func main() {
	serve.Logging()
	slog.SetDefault(slog.Default().With("svc", "gateway"))
	ctx, stop := serve.Context()
	defer stop()
	cfg := gateway.LoadConfig()
	trConn := grpcx.MustDial(ctx, cfg.TradingAddr)
	api := gateway.New(slog.Default(), gateway.Clients{
		MarketData: marketdatav1.NewMarketDataServiceClient(grpcx.MustDial(ctx, cfg.MarketdataAddr)),
		Trading:    tradingv1.NewTradingServiceClient(trConn),
		Policy:     policyv1.NewPolicyServiceClient(trConn),
		Learning:   learningv1.NewLearningServiceClient(grpcx.MustDial(ctx, cfg.LearningAddr)),
		Sentiment:  sentimentv1.NewSentimentServiceClient(grpcx.MustDial(ctx, cfg.SentimentAddr)),
		Advisor:    advisorv1.NewAdvisorServiceClient(grpcx.MustDial(ctx, cfg.AdvisorAddr)),
	})
	if err := serve.HTTP(ctx, cfg.Bind, api.Handler()); err != nil {
		slog.Error("gateway", "err", err)
		os.Exit(1)
	}
}
