package trading

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/llm"
	"aperture/pkg/marketclock"
	"aperture/pkg/strategies"
	"aperture/pkg/tilt"
	"aperture/pkg/universe"
)

// llmArticle is a report whose thesis came from the LLM; mode "heuristic"
// is the same headline with the LLM off or over budget.
func llmArticle(when time.Time, score float64, mode string) *commonv1.InvestigationReport {
	r := articleAt(when, false, score)
	r.ResearchMode = mode
	r.StrategyImplications = nil // isolate the sleeve tilt from method weights
	return r
}

// ledger is the book's state in one comparable string: cash, every
// position, and the core sleeve's quantities.
func ledger(svc *Service) string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "cash=%.4f\n", svc.cash)
	syms := make([]string, 0, len(svc.pos))
	for sym := range svc.pos {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	for _, sym := range syms {
		p := svc.pos[sym]
		fmt.Fprintf(&b, "%s qty=%.0f avg=%.4f core=%.0f\n", sym, p.Qty, p.AvgPrice, svc.coreQty[sym])
	}
	return b.String()
}

func coreOnly(svc *Service) map[string]float64 {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	out := map[string]float64{}
	for k, v := range svc.coreQty {
		out[k] = v
	}
	return out
}

func satelliteNotional(svc *Service) (sat, equity float64) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	snap := svc.snapshotLocked()
	last := map[string]float64{}
	for sym := range snap.Held {
		last[sym] = 1000
	}
	return snap.Gross - svc.coreValueLocked(last), snap.Equity
}

type replayOpts struct {
	learning learningv1.LearningServiceClient
	reports  []*commonv1.InvestigationReport
	coreIdle bool
	log      *slog.Logger
}

func replay(t *testing.T, sessions int, o replayOpts) *Service {
	t.Helper()
	clk := &testClock{t: time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())}
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	svc := New(Deps{MarketData: mdStub{}, Learning: o.learning, Sentiment: &snStub{reports: o.reports}, Now: clk.Now, IPS: &p, Log: o.log})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	if o.coreIdle {
		svc.mu.Lock()
		svc.coreLastRebal = "2026-09-14"
		svc.mu.Unlock()
	}
	for i := 0; i < sessions; i++ {
		if err := svc.Plan(ctx); err != nil {
			t.Fatal(err)
		}
		if o.coreIdle {
			svc.mu.Lock()
			for _, sym := range universe.EquitySymbols() {
				svc.plan.signals[sym] = strategies.Signal{Symbol: sym, StrategyID: strategies.Momentum1d, Direction: 1, Score: 1}
			}
			svc.mu.Unlock()
		}
		if _, err := svc.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Set(clk.Now().AddDate(0, 0, 1))
		for clk.Now().Weekday() == time.Saturday || clk.Now().Weekday() == time.Sunday {
			clk.Set(clk.Now().AddDate(0, 0, 1))
		}
	}
	return svc
}

// TestFutureHeadlineDoesNotTiltThisBar: a bearish LLM report published
// after the bar is not known at the bar (pkg/asof) and leaves the tilt at
// what the known report says.
func TestFutureHeadlineDoesNotTiltThisBar(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	now := time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())
	known := llmArticle(now.Add(-2*time.Hour), -0.10, "llm")
	future := llmArticle(now.Add(30*time.Minute), -0.90, "llm")
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	for _, reports := range [][]*commonv1.InvestigationReport{{known}, {known, future}} {
		svc := New(Deps{MarketData: mdStub{}, Learning: admitted{}, Sentiment: &snStub{reports: reports}, Now: func() time.Time { return now }, IPS: &p})
		if err := svc.Plan(context.Background()); err != nil {
			t.Fatal(err)
		}
		svc.mu.Lock()
		got := svc.plan.satScore
		svc.mu.Unlock()
		if math.Abs(got-(-0.10)) > 1e-9 {
			t.Fatalf("%d reports: tilt score %v, want -0.10 from the known report only", len(reports), got)
		}
	}
	// And the future headline, once it is the past, does count.
	later := now.Add(2 * time.Hour)
	svc := New(Deps{MarketData: mdStub{}, Learning: admitted{}, Sentiment: &snStub{reports: []*commonv1.InvestigationReport{future}}, Now: func() time.Time { return later }, IPS: &p})
	if err := svc.Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.plan.satScore != -0.90 {
		t.Fatalf("next bar should see the headline: %v", svc.plan.satScore)
	}
}

