package main

import (
	"log/slog"
	"os"

	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	policyv1 "aperture/gen/policy/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/policy"
	"aperture/internal/trading"
	"aperture/pkg/campaign"
	"aperture/pkg/config"
	"aperture/pkg/grpcx"
	"aperture/pkg/llm"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	slog.SetDefault(slog.Default().With("svc", "trading"))
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
	// Policy is hosted on the trading process: one binary, two services.
	// Policy decides targets; trading executes them.
	store := policy.NewFileStore(config.String("IPS_DIR", "data/ips"))
	pol := policy.New(policy.Deps{
		Store:  store,
		Frozen: campaign.FrozenIPS,
		Book:   svc,
		OnPut:  svc.BindIPS,
		Log:    slog.Default(),
	})
	// Boot under the statement named by IPS_ID, if it is on disk, so a
	// restart does not silently drop the core.
	if id := config.String("IPS_ID", ""); id != "" {
		if p, err := store.Get(id); err == nil {
			svc.BindIPS(p)
		} else {
			slog.Warn("IPS_ID not found; book runs without a core until one is put", "id", id, "err", err)
		}
	}
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		tradingv1.RegisterTradingServiceServer(s, svc)
		policyv1.RegisterPolicyServiceServer(s, pol)
	}); err != nil {
		slog.Error("trading", "err", err)
		os.Exit(1)
	}
}
