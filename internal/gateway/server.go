package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	advisorv1 "aperture/gen/advisor/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	policyv1 "aperture/gen/policy/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/campaign"
	"aperture/pkg/config"
	"aperture/pkg/ratelimit"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	Bind           string
	MarketdataAddr string
	TradingAddr    string
	LearningAddr   string
	SentimentAddr  string
	AdvisorAddr    string
}

func LoadConfig() Config {
	return Config{
		Bind:           config.String("GATEWAY_BIND", ":8080"),
		MarketdataAddr: config.String("MARKETDATA_ADDR", "127.0.0.1:9081"),
		TradingAddr:    config.String("TRADING_ADDR", "127.0.0.1:9082"),
		LearningAddr:   config.String("LEARNING_ADDR", "127.0.0.1:9083"),
		SentimentAddr:  config.String("SENTIMENT_ADDR", "127.0.0.1:9084"),
		AdvisorAddr:    config.String("ADVISOR_ADDR", "127.0.0.1:9085"),
	}
}

type API struct {
	log *slog.Logger
	md  marketdatav1.MarketDataServiceClient
	tr  tradingv1.TradingServiceClient
	ln  learningv1.LearningServiceClient
	sn  sentimentv1.SentimentServiceClient
	ad  advisorv1.AdvisorServiceClient
	pl  policyv1.PolicyServiceClient
}

type Clients struct {
	MarketData marketdatav1.MarketDataServiceClient
	Trading    tradingv1.TradingServiceClient
	Learning   learningv1.LearningServiceClient
	Sentiment  sentimentv1.SentimentServiceClient
	Advisor    advisorv1.AdvisorServiceClient
	Policy     policyv1.PolicyServiceClient
}

func New(log *slog.Logger, c Clients) *API {
	if log == nil {
		log = slog.Default()
	}
	return &API{log: log, md: c.MarketData, tr: c.Trading, ln: c.Learning, sn: c.Sentiment, ad: c.Advisor, pl: c.Policy}
}

