package trading

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/learn"
	"aperture/pkg/live"
	"aperture/pkg/marketclock"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

func TestDefaultRosterHoldsASession(t *testing.T) {
	for _, spec := range strategies.DefaultSpecs() {
		if strategies.HoldsUnderSession(spec.Timeframe) {
			t.Fatalf("%s timeframe %s holds under a session", spec.ID, spec.Timeframe)
		}
	}
	for _, id := range strategies.IDs() {
		if strategies.HoldsUnderSession(strategies.Parse(id).Timeframe) {
			t.Fatalf("shipped id %s", id)
		}
	}
}

func TestPositionalExitNeedsMinHoldSessions(t *testing.T) {
	min := costs.MinHoldSessions
	if shouldExit(false, 45*time.Second, 0, 0.05, 1) {
		t.Fatal("must not close on the 45s scalp path")
	}
	if shouldExit(false, costs.SessionHold-time.Second, 0, -0.02, 0) {
		t.Fatal("must hold through the session even if the signal flipped")
	}
	if shouldExit(false, 24*time.Hour, min-1, -0.02, 0) {
		t.Fatalf("a flip after %d sessions must not close a positional name (min %d)", min-1, min)
	}
	if !shouldExit(false, 72*time.Hour, min, 0.01, 0) {
		t.Fatal("after the min hold, flat/short signal should exit")
	}
	if shouldExit(false, 72*time.Hour, min, 0.01, 1) {
		t.Fatal("still long after the min hold — keep the name")
	}
	if !shouldExit(false, costs.MaxHold, 0, 0.01, 1) {
		t.Fatal("weeks-long cap should recycle the name regardless of sessions")
	}
}

func TestScalpExitPath(t *testing.T) {
	if !shouldExit(true, costs.ScalpMaxHold+time.Second, 0, 0, 1) {
		t.Fatal("45s")
	}
	if !shouldExit(true, time.Second, 0, 0.013, 1) {
		t.Fatal("take")
	}
	if !shouldExit(true, time.Second, 0, -0.009, 1) {
		t.Fatal("stop")
	}
	if shouldExit(true, time.Second, 0, 0.001, 1) {
		t.Fatal("hold")
	}
}

// zeroWeights is a learning stub whose real-tape gate failed every default:
// each id is published at weight 0.
type zeroWeights struct{ lnStub }

func (zeroWeights) GetWeights(context.Context, *learningv1.GetWeightsRequest, ...grpc.CallOption) (*learningv1.GetWeightsResponse, error) {
	var w []*commonv1.StrategyWeight
	for _, id := range strategies.IDs() {
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 0, Regime: "failing-gate"})
	}
	return &learningv1.GetWeightsResponse{Weights: w}, nil
}

