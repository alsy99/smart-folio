package trading

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/policy"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/live"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

// mdPrices is mdStub with per-symbol overrides so a test can move one name.
type mdPrices struct {
	mdStub
	mu sync.Mutex
	px map[string]float64
}

func (m *mdPrices) set(sym string, px float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.px == nil {
		m.px = map[string]float64{}
	}
	m.px[sym] = px
}

func (m *mdPrices) GetQuotes(ctx context.Context, req *marketdatav1.GetQuotesRequest, opts ...grpc.CallOption) (*marketdatav1.GetQuotesResponse, error) {
	resp, err := m.mdStub.GetQuotes(ctx, req, opts...)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, q := range resp.Quotes {
		if px, ok := m.px[q.Symbol]; ok {
			q.Last = px
		}
	}
	return resp, nil
}

// snDown: the sentiment/LLM side is unreachable.
type snDown struct{}

func (snDown) GetInvestigations(context.Context, *sentimentv1.GetInvestigationsRequest, ...grpc.CallOption) (*sentimentv1.GetInvestigationsResponse, error) {
	return nil, errors.New("sentiment down")
}
func (snDown) ScoreSymbols(context.Context, *sentimentv1.ScoreSymbolsRequest, ...grpc.CallOption) (*sentimentv1.ScoreSymbolsResponse, error) {
	return nil, errors.New("sentiment down")
}
func (snDown) ListNews(context.Context, *sentimentv1.ListNewsRequest, ...grpc.CallOption) (*sentimentv1.ListNewsResponse, error) {
	return nil, errors.New("sentiment down")
}

type coreFixture struct {
	svc *Service
	clk *testClock
	md  *mdPrices
	log *bytes.Buffer
}

// newCoreFixture: empty roster (every default at weight 0), IPS bound,
// Monday 14 Sep 2026 15:25 IST.
func newCoreFixture(t *testing.T, p ips.IPS, sn sentimentv1.SentimentServiceClient) coreFixture {
	t.Helper()
	t.Setenv("INVESTIGATION_DIR", t.TempDir())
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	clk := &testClock{t: time.Date(2026, 9, 14, 15, 25, 0, 0, marketclock.Location())}
	md := &mdPrices{}
	if sn == nil {
		sn = &snStub{}
	}
	svc := New(Deps{MarketData: md, Learning: zeroWeights{}, Sentiment: sn, Log: log, Now: clk.Now, IPS: &p})
	if _, err := svc.StartCampaign(context.Background(), &tradingv1.StartCampaignRequest{Days: 30}); err != nil {
		t.Fatal(err)
	}
	return coreFixture{svc: svc, clk: clk, md: md, log: &buf}
}

func (f coreFixture) tick(t *testing.T) (core, sat int) {
	t.Helper()
	if _, err := f.svc.Tick(context.Background(), &tradingv1.TickRequest{}); err != nil {
		t.Fatal(err)
	}
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	return f.svc.lastCoreFills, f.svc.lastSatFills
}

// nextSession moves the clock to 15:25 IST on the next weekday.
func (f coreFixture) nextSession() {
	n := f.clk.Now().AddDate(0, 0, 1)
	for n.Weekday() == time.Saturday || n.Weekday() == time.Sunday {
		n = n.AddDate(0, 0, 1)
	}
	f.clk.Set(n)
}

func (f coreFixture) coreState() (pending bool, last string, held map[string]float64, cash, eq float64) {
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	held = map[string]float64{}
	for s, q := range f.svc.coreQty {
		held[s] = q
	}
	return f.svc.corePending, f.svc.coreLastRebal, held, f.svc.cash, f.svc.markLocked()
}

