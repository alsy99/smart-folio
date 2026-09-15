package trading

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/learning"
	"aperture/pkg/backtest"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

// rosterWeights publishes a data/roster snapshot the way learning does:
// roster ids share weight 1, failing ids sit at 0 with regime failing-gate.
type rosterWeights struct {
	lnStub
	snap backtest.RosterSnapshot
}

func (r rosterWeights) GetWeights(context.Context, *learningv1.GetWeightsRequest, ...grpc.CallOption) (*learningv1.GetWeightsResponse, error) {
	var w []*commonv1.StrategyWeight
	for _, id := range r.snap.Roster {
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 1 / float64(len(r.snap.Roster))})
	}
	for _, id := range r.snap.Failing {
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 0, Regime: learning.RegimeFailing})
	}
	return &learningv1.GetWeightsResponse{Weights: w}, nil
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.Abs(".")
	for i := 0; i < 6; i++ {
		if _, err := filepath.Glob(filepath.Join(dir, "go.mod")); err == nil {
			if m, _ := filepath.Glob(filepath.Join(dir, "go.mod")); len(m) == 1 {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}

// TestCheckedInRosterGivesZeroSatelliteFills replays a session on the
// shipped data/roster snapshot: roster [] and six failing defaults. The
// satellite must not fill; with an IPS the core builds regardless.
func loadShippedRoster(t *testing.T) (backtest.RosterSnapshot, string, error) {
	t.Helper()
	return backtest.LoadLatestRoster(filepath.Join(repoRoot(t), "data", "roster"))
}

func TestCheckedInRosterGivesZeroSatelliteFills(t *testing.T) {
	snap, file, err := loadShippedRoster(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Roster) != 0 || len(snap.Failing) == 0 {
		t.Fatalf("%s: this test assumes the shipped roster is empty and failing is not: %+v", file, snap)
	}
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	now := time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())
	bull := lateArticle(false, 0.9)
	bull.Sources[0].PublishedAtUnixMs = now.Add(-2 * time.Hour).UnixMilli()
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	svc := New(Deps{MarketData: mdStub{}, Learning: rosterWeights{snap: snap}, Sentiment: &snStub{reports: []*commonv1.InvestigationReport{bull}}, Now: func() time.Time { return now }, IPS: &p})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Tick(ctx, &tradingv1.TickRequest{}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.lastSatFills != 0 {
		t.Fatalf("satellite filled %d names on an empty roster", svc.lastSatFills)
	}
	if svc.lastCoreFills == 0 {
		t.Fatal("core must build even though the satellite is empty")
	}
	if len(svc.open) != 0 {
		t.Fatal("no satellite trades may be open")
	}
	for id, w := range svc.plan.weights {
		if w != 0 {
			t.Fatalf("%s carries weight %v on a failing roster", id, w)
		}
	}
	if !svc.lastCorePlan.Session || svc.lastCorePlan.CoreWanted != 1.0 {
		t.Fatalf("empty satellite folds into core under FoldSatellite: wanted %.2f", svc.lastCorePlan.CoreWanted)
	}
}

// admitted is learning with one fake method through the gate.
type admitted struct{ lnStub }

func (admitted) GetWeights(context.Context, *learningv1.GetWeightsRequest, ...grpc.CallOption) (*learningv1.GetWeightsResponse, error) {
	var w []*commonv1.StrategyWeight
	for _, id := range strategies.IDs() {
		if id == strategies.Momentum1d {
			w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 1})
			continue
		}
		w = append(w, &commonv1.StrategyWeight{StrategyId: id, Weight: 0, Regime: learning.RegimeFailing})
	}
	return &learningv1.GetWeightsResponse{Weights: w}, nil
}