// TestZeroWeightDefaultsNeverFill: failing defaults carry weight 0 and the
// book must hold cash — no equal-weight fallback, no news tilt readmission.
func TestZeroWeightDefaultsNeverFill(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	now := friday1525()
	bull := lateArticle(false, 0.9)
	bull.Sources[0].PublishedAtUnixMs = now.Add(-2 * time.Hour).UnixMilli() // in-session, would tilt
	svc := New(Deps{MarketData: mdStub{}, Learning: zeroWeights{}, Sentiment: &snStub{reports: []*commonv1.InvestigationReport{bull}}, Now: func() time.Time { return now }})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.Tick(ctx, &tradingv1.TickRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Fills != 0 {
		t.Fatalf("all defaults gated out at weight 0, yet %d fills", resp.Fills)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for id, w := range svc.plan.weights {
		if w != 0 {
			t.Fatalf("%s resurrected to weight %v", id, w)
		}
	}
	for sym, sig := range svc.plan.signals {
		if sig.Direction != 0 {
			t.Fatalf("%s got a signal from a zero-weight method %s", sym, sig.StrategyID)
		}
	}
	// Control: the same tape with equal weights does fill.
	ctl := tickBook(t, now, nil)
	if ctl.Fills == 0 {
		t.Fatal("control fixture must fill so the zero-weight test is not vacuous")
	}
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func newReplaySvc(t *testing.T) (*Service, *testClock) {
	t.Helper()
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	clk := &testClock{t: time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())} // Monday
	svc := New(Deps{MarketData: mdStub{}, Learning: lnStub{}, Sentiment: &snStub{}, Now: clk.Now})
	return svc, clk
}

// TestExecuteHonoursTurnoverCap drives Execute on the deterministic tape with
// every strategy voting long at full score: without the rail the book would
// fill up to the gross cap on day one; with it, session buys stay under
// costs.TurnoverCapDay of equity and a second Execute in the same session
// adds nothing.
func TestExecuteHonoursTurnoverCap(t *testing.T) {
	svc, clk := newReplaySvc(t)
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	plan := bookPlan{signals: map[string]strategies.Signal{}, weights: map[string]float64{}, standAside: map[string]bool{}, invID: map[string]string{}}
	for _, sym := range universe.EquitySymbols() {
		plan.signals[sym] = strategies.Signal{Symbol: sym, StrategyID: strategies.Momentum1d, Direction: 1, Score: 1}
	}
	plan.weights[strategies.Momentum1d] = 1
	svc.plan = plan
	svc.mu.Unlock()

	resp, err := svc.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Fills == 0 {
		t.Fatal("expected at least one fill with every name voting long")
	}
	svc.mu.Lock()
	buys, eq := svc.dayBuys, svc.markLocked()
	svc.mu.Unlock()
	if buys > eq*costs.TurnoverCapDay+1 {
		t.Fatalf("session buys %.0f exceed cap %.0f (%.0f%% of equity %.0f)", buys, eq*costs.TurnoverCapDay, costs.TurnoverCapDay*100, eq)
	}
	if buys < costs.MinNameNotional {
		t.Fatalf("cap should still admit at least one ticket, got %.0f", buys)
	}

	clk.Set(clk.Now().Add(2 * time.Minute)) // same session
	again, err := svc.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Fills != 0 {
		t.Fatalf("second Execute in the same session filled %d more names past the cap", again.Fills)
	}

	// Next session: the rail resets, more names may be admitted.
	clk.Set(clk.Now().Add(24 * time.Hour))
	next, err := svc.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if next.Fills == 0 {
		t.Fatal("turnover rail must reset on a new IST session")
	}
	// ...and nothing closed: every name is inside the min hold.
	if next.Closes != 0 {
		t.Fatalf("names closed after 1 session, min hold is %d", costs.MinHoldSessions)
	}
}

func TestNameCapAndDrawdown(t *testing.T) {
	if got := costs.NameRoom(1_000_000, 0); got != 80_000 {
		t.Fatalf("room %v", got)
	}
	if got := costs.NameRoom(1_000_000, 80_000); got != 0 {
		t.Fatalf("full name %v", got)
	}
	if costs.BookHalted(900_000, 1_000_000) {
		t.Fatal("10% is not a halt")
	}
	if !costs.BookHalted(850_000, 1_000_000) {
		t.Fatal("15% book drawdown should halt")
	}
}

func TestCashSliceIsNotAQuarterOfCash(t *testing.T) {
	s := broker.Snapshot{Equity: 1_000_000, Peak: 1_000_000, Cash: 1_000_000, Held: map[string]float64{}, Sector: map[string]float64{}}
	if broker.Room(s, "TCS") > s.Cash*0.10 {
		t.Fatal("a 10-name book cannot put more than ~8-10% of cash in one ticket")
	}
}

func TestClosePnLIsAfterCosts(t *testing.T) {
	gross := afterCostPnL(100, 110, 10)
	if gross >= 100 {
		t.Fatalf("delivery charges must come out of the ₹100 gross, got %v", gross)
	}
	if afterCostPnL(100, 100, 10) >= 0 {
		t.Fatal("flat round-trip after costs must be a loss")
	}
}

func TestExcursionTracksMAEAndMFE(t *testing.T) {
	e := &excursion{}
	updateExcursion(e, "BUY", 100, 97)
	updateExcursion(e, "BUY", 100, 104)
	updateExcursion(e, "BUY", 100, 101)
	if e.mae != -0.03 {
		t.Fatalf("mae %v", e.mae)
	}
	if e.mfe != 0.04 {
		t.Fatalf("mfe %v", e.mfe)
	}
}

func TestCloseStoresLearnFacts(t *testing.T) {
	s := New(Deps{Now: func() time.Time { return time.Date(2026, 9, 15, 15, 30, 0, 0, time.UTC) }})
	tr := &commonv1.PaperTrade{
		Id: "t-1", Symbol: "TCS", Side: "BUY", StrategyId: strategies.Momentum1d,
		Qty: 10, Entry: 100, Open: true, OpenedAtUnixMs: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC).UnixMilli(),
	}
	s.open = []*commonv1.PaperTrade{tr}
	s.markExcursionLocked(tr, 96)
	s.markExcursionLocked(tr, 105)
	s.closeLocked(tr, 102, 0.012, 0)
	if tr.Pnl >= (102-100)*10 {
		t.Fatalf("pnl after costs %v", tr.Pnl)
	}
	mae, mfe, regime, hold := learn.ParseFacts(tr.AttributionTags)
	if mae >= 0 || mfe <= 0 {
		t.Fatalf("mae/mfe %v %v tags %v", mae, mfe, tr.AttributionTags)
	}
	if regime != "bull" {
		t.Fatalf("regime %s", regime)
	}
	if hold <= 0 {
		t.Fatal("hold")
	}
}

func TestStopAutopilotTripsHumanKillSwitch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "KILL")
	t.Setenv("KILL_FILE", p)
	s := New(Deps{})
	if _, err := s.SetAutopilot(context.Background(), &tradingv1.SetAutopilotRequest{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if !live.Killed() {
		t.Fatal("Stop Autopilot must drop the kill file a human can also touch")
	}
	if _, err := s.SetAutopilot(context.Background(), &tradingv1.SetAutopilotRequest{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if live.Killed() {
		t.Fatal("Resume Autopilot clears the file for the paper book")
	}
}
