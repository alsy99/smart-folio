package main

import (
	"log/slog"
	"os"

	sentimentv1 "aperture/gen/sentiment/v1"
	"aperture/internal/sentiment"
	"aperture/pkg/llm"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	slog.SetDefault(slog.Default().With("svc", "sentiment"))
	ctx, stop := serve.Context()
	defer stop()
	cfg := sentiment.LoadConfig()
	svc := sentiment.New(cfg, slog.Default(), llm.FromEnv(), nil)
	go svc.Refresh(ctx, false)
	go svc.Loop(ctx)
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		sentimentv1.RegisterSentimentServiceServer(s, svc)
	}); err != nil {
		slog.Error("sentiment", "err", err)
		os.Exit(1)
	}
}
