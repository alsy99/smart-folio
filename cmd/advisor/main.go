package main

import (
	"log/slog"
	"os"

	advisorv1 "aperture/gen/advisor/v1"
	"aperture/internal/advisor"
	"aperture/pkg/llm"
	"aperture/pkg/serve"

	"google.golang.org/grpc"
)

func main() {
	serve.Logging()
	slog.SetDefault(slog.Default().With("svc", "advisor"))
	ctx, stop := serve.Context()
	defer stop()
	cfg := advisor.LoadConfig()
	svc := advisor.New(llm.FromEnv())
	if err := serve.GRPC(ctx, cfg.Bind, func(s *grpc.Server) {
		advisorv1.RegisterAdvisorServiceServer(s, svc)
	}); err != nil {
		slog.Error("advisor", "err", err)
		os.Exit(1)
	}
}