// TestCoreBuildsUnderTheRailsThenIdles: with the roster empty and core
// 100%, the book legs into the core at ≤10%/session, ends at 81% invested
// (name and sector caps), and then does nothing on a non-rebalance day.
func TestCoreBuildsUnderTheRailsThenIdles(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	core, sat := f.tick(t)
	if core == 0 || sat != 0 {
		t.Fatalf("day one: core %d satellite %d; the satellite is empty, the core must start", core, sat)
	}
	f.svc.mu.Lock()
	buys, eq := f.svc.dayBuys, f.svc.markLocked()
	f.svc.mu.Unlock()
	if buys > eq*costs.TurnoverCapDay+1 {
		t.Fatalf("core build must respect the 10%% session rail: %.0f of %.0f", buys, eq)
	}
	// Leg in over following sessions until every name is inside the band.
	sessions := 1
	for pending, _, _, _, _ := f.coreState(); pending && sessions < 20; pending, _, _, _, _ = f.coreState() {
		f.nextSession()
		f.tick(t)
		sessions++
	}
	pending, _, held, cash, eq := f.coreState()
	if pending {
		t.Fatalf("core still pending after %d sessions", sessions)
	}
	if sessions < 8 || sessions > 12 {
		t.Logf("build took %d sessions", sessions)
	}
	if len(held) != 12 {
		t.Fatalf("core must hold the 12 names, got %d", len(held))
	}
	invested := (eq - cash) / eq
	if invested < 0.79 || invested > 0.82 {
		t.Fatalf("100%% core trims to ~81%% under the name and sector caps, got %.3f", invested)
	}
	f.svc.mu.Lock()
	for sym, mv := range f.svc.snapshotLocked().Held {
		if mv > eq*costs.NameCap+1 {
			t.Fatalf("%s %.0f breaks the name cap", sym, mv)
		}
	}
	f.svc.mu.Unlock()
	// A non-rebalance day: zero fills, zero turnover.
	f.nextSession()
	if f.clk.Now().Day() >= 28 {
		t.Fatalf("fixture drifted to month end: %s", f.clk.Now())
	}
	core, sat = f.tick(t)
	f.svc.mu.Lock()
	turn := f.svc.lastTurnover
	f.svc.mu.Unlock()
	if core != 0 || sat != 0 || turn != 0 {
		t.Fatalf("non-rebalance day: core %d satellite %d turnover %.0f", core, sat, turn)
	}
	if !strings.Contains(f.log.String(), policy.ReasonCoreInitial) {
		t.Fatal("initial build must be logged as CORE_INITIAL")
	}
	if strings.Contains(f.log.String(), "MAX_DRAWDOWN") {
		t.Fatal("no halt on a flat tape")
	}
}

// TestMonthEndTradesOnlyNamesOutsideTheBand: after the build, one name
// rallies 40%. At the month-end session only that name is sold back to
// target; the rest sit inside 100 bps and do not trade.
func TestMonthEndTradesOnlyNamesOutsideTheBand(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	for i := 0; i < 14; i++ {
		f.tick(t)
		if p, _, _, _, _ := f.coreState(); !p && i > 0 {
			break
		}
		f.nextSession()
	}
	if p, _, _, _, _ := f.coreState(); p {
		t.Fatal("build did not complete")
	}
	// Not a rebalance day: a 40% move is not acted on.
	f.md.set("TCS", 1400)
	f.clk.Set(time.Date(2026, 9, 25, 15, 25, 0, 0, marketclock.Location())) // Friday
	if core, _ := f.tick(t); core != 0 {
		t.Fatalf("drift is only traded on the calendar, got %d fills", core)
	}
	// Wed 30 Sep 2026 is the last September session.
	f.clk.Set(time.Date(2026, 9, 30, 15, 25, 0, 0, marketclock.Location()))
	_, _, before, _, _ := f.coreState()
	core, _ := f.tick(t)
	if core != 1 {
		t.Fatalf("month end must trade exactly the one name outside the band, got %d", core)
	}
	_, last, after, _, _ := f.coreState()
	if last != "2026-09-30" {
		t.Fatalf("last rebalance %s", last)
	}
	for sym := range before {
		if sym == "TCS" {
			if after[sym] >= before[sym] {
				t.Fatal("TCS must be sold down to target")
			}
			continue
		}
		if after[sym] != before[sym] {
			t.Fatalf("%s inside the band must not trade", sym)
		}
	}
	if !strings.Contains(f.log.String(), policy.ReasonCoreRebalance) {
		t.Fatal("rebalance must be logged")
	}
}

