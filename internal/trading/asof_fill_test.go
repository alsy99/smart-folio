package trading

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/broker"
	"aperture/pkg/marketclock"
	"aperture/pkg/research"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

func friday1525() time.Time {
	return time.Date(2026, 9, 11, 15, 25, 0, 0, marketclock.Location())
}

func friday1600() time.Time {
	return time.Date(2026, 9, 11, 16, 0, 0, 0, marketclock.Location())
}

type mdStub struct{}

func (mdStub) ListUniverse(context.Context, *marketdatav1.ListUniverseRequest, ...grpc.CallOption) (*marketdatav1.ListUniverseResponse, error) {
	return &marketdatav1.ListUniverseResponse{}, nil
}

func (mdStub) GetQuotes(_ context.Context, req *marketdatav1.GetQuotesRequest, _ ...grpc.CallOption) (*marketdatav1.GetQuotesResponse, error) {
	syms := req.Symbols
	if len(syms) == 0 {
		for _, i := range universe.All() {
			syms = append(syms, i.Symbol)
		}
	}
	out := make([]*commonv1.Quote, 0, len(syms))
	for _, s := range syms {
		px := 1000.0
		if s == "NIFTY50" || s == "NIFTY500" || s == "SENSEX" {
			px = 25000
		}
		out = append(out, &commonv1.Quote{Symbol: s, Last: px, Currency: "INR"})
	}
	return &marketdatav1.GetQuotesResponse{Quotes: out}, nil
}

func (mdStub) GetBars(_ context.Context, req *marketdatav1.GetBarsRequest, _ ...grpc.CallOption) (*marketdatav1.GetBarsResponse, error) {
	n := int(req.Count)
	if n <= 0 {
		n = 40
	}
	bars := make([]*commonv1.Bar, n)
	for i := 0; i < n; i++ {
		px := 900 + float64(i)*4 // rising 1d tape so momentum can fire without news
		bars[i] = &commonv1.Bar{
			Symbol: req.Symbol, Interval: req.Interval,
			Open: px, High: px * 1.004, Low: px * 0.996, Close: px, Volume: 2e6,
		}
	}
	return &marketdatav1.GetBarsResponse{Bars: bars}, nil
}

type lnStub struct{}

func (lnStub) RecordTrade(context.Context, *learningv1.RecordTradeRequest, ...grpc.CallOption) (*learningv1.RecordTradeResponse, error) {
	return &learningv1.RecordTradeResponse{}, nil
}
func (lnStub) ListJournal(context.Context, *learningv1.ListJournalRequest, ...grpc.CallOption) (*learningv1.ListJournalResponse, error) {
	return &learningv1.ListJournalResponse{}, nil
}
func (lnStub) GetWeights(context.Context, *learningv1.GetWeightsRequest, ...grpc.CallOption) (*learningv1.GetWeightsResponse, error) {
	ids := strategies.IDs()
	eq := 1.0 / float64(len(ids))
	var w []*commonv1.StrategyWeight
	for _, id := range ids {
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: eq})
	}
	return &learningv1.GetWeightsResponse{Weights: w}, nil
}
func (lnStub) RunBacktest(context.Context, *learningv1.RunBacktestRequest, ...grpc.CallOption) (*learningv1.BacktestReport, error) {
	return &learningv1.BacktestReport{}, nil
}
func (lnStub) GetBacktest(context.Context, *learningv1.GetBacktestRequest, ...grpc.CallOption) (*learningv1.BacktestReport, error) {
	return &learningv1.BacktestReport{}, nil
}

type snStub struct {
	reports []*commonv1.InvestigationReport
}

func (s *snStub) ScoreSymbols(context.Context, *sentimentv1.ScoreSymbolsRequest, ...grpc.CallOption) (*sentimentv1.ScoreSymbolsResponse, error) {
	return &sentimentv1.ScoreSymbolsResponse{}, nil
}
func (s *snStub) ListNews(context.Context, *sentimentv1.ListNewsRequest, ...grpc.CallOption) (*sentimentv1.ListNewsResponse, error) {
	return &sentimentv1.ListNewsResponse{}, nil
}
func (s *snStub) GetInvestigations(context.Context, *sentimentv1.GetInvestigationsRequest, ...grpc.CallOption) (*sentimentv1.GetInvestigationsResponse, error) {
	return &sentimentv1.GetInvestigationsResponse{Reports: s.reports}, nil
}

