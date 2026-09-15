package campaign

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/trading"
	"aperture/pkg/backtest"
	pub "aperture/pkg/campaign"
	"aperture/pkg/excess"
	"aperture/pkg/marketclock"

	"google.golang.org/grpc"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// rosterWeights is the frozen learning client: equal weight across the
// roster that cleared the gate as-of the window start, weight 0 for the
// defaults that failed it. Nothing moves for 30 days.
type rosterWeights struct {
	snap backtest.RosterSnapshot
}

func (rosterWeights) RecordTrade(context.Context, *learningv1.RecordTradeRequest, ...grpc.CallOption) (*learningv1.RecordTradeResponse, error) {
	return &learningv1.RecordTradeResponse{}, nil
}
func (rosterWeights) ListJournal(context.Context, *learningv1.ListJournalRequest, ...grpc.CallOption) (*learningv1.ListJournalResponse, error) {
	return &learningv1.ListJournalResponse{}, nil
}
func (r rosterWeights) GetWeights(context.Context, *learningv1.GetWeightsRequest, ...grpc.CallOption) (*learningv1.GetWeightsResponse, error) {
	var w []*commonv1.StrategyWeight
	if n := len(r.snap.Roster); n > 0 {
		eq := 1.0 / float64(n)
		for _, id := range r.snap.Roster {
			w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: eq})
		}
	}
	for _, id := range r.snap.Failing {
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 0, Regime: "failing-gate"})
	}
	// Always non-nil: an all-zero book is a verdict, not "learning is down".
	if w == nil {
		w = []*commonv1.StrategyWeight{}
	}
	return &learningv1.GetWeightsResponse{Weights: w}, nil
}
func (rosterWeights) RunBacktest(context.Context, *learningv1.RunBacktestRequest, ...grpc.CallOption) (*learningv1.BacktestReport, error) {
	return &learningv1.BacktestReport{}, nil
}
func (rosterWeights) GetBacktest(context.Context, *learningv1.GetBacktestRequest, ...grpc.CallOption) (*learningv1.BacktestReport, error) {
	return &learningv1.BacktestReport{}, nil
}

type silentNews struct{}

func (silentNews) ScoreSymbols(context.Context, *sentimentv1.ScoreSymbolsRequest, ...grpc.CallOption) (*sentimentv1.ScoreSymbolsResponse, error) {
	return &sentimentv1.ScoreSymbolsResponse{}, nil
}
func (silentNews) ListNews(context.Context, *sentimentv1.ListNewsRequest, ...grpc.CallOption) (*sentimentv1.ListNewsResponse, error) {
	return &sentimentv1.ListNewsResponse{}, nil
}
func (silentNews) GetInvestigations(context.Context, *sentimentv1.GetInvestigationsRequest, ...grpc.CallOption) (*sentimentv1.GetInvestigationsResponse, error) {
	return &sentimentv1.GetInvestigationsResponse{}, nil
}

func pinEnv() func() {
	keys := []string{"SCALP_MODE", "INVESTIGATION_LLM", "CAMPAIGN_REPLAY", "MARKET_CLOCK_OVERRIDE", "INVESTIGATION_DIR"}
	prev := map[string]string{}
	had := map[string]bool{}
	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		prev[k], had[k] = v, ok
	}
	_ = os.Setenv("SCALP_MODE", "false")
	_ = os.Setenv("INVESTIGATION_LLM", "false")
	_ = os.Setenv("CAMPAIGN_REPLAY", "1")
	_ = os.Unsetenv("MARKET_CLOCK_OVERRIDE")
	_ = os.Setenv("INVESTIGATION_DIR", os.TempDir())
	return func() {
		for _, k := range keys {
			if had[k] {
				_ = os.Setenv(k, prev[k])
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

func excessPct(portRet, start, last float64) float64 {
	bench := 0.0
	if start > 0 && last > 0 {
		bench = last/start - 1
	}
	return pub.PP(excess.Excess(portRet, bench) * 100)
}

func drawdownPct(equity, peak float64) float64 {
	if peak <= 0 {
		return 0
	}
	return pub.PP((peak - equity) / peak * 100)
}

func turnoverPct(notional, equity float64) float64 {
	if equity <= 0 {
		return 0
	}
	return pub.PP(notional / equity * 100)
}

// Replay runs the frozen 30-day paper book on the checked-in INDstocks daily
// tape with the as-of roster: one MOC tick per session at 15:30 IST.
func Replay() (*pub.Ledger, error) {
	return ReplayDir("")
}

// ReplayDir replays the campaign whose bars.json and roster.json live in dir.
func ReplayDir(dir string) (*pub.Ledger, error) {
	undo := pinEnv()
	defer undo()

	in, err := pub.LoadInputs(dir)
	if err != nil {
		return nil, err
	}
	start, end := pub.Window()
	clk := &clock{t: start}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := trading.New(trading.Deps{
		MarketData: pub.BarTape{Bars: in.Bars, Now: clk.Now},
		Learning:   rosterWeights{snap: in.Roster},
		Sentiment:  silentNews{},
		Log:        log,
		Now:        clk.Now,
		Cfg:        trading.Config{CampaignDays: pub.Days},
	})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: int32(pub.Days)}); err != nil {
		return nil, err
	}

	sha, dirty := pub.GitSHA()
	led := &pub.Ledger{Manifest: pub.NewManifest(sha, dirty, in)}
	loc := marketclock.Location()

	for i := 0; i < pub.Days; i++ {
		day := start.AddDate(0, 0, i)
		y, m, d := day.Date()
		closeT := time.Date(y, m, d, 15, 30, 0, 0, loc)
		if closeT.After(end) {
			closeT = end
		}
		svc.ResetTickStats()
		status := "weekend"
		if marketclock.IsOpen(closeT) && hasSession(in.Bars, closeT) {
			status = "open"
			clk.Set(closeT)
			if _, err := svc.Tick(ctx, &tradingv1.TickRequest{}); err != nil {
				return nil, err
			}
		} else if marketclock.IsOpen(closeT) {
			status = "holiday"
		}
		clk.Set(closeT)
		pr, err := svc.Print(ctx)
		if err != nil {
			return nil, err
		}
		portRet := excess.PortfolioReturn(pr.Equity, led.Manifest.StartCash)
		led.Days = append(led.Days, pub.Day{
			Date:              closeT.Format("2006-01-02"),
			Session:           status,
			Equity:            pub.INR(pr.Equity),
			ExcessNifty50Pct:  excessPct(portRet, pr.BenchStart["NIFTY50"], pr.Last["NIFTY50"]),
			ExcessNifty500Pct: excessPct(portRet, pr.BenchStart["NIFTY500"], pr.Last["NIFTY500"]),
			ExcessSensexPct:   excessPct(portRet, pr.BenchStart["SENSEX"], pr.Last["SENSEX"]),
			DrawdownPct:       drawdownPct(pr.Equity, pr.Peak),
			TurnoverPct:       turnoverPct(pr.Turnover, pr.Equity),
			TurnoverINR:       pub.INR(pr.Turnover),
			Fills:             pr.Fills,
			Halted:            pr.Halted,
		})
	}
	return led, nil
}

// hasSession is true when NIFTY50 printed a close on that IST date — the
// exchange calendar, so holidays fall out of the tape rather than a list.
func hasSession(bars *pub.Bars, t time.Time) bool {
	if bars == nil {
		return false
	}
	d := marketclock.SessionDate(t)
	for _, b := range bars.Series["NIFTY50"] {
		if b.Date == d {
			return true
		}
	}
	return false
}