// TestClientCapHaltsCoreAndSatellite: −16% on a 15% cap. MAX_DRAWDOWN once
// from the broker, CORE_HALT once from the core, no buys from either sleeve.
func TestClientCapHaltsCoreAndSatellite(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	f.svc.mu.Lock()
	f.svc.cash = 840_000
	f.svc.peak = 1_000_000
	f.svc.eq0 = 1_000_000
	f.svc.pos = map[string]*commonv1.Position{}
	f.svc.mu.Unlock()
	for i := 0; i < 2; i++ {
		core, sat := f.tick(t)
		if core != 0 || sat != 0 {
			t.Fatalf("tick %d: halted book bought core %d satellite %d", i, core, sat)
		}
		f.nextSession()
	}
	logs := f.log.String()
	if n := strings.Count(logs, broker.ReasonMaxDrawdown); n != 1 {
		t.Fatalf("MAX_DRAWDOWN once, got %d", n)
	}
	if n := strings.Count(logs, policy.LogCoreHalt); n != 1 {
		t.Fatalf("CORE_HALT once, got %d", n)
	}
	// A tighter cap halts where the book default would not.
	tight := ips.Default("c-2", costs.StartCash)
	tight.MaxDD = 0.10
	g := newCoreFixture(t, tight, nil)
	g.svc.mu.Lock()
	g.svc.cash = 890_000
	g.svc.peak = 1_000_000
	g.svc.mu.Unlock()
	if core, _ := g.tick(t); core != 0 {
		t.Fatalf("11%% drawdown under a 10%% cap must halt, got %d core fills", core)
	}
}

// TestCoreRebalancesWithSentimentDown: the sentiment/LLM side erroring is
// a satellite problem. The core is a calendar and a target; it builds.
func TestCoreRebalancesWithSentimentDown(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), snDown{})
	core, sat := f.tick(t)
	if core == 0 || sat != 0 {
		t.Fatalf("core %d satellite %d with sentiment down", core, sat)
	}
}

// TestCoreTicketsArePaperOnly: a built core changes nothing about the live
// gate. Ready() stays false and orders stay compile-time off.
func TestCoreTicketsArePaperOnly(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	if core, _ := f.tick(t); core == 0 {
		t.Fatal("fixture must fill")
	}
	if live.Ready() {
		t.Fatal("core tickets must not make the live gate ready")
	}
	if live.OrdersMode() != "compile-off" {
		t.Fatalf("orders mode %q", live.OrdersMode())
	}
}

// TestCoreNeverCallsStrategies: the core plan is derived from the IPS and
// the marks alone. Wiping the satellite plan changes nothing about it.
func TestCoreNeverCallsStrategies(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	f.svc.mu.Lock()
	in := f.svc.coreInputLocked(context.Background(), f.clk.Now(), map[string]float64{"TCS": 1000})
	f.svc.mu.Unlock()
	a := policy.Rebalance(in)
	f.svc.mu.Lock()
	f.svc.plan = bookPlan{}
	f.svc.w = nil
	in2 := f.svc.coreInputLocked(context.Background(), f.clk.Now(), map[string]float64{"TCS": 1000})
	f.svc.mu.Unlock()
	in2.SatelliteEmpty = in.SatelliteEmpty // the only thing the core asks the satellite
	b := policy.Rebalance(in2)
	if a.Reason != b.Reason || len(a.Tickets) != len(b.Tickets) {
		t.Fatalf("core plan depends on satellite state: %s vs %s", a.Reason, b.Reason)
	}
	for _, sym := range universe.EquitySymbols() {
		if _, ok := a.Targets.Core[sym]; !ok {
			t.Fatalf("%s missing from core", sym)
		}
	}
}
