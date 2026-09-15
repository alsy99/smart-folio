package advisor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	advisorv1 "aperture/gen/advisor/v1"
	"aperture/pkg/config"
	"aperture/pkg/llm"
)

type Config struct {
	Bind string
}

func LoadConfig() Config {
	return Config{Bind: config.String("ADVISOR_BIND", ":9085")}
}

type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, system, user string) (string, error)
}

type Service struct {
	advisorv1.UnimplementedAdvisorServiceServer
	llm Completer
}

func New(c Completer) *Service {
	if c == nil {
		c = llm.FromEnv()
	}
	return &Service{llm: c}
}

func (s *Service) Chat(ctx context.Context, req *advisorv1.ChatRequest) (*advisorv1.ChatResponse, error) {
	if s.llm != nil && s.llm.Enabled() {
		if reply, err := s.llm.Complete(ctx, systemPrompt, userPrompt(req)); err == nil && reply != "" {
			return &advisorv1.ChatResponse{Reply: reply, Mode: "live"}, nil
		} else if err != nil {
			slog.Warn("advisor llm", "err", err)
		}
	}
	return &advisorv1.ChatResponse{Reply: heuristic(req), Mode: "mock"}, nil
}

const systemPrompt = "You are Aperture, an India paper-trading desk copilot. Not financial advice. Be concise. Use the JSON snapshots if present."

func userPrompt(req *advisorv1.ChatRequest) string {
	return req.Message + "\n\nportfolio:" + truncate(req.PortfolioJson, 2500) +
		"\nbenchmarks:" + truncate(req.BenchmarksJson, 1500) +
		"\njournal:" + truncate(req.JournalJson, 1500) +
		"\ninvestigations:" + truncate(req.InvestigationsJson, 1500)
}

func heuristic(req *advisorv1.ChatRequest) string {
	msg := strings.ToLower(req.Message)
	switch {
	case strings.Contains(msg, "nifty") || strings.Contains(msg, "benchmark"):
		return "North star is +10 percentage points of excess vs Nifty 50 (and peers), annualized. The 30-day campaign shows a run-rate, not a guaranteed year. Check the benchmark table for on-track badges and beat rate."
	case strings.Contains(msg, "news") || strings.Contains(msg, "invest"):
		return "Investigations fan out in parallel over clustered headlines from NewsAPI, Currents, and Finnhub (mocked when keys are missing). Each report tilts the strategy roster; it does not invent a new strategy per article."
	case strings.Contains(msg, "why") || strings.Contains(msg, "learn"):
		return "After every closed paper trade the learning service tags whether it helped or hurt vs Nifty and updates strategy weights with an exploration floor so weak methods still get trials."
	case strings.Contains(msg, "risk") || strings.Contains(msg, "loss"):
		return "15% book drawdown halts new paper buys. Each name is capped at 8% of equity. Holds are days to weeks — the 45-second scalp path is off unless SCALP_MODE=true."
	case strings.Contains(msg, "backtest") || strings.Contains(msg, "five year") || strings.Contains(msg, "5 year"):
		return "The learning service backtests a grid of methods and timeframes over a 5-year weekday mock tape, then promotes variants that beat Nifty into the live roster. That is research, not a promise of live excess."
	default:
		return fmt.Sprintf("Aperture is a positional NSE cash paper book: fills only in session hours (09:15–15:30 IST); research runs 24x7. Daily-bar roster, 8%% name cap, 15%% book halt, +10pp excess target vs Nifty. Ask about Nifty, investigations, or the journal. Your note: %q", req.Message)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
