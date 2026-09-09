package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	advisorv1 "aperture/go/gen/advisor/v1"

	"google.golang.org/grpc"
)

type server struct {
	advisorv1.UnimplementedAdvisorServiceServer
}

func (s *server) Chat(ctx context.Context, req *advisorv1.ChatRequest) (*advisorv1.ChatResponse, error) {
	if os.Getenv("OPENAI_API_KEY") != "" {
		if reply := llm(req); reply != "" {
			return &advisorv1.ChatResponse{Reply: reply, Mode: "live"}, nil
		}
	}
	return &advisorv1.ChatResponse{Reply: heuristic(req), Mode: "mock"}, nil
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
		return "There is no promised −15% collar. You have a kill switch, per-trade caps, and cash buffer. Underperformance vs Nifty is a warning, not a halt."
	default:
		return fmt.Sprintf("Aperture is a paper lab: NSE hours (or clock override), multi-strategy book, parallel news investigations, and a measured +10pp excess target vs Indian indexes and top-fund peers. Ask about Nifty, investigations, or the journal. Your note: %q", req.Message)
	}
}

func llm(req *advisorv1.ChatRequest) string {
	key := os.Getenv("OPENAI_API_KEY")
	sys := "You are Aperture, an India paper-trading desk copilot. Not financial advice. Be concise. Use the JSON snapshots if present."
	user := req.Message + "\n\nportfolio:" + truncate(req.PortfolioJson, 2500) +
		"\nbenchmarks:" + truncate(req.BenchmarksJson, 1500) +
		"\njournal:" + truncate(req.JournalJson, 1500) +
		"\ninvestigations:" + truncate(req.InvestigationsJson, 1500)
	body := map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": user},
		},
	}
	b, _ := json.Marshal(body)
	httpReq, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(string(b)))
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &parsed) != nil || len(parsed.Choices) == 0 {
		return ""
	}
	return parsed.Choices[0].Message.Content
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func main() {
	addr := os.Getenv("ADVISOR_BIND")
	if addr == "" {
		addr = ":9085"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer()
	advisorv1.RegisterAdvisorServiceServer(gs, &server{})
	log.Printf("advisor listening on %s", addr)
	log.Fatal(gs.Serve(lis))
}
