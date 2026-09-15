package main

import (
	"log/slog"
	"os"

	learningv1 "aperture/gen/learning/v1"
	"aperture/internal/learning"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	ctx, stop := serve.Context()
	defer stop()
	cfg := learning.LoadConfig()
	svc := learning.New(slog.Default())
	go svc.Seed(ctx)
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		learningv1.RegisterLearningServiceServer(s, svc)
	}); err != nil {
		slog.Error("learning", "err", err)
		os.Exit(1)
	}
}