// TestLLMBudgetExhaustedZeroesTiltNotCore: with the IST-day call cap spent,
// Plan logs LLM_BUDGET once, the tilt is 0 even though an LLM-mode report is
// known, and the core still builds.
func TestLLMBudgetExhaustedZeroesTiltNotCore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INVESTIGATION_DIR", dir)
	now := time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())
	day := llm.ISTDay(now)
	if err := llm.SaveDay(dir, llm.DayState{Day: day, Calls: llm.MaxCalls(), Tokens: 10}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	bear := llmArticle(now.Add(-2*time.Hour), -0.90, "llm")
	svc := New(Deps{MarketData: mdStub{}, Learning: admitted{}, Sentiment: &snStub{reports: []*commonv1.InvestigationReport{bear}}, Now: func() time.Time { return now }, IPS: &p, Log: logger})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := svc.Plan(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	score, coreFills := svc.plan.satScore, svc.lastCoreFills
	svc.mu.Unlock()
	if score != 0 {
		t.Fatalf("tilt must be 0 with the budget spent, got %v", score)
	}
	if n := strings.Count(buf.String(), LogLLMBudget); n != 1 {
		t.Fatalf("want exactly one %s log for the day, got %d:\n%s", LogLLMBudget, n, buf.String())
	}
	if coreFills == 0 {
		t.Fatal("core must build regardless of the LLM budget")
	}
	if tilt.Effective(p.SatellitePct, p.SatellitePct, score) != p.SatellitePct {
		t.Fatal("zero score must leave the full slice")
	}
}

// TestLLMOnOffIdenticalLedgersOnEmptyRoster: the shipped roster admits no
// method, so LLM sentiment has nothing to tilt. Two six-session replays,
// one with bearish LLM reports and one with the same headlines heuristic
// only, end with byte-identical books.
func TestLLMOnOffIdenticalLedgersOnEmptyRoster(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	snap, _, err := loadShippedRoster(t)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 14, 13, 0, 0, 0, marketclock.Location())
	on := replay(t, 6, replayOpts{learning: rosterWeights{snap: snap}, reports: []*commonv1.InvestigationReport{llmArticle(at, -0.9, "llm")}})
	off := replay(t, 6, replayOpts{learning: rosterWeights{snap: snap}, reports: []*commonv1.InvestigationReport{llmArticle(at, -0.9, "heuristic")}})
	if a, b := ledger(on), ledger(off); a != b {
		t.Fatalf("ledgers differ with an empty roster:\n%s\n---\n%s", a, b)
	}
	if !strings.Contains(ledger(on), "core=") || len(coreOnly(on)) == 0 {
		t.Fatal("core must have built in both replays")
	}
}

// TestLLMOnOffCoreIdenticalSatelliteWithinTilt: with a fake admitted method
// the LLM may shrink the satellite by at most 25% of the sleeve; the core
// holdings are identical in both replays.
func TestLLMOnOffCoreIdenticalSatelliteWithinTilt(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	at := time.Date(2026, 9, 14, 13, 0, 0, 0, marketclock.Location())
	on := replay(t, 4, replayOpts{learning: admitted{}, coreIdle: true, reports: []*commonv1.InvestigationReport{llmArticle(at, -0.9, "llm")}})
	off := replay(t, 4, replayOpts{learning: admitted{}, coreIdle: true, reports: []*commonv1.InvestigationReport{llmArticle(at, -0.9, "heuristic")}})
	if a, b := fmt.Sprint(coreOnly(on)), fmt.Sprint(coreOnly(off)); a != b {
		t.Fatalf("core holdings differ: %s vs %s", a, b)
	}
	satOn, eq := satelliteNotional(on)
	satOff, _ := satelliteNotional(off)
	sleeve := 0.20 * eq
	if satOff < sleeve*0.9 {
		t.Fatalf("LLM-off satellite only %.0f of a %.0f sleeve: the fixture is not filling the slice", satOff, sleeve)
	}
	if satOn >= satOff {
		t.Fatalf("a bearish LLM tilt must shrink the satellite: on %.0f off %.0f", satOn, satOff)
	}
	if d := satOff - satOn; d > tilt.Cap*sleeve+1 {
		t.Fatalf("satellite differs by %.0f, more than 25%% of the %.0f sleeve", d, sleeve)
	}
}