func tickBook(t *testing.T, now time.Time, reports []*commonv1.InvestigationReport) *tradingv1.TickResponse {
	t.Helper()
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	svc := New(Deps{MarketData: mdStub{}, Learning: lnStub{}, Sentiment: &snStub{reports: reports}, Now: func() time.Time { return now }})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.Tick(ctx, &tradingv1.TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func lateArticle(standAside bool, score float64) *commonv1.InvestigationReport {
	return &commonv1.InvestigationReport{
		Id: "fri-1600", Headline: "TCS beats after the cash close",
		Symbols: universe.EquitySymbols(), EventType: "earnings", Stance: "bullish",
		Score: score, Confidence: 0.9, StandAside: standAside,
		Sources: []*commonv1.NewsItem{{
			Id: "n-1600", Title: "TCS beats after the cash close",
			PublishedAtUnixMs: friday1600().UnixMilli(), Symbols: []string{"TCS"},
		}},
		StrategyImplications: []*commonv1.StrategyTilt{
			{StrategyId: strategies.SentimentTilt, Tilt: score},
			{StrategyId: strategies.Momentum1d, Tilt: 0.5},
		},
	}
}

func articleAt(when time.Time, standAside bool, score float64) *commonv1.InvestigationReport {
	r := lateArticle(standAside, score)
	r.Id = when.Format("15:04")
	r.Sources[0].PublishedAtUnixMs = when.UnixMilli()
	return r
}

// TestFriday1600ArticleCannotMoveFriday1525Fill is the look-ahead acceptance:
// a headline printed at 16:00 IST must not change fills decided at 15:25.
func TestFriday1600ArticleCannotMoveFriday1525Fill(t *testing.T) {
	bar := friday1525()
	if !marketclock.IsOpen(bar) {
		t.Fatalf("fixture must be inside the cash session, got %s", marketclock.SessionStatus(bar))
	}
	base := tickBook(t, bar, nil)
	if base.Fills == 0 {
		t.Fatal("fixture must produce fills so a leaked stand-aside can be detected")
	}
	sessionHalt := tickBook(t, bar, []*commonv1.InvestigationReport{
		articleAt(time.Date(2026, 9, 11, 14, 0, 0, 0, marketclock.Location()), true, -0.9),
	})
	if sessionHalt.Fills != 0 {
		t.Fatalf("a 14:00 stand-aside must be able to halt fills, got %d (else the 16:00 test is vacuous)", sessionHalt.Fills)
	}
	leaked := tickBook(t, bar, []*commonv1.InvestigationReport{
		lateArticle(true, -0.9),
	})
	if leaked.Fills != base.Fills {
		t.Fatalf("Friday 16:00 article moved Friday 15:25 fills: base=%d leaked=%d", base.Fills, leaked.Fills)
	}
	induced := tickBook(t, bar, []*commonv1.InvestigationReport{
		lateArticle(false, 0.9),
	})
	if induced.Fills != base.Fills {
		t.Fatalf("Friday 16:00 bullish article moved Friday 15:25 fills: base=%d induced=%d", base.Fills, induced.Fills)
	}
}

func TestFillPointsAtExactInvestigation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INVESTIGATION_DIR", dir)
	invID := "inv-exact-tcs"
	if err := research.SaveReceipt(dir, research.Receipt{
		ID: invID, Symbol: "TCS", Prompt: "sys-user", System: "research desk",
		Output: "lean long vs Nifty", Model: "stub-model", Provider: "stub",
		TotalTokens: 7, Mode: "llm",
	}); err != nil {
		t.Fatal(err)
	}
	now := friday1525()
	svc := New(Deps{
		MarketData: mdStub{}, Learning: lnStub{},
		Sentiment: &snStub{reports: []*commonv1.InvestigationReport{{
			Id: invID, Headline: "TCS wins mandate", Symbols: []string{"TCS"},
			Score: 0.8, Confidence: 0.9, EventType: "product", Stance: "bullish",
			Sources: []*commonv1.NewsItem{{PublishedAtUnixMs: now.UnixMilli(), Symbols: []string{"TCS"}}},
		}}},
		Now: func() time.Time { return now },
	})
	svc.inv = research.NewDesk(dir)
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Tick(ctx, &tradingv1.TickRequest{}); err != nil {
		t.Fatal(err)
	}
	port, err := svc.GetPortfolio(ctx, &tradingv1.GetPortfolioRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(port.OpenTrades) == 0 {
		t.Fatal("expected fills")
	}
	var tcs *commonv1.PaperTrade
	for _, tr := range port.OpenTrades {
		if len(tr.InvestigationIds) != 1 || tr.InvestigationIds[0] == "" {
			t.Fatalf("%s fill must point at exactly one investigation, got %v", tr.Symbol, tr.InvestigationIds)
		}
		if tr.Symbol == "TCS" {
			tcs = tr
		}
		b, err := os.ReadFile(filepath.Join(dir, "fills", tr.Id+".json"))
		if err != nil {
			t.Fatalf("receipt next to fill %s: %v", tr.Id, err)
		}
		if !strings.Contains(string(b), tr.InvestigationIds[0]) {
			t.Fatalf("fill %s receipt missing id", tr.Id)
		}
	}
	if tcs == nil {
		t.Fatal("expected a TCS fill")
	}
	if tcs.InvestigationIds[0] != invID {
		t.Fatalf("TCS fill %v want %s", tcs.InvestigationIds, invID)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "fills", tcs.Id+".json"))
	got := string(b)
	for _, want := range []string{"sys-user", "stub-model", `"totalTokens": 7`, "lean long vs Nifty"} {
		if !strings.Contains(got, want) {
			t.Fatalf("TCS fill receipt missing %s in %s", want, got)
		}
	}
}

func TestExecute16PctDrawdownRefusesBuysLogsOnce(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	now := friday1525()
	svc := New(Deps{
		MarketData: mdStub{}, Learning: lnStub{}, Sentiment: &snStub{},
		Log: log, Now: func() time.Time { return now },
	})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.cash = 840_000
	svc.peak = 1_000_000
	svc.eq0 = 1_000_000
	svc.pos = map[string]*commonv1.Position{}
	svc.mu.Unlock()
	r1, err := svc.Tick(ctx, &tradingv1.TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Fills != 0 {
		t.Fatalf("−16%% path must refuse new buys, got %d fills", r1.Fills)
	}
	r2, err := svc.Tick(ctx, &tradingv1.TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Fills != 0 {
		t.Fatalf("second tick still halted, got %d", r2.Fills)
	}
	if n := strings.Count(buf.String(), broker.ReasonMaxDrawdown); n != 1 {
		t.Fatalf("MAX_DRAWDOWN once, got %d in %s", n, buf.String())
	}
}
