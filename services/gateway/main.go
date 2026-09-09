package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	advisorv1 "aperture/go/gen/advisor/v1"
	learningv1 "aperture/go/gen/learning/v1"
	marketdatav1 "aperture/go/gen/marketdata/v1"
	sentimentv1 "aperture/go/gen/sentiment/v1"
	tradingv1 "aperture/go/gen/trading/v1"
	"aperture/pkg/grpcx"

	"google.golang.org/protobuf/encoding/protojson"
)

type api struct {
	md marketdatav1.MarketDataServiceClient
	tr tradingv1.TradingServiceClient
	ln learningv1.LearningServiceClient
	sn sentimentv1.SentimentServiceClient
	ad advisorv1.AdvisorServiceClient
}

var marshaler = protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: false}

func envAddr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *api) handle(w http.ResponseWriter, r *http.Request) {
	timeout := 15 * time.Second
	if r.URL.Path == "/backtest" {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	switch r.URL.Path {
	case "/health":
		writeJSON(w, 200, map[string]string{"status": "ok"})
	case "/universe":
		resp, err := a.md.ListUniverse(ctx, &marketdatav1.ListUniverseRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/portfolio":
		resp, err := a.tr.GetPortfolio(ctx, &tradingv1.GetPortfolioRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/campaign":
		if r.Method == http.MethodPost {
			var body struct {
				Days int32 `json:"days"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			resp, err := a.tr.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: body.Days})
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			b, _ := marshaler.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		resp, err := a.tr.GetCampaign(ctx, &tradingv1.GetCampaignRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/autopilot":
		var body struct {
			Enabled bool `json:"enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		resp, err := a.tr.SetAutopilot(ctx, &tradingv1.SetAutopilotRequest{Enabled: body.Enabled})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/benchmarks":
		resp, err := a.tr.GetBenchmarks(ctx, &tradingv1.GetBenchmarksRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/paper/tick":
		resp, err := a.tr.Tick(ctx, &tradingv1.TickRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/journal":
		resp, err := a.ln.ListJournal(ctx, &learningv1.ListJournalRequest{Limit: 50})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/weights":
		resp, err := a.ln.GetWeights(ctx, &learningv1.GetWeightsRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/backtest":
		if r.Method == http.MethodPost {
			var body struct {
				Years int32 `json:"years"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Years == 0 {
				body.Years = 5
			}
			resp, err := a.ln.RunBacktest(ctx, &learningv1.RunBacktestRequest{Years: body.Years})
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			b, _ := marshaler.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		resp, err := a.ln.GetBacktest(ctx, &learningv1.GetBacktestRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/sentiment":
		resp, err := a.sn.ScoreSymbols(ctx, &sentimentv1.ScoreSymbolsRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/news":
		resp, err := a.sn.ListNews(ctx, &sentimentv1.ListNewsRequest{Limit: 40})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/investigations":
		resp, err := a.sn.GetInvestigations(ctx, &sentimentv1.GetInvestigationsRequest{})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case "/advisor/chat":
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &body)
		port, _ := a.tr.GetPortfolio(ctx, &tradingv1.GetPortfolioRequest{})
		bench, _ := a.tr.GetBenchmarks(ctx, &tradingv1.GetBenchmarksRequest{})
		jour, _ := a.ln.ListJournal(ctx, &learningv1.ListJournalRequest{Limit: 8})
		inv, _ := a.sn.GetInvestigations(ctx, &sentimentv1.GetInvestigationsRequest{})
		pj, _ := marshaler.Marshal(port)
		bj, _ := marshaler.Marshal(bench)
		jj, _ := marshaler.Marshal(jour)
		ij, _ := marshaler.Marshal(inv)
		resp, err := a.ad.Chat(ctx, &advisorv1.ChatRequest{
			Message: body.Message, PortfolioJson: string(pj), BenchmarksJson: string(bj),
			JournalJson: string(jj), InvestigationsJson: string(ij),
		})
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := marshaler.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	default:
		http.NotFound(w, r)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	a := &api{
		md: marketdatav1.NewMarketDataServiceClient(grpcx.MustDial(envAddr("MARKETDATA_ADDR", "127.0.0.1:9081"))),
		tr: tradingv1.NewTradingServiceClient(grpcx.MustDial(envAddr("TRADING_ADDR", "127.0.0.1:9082"))),
		ln: learningv1.NewLearningServiceClient(grpcx.MustDial(envAddr("LEARNING_ADDR", "127.0.0.1:9083"))),
		sn: sentimentv1.NewSentimentServiceClient(grpcx.MustDial(envAddr("SENTIMENT_ADDR", "127.0.0.1:9084"))),
		ad: advisorv1.NewAdvisorServiceClient(grpcx.MustDial(envAddr("ADVISOR_ADDR", "127.0.0.1:9085"))),
	}
	addr := os.Getenv("GATEWAY_BIND")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handle)
	log.Printf("gateway listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}