var marshaler = protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: false}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /indstocks", a.health)
	mux.HandleFunc("GET /universe", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.md.ListUniverse(ctx, &marketdatav1.ListUniverseRequest{})
	}))
	mux.HandleFunc("GET /portfolio", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.tr.GetPortfolio(ctx, &tradingv1.GetPortfolioRequest{})
	}))
	mux.HandleFunc("GET /campaign", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.tr.GetCampaign(ctx, &tradingv1.GetCampaignRequest{})
	}))
	mux.HandleFunc("GET /public-campaign", a.publicCampaign)
	mux.HandleFunc("GET /public-campaigns", a.publicCampaigns)
	mux.HandleFunc("POST /campaign", func(w http.ResponseWriter, r *http.Request) {
		if campaign.IsFrozen() {
			http.Error(w, "public 30-day campaign is frozen; same git SHA, settings, universe, and cost model. Clone and go run ./cmd/campaign -verify.", http.StatusConflict)
			return
		}
		a.unary(45*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
			var body struct {
				Days int32 `json:"days"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			camp, err := a.tr.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: body.Days})
			if err != nil {
				return nil, err
			}
			a.log.Info("campaign start, refreshing news")
			if _, err := a.sn.ListNews(ctx, &sentimentv1.ListNewsRequest{Limit: -1}); err != nil {
				a.log.Warn("campaign news refresh", "err", err)
			}
			return camp, nil
		})(w, r)
	})
	mux.HandleFunc("POST /autopilot", a.unary(15*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		return a.tr.SetAutopilot(ctx, &tradingv1.SetAutopilotRequest{Enabled: body.Enabled})
	}))
	mux.HandleFunc("GET /benchmarks", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.tr.GetBenchmarks(ctx, &tradingv1.GetBenchmarksRequest{})
	}))
	mux.HandleFunc("POST /paper/tick", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.tr.Tick(ctx, &tradingv1.TickRequest{})
	}))
	mux.HandleFunc("GET /journal", a.unaryFallback(15*time.Second, &learningv1.ListJournalResponse{}, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.ln.ListJournal(ctx, &learningv1.ListJournalRequest{Limit: 50})
	}))
	mux.HandleFunc("GET /weights", a.unaryFallback(15*time.Second, &learningv1.GetWeightsResponse{}, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.ln.GetWeights(ctx, &learningv1.GetWeightsRequest{})
	}))
	mux.HandleFunc("GET /backtest", a.unaryFallback(15*time.Second, &learningv1.BacktestReport{}, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.ln.GetBacktest(ctx, &learningv1.GetBacktestRequest{})
	}))
	mux.HandleFunc("POST /backtest", a.unary(90*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
		var body struct {
			Years int32 `json:"years"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Years == 0 {
			body.Years = 5
		}
		return a.ln.RunBacktest(ctx, &learningv1.RunBacktestRequest{Years: body.Years})
	}))
	mux.HandleFunc("GET /sentiment", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.sn.ScoreSymbols(ctx, &sentimentv1.ScoreSymbolsRequest{})
	}))
	mux.HandleFunc("GET /news", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.sn.ListNews(ctx, &sentimentv1.ListNewsRequest{Limit: 40})
	}))
	mux.HandleFunc("GET /investigations", a.unary(15*time.Second, func(ctx context.Context, _ *http.Request) (proto.Message, error) {
		return a.sn.GetInvestigations(ctx, &sentimentv1.GetInvestigationsRequest{})
	}))
	mux.HandleFunc("GET /research", a.research)
	mux.HandleFunc("POST /advisor/chat", a.chat)
	// Policy: the IPS is typed fields only. The body is decoded with
	// protojson so unknown or free-text fields are rejected, not ignored.
	mux.HandleFunc("PUT /ips", a.unary(15*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
		raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<16))
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		var in policyv1.IPS
		if err := protojson.Unmarshal(raw, &in); err != nil {
			return nil, status.Error(codes.InvalidArgument, "ips: "+err.Error())
		}
		return a.pl.PutIPS(ctx, &policyv1.PutIPSRequest{Ips: &in})
	}))
	mux.HandleFunc("GET /ips/{id}", a.unary(15*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
		return a.pl.GetIPS(ctx, &policyv1.GetIPSRequest{Id: r.PathValue("id")})
	}))
	mux.HandleFunc("POST /ips/{id}/preview", a.unary(15*time.Second, func(ctx context.Context, r *http.Request) (proto.Message, error) {
		return a.pl.PreviewTargets(ctx, &policyv1.PreviewTargetsRequest{Id: r.PathValue("id")})
	}))
	rps := config.Int("GATEWAY_RPS", 40)
	burst := config.Int("GATEWAY_BURST", 80)
	return chain(recoverer(a.log), cors, ratelimit.New(rps, burst).Middleware, requestLog(a.log))(mux)
}

func (a *API) publicCampaign(w http.ResponseWriter, _ *http.Request) {
	led, err := campaign.Load("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(led)
}

func (a *API) publicCampaigns(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Campaigns []*campaign.Ledger `json:"campaigns"`
	}{Campaigns: campaign.LoadAll()})
}

func (a *API) research(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	b, err := os.ReadFile("data/picks.json")
	if err != nil || len(b) == 0 {
		_, _ = w.Write([]byte(`{"picks":[]}`))
		return
	}
	_, _ = w.Write([]byte(`{"picks":`))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte(`}`))
}

func (a *API) unary(timeout time.Duration, fn func(context.Context, *http.Request) (proto.Message, error)) http.HandlerFunc {
	return a.unaryFallback(timeout, nil, fn)
}

func (a *API) unaryFallback(timeout time.Duration, fallback proto.Message, fn func(context.Context, *http.Request) (proto.Message, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		msg, err := fn(ctx, r)
		if err != nil {
			if fallback != nil {
				if st, ok := status.FromError(err); ok && (st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded) {
					a.log.Warn("gateway", "path", r.URL.Path, "err", err)
					msg = fallback
					err = nil
				}
			}
			if err != nil {
				a.log.Error("gateway", "path", r.URL.Path, "err", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		b, err := marshaler.Marshal(msg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}
}

func (a *API) chat(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
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
		a.log.Error("advisor chat", "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	b, err := marshaler.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

type middleware func(http.Handler) http.Handler

func chain(mw ...middleware) middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestLog(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			if r.Method != http.MethodOptions && r.URL.Path != "/health" {
				log.Info("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
			}
		})
	}
}

func recoverer(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "path", r.URL.Path)
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