// TestFakeAdmittedMethodIsCappedAtTheSatelliteSlice: every name votes long
// at full score for several sessions while the core sits idle between
// rebalance sessions (a flat core booked as "worked" on 14 Sep; month-end
// is the 30th). Without the IPS cap the satellite would add 10%/session
// toward the gross cap; with an 80/20 IPS it stops at 20% of equity.
func TestFakeAdmittedMethodIsCappedAtTheSatelliteSlice(t *testing.T) {
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	clk := &testClock{t: time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())}
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	svc := New(Deps{MarketData: mdStub{}, Learning: admitted{}, Sentiment: &snStub{}, Now: clk.Now, IPS: &p})
	ctx := context.Background()
	if _, err := svc.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.coreLastRebal = "2026-09-14"
	svc.mu.Unlock()
	satFills := 0
	for i := 0; i < 8; i++ {
		if err := svc.Plan(ctx); err != nil {
			t.Fatal(err)
		}
		svc.mu.Lock()
		for _, sym := range universe.EquitySymbols() {
			svc.plan.signals[sym] = strategies.Signal{Symbol: sym, StrategyID: strategies.Momentum1d, Direction: 1, Score: 1}
		}
		svc.mu.Unlock()
		if _, err := svc.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		svc.mu.Lock()
		satFills += svc.lastSatFills
		snap := svc.snapshotLocked()
		last := map[string]float64{}
		for sym := range snap.Held {
			last[sym] = 1000
		}
		sat := snap.Gross - svc.coreValueLocked(last)
		svc.mu.Unlock()
		if sat > snap.Equity*p.SatellitePct+1 {
			t.Fatalf("session %d: satellite %.0f exceeds %.0f%% of equity %.0f", i, sat, p.SatellitePct*100, snap.Equity)
		}
		clk.Set(clk.Now().AddDate(0, 0, 1))
		for clk.Now().Weekday() == time.Saturday || clk.Now().Weekday() == time.Sunday {
			clk.Set(clk.Now().AddDate(0, 0, 1))
		}
	}
	if satFills == 0 {
		t.Fatal("an admitted method must be allowed to fill inside the cap, else the cap test is vacuous")
	}
	svc.mu.Lock()
	snap := svc.snapshotLocked()
	if snap.Gross < snap.Equity*0.15 {
		t.Fatalf("satellite only reached %.1f%%: the rails, not the cap, bounded it", 100*snap.Gross/snap.Equity)
	}
	for _, tr := range svc.open {
		if tr.StrategyId != strategies.Momentum1d {
			t.Fatalf("a failing id %s filled", tr.StrategyId)
		}
	}
	svc.mu.Unlock()
	// Core 100%: SatellitePct 0 means no satellite fill, ever.
	q := ips.Default("c-2", costs.StartCash)
	svc2 := New(Deps{MarketData: mdStub{}, Learning: admitted{}, Sentiment: &snStub{}, Now: clk.Now, IPS: &q})
	if _, err := svc2.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	if err := svc2.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	svc2.mu.Lock()
	for _, sym := range universe.EquitySymbols() {
		svc2.plan.signals[sym] = strategies.Signal{Symbol: sym, StrategyID: strategies.Momentum1d, Direction: 1, Score: 1}
	}
	svc2.mu.Unlock()
	if _, err := svc2.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	svc2.mu.Lock()
	defer svc2.mu.Unlock()
	if svc2.lastSatFills != 0 || len(svc2.open) != 0 {
		t.Fatalf("100%% core admits no satellite fills, got %d", svc2.lastSatFills)
	}
}

// TestBestSignalNeverReturnsAFailingID: only ids with weight > 0 may
// produce a signal; an all-zero map yields no signal at all.
func TestBestSignalNeverReturnsAFailingID(t *testing.T) {
	svc := New(Deps{MarketData: mdStub{}, Learning: lnStub{}, Sentiment: &snStub{}})
	weights := map[string]float64{}
	for _, id := range strategies.IDs() {
		weights[id] = 0
	}
	weights[strategies.Momentum1d] = 1
	for _, sym := range universe.EquitySymbols() {
		sig := svc.bestSignal(context.Background(), sym, 0, weights)
		if sig.StrategyID != "" && sig.StrategyID != strategies.Momentum1d {
			t.Fatalf("%s: failing id %s produced a signal", sym, sig.StrategyID)
		}
	}
	for id := range weights {
		weights[id] = 0
	}
	for _, sym := range universe.EquitySymbols() {
		if sig := svc.bestSignal(context.Background(), sym, 0.9, weights); sig.StrategyID != "" || sig.Direction != 0 {
			t.Fatalf("%s: all-zero weights must yield no signal, got %+v", sym, sig)
		}
	}
	// A failing id keeps weight 0 after any tilt; the tilt path is in Plan.
	if strings.TrimSpace(learning.RegimeFailing) != "failing-gate" {
		t.Fatal("regime label drifted")
	}
}
