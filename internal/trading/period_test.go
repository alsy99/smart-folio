package trading

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	learningv1 "aperture/gen/learning/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/learn"
	"aperture/pkg/marketclock"

	"google.golang.org/grpc"
)

// recordingLearning is zeroWeights that keeps every RecordPeriod it sees.
type recordingLearning struct {
	zeroWeights
	mu      sync.Mutex
	periods []*learningv1.Period
}

func (r *recordingLearning) RecordPeriod(_ context.Context, req *learningv1.RecordPeriodRequest, _ ...grpc.CallOption) (*learningv1.RecordPeriodResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.periods = append(r.periods, req.Period)
	return &learningv1.RecordPeriodResponse{}, nil
}

func (r *recordingLearning) wait(t *testing.T, n int) []*learningv1.Period {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		got := len(r.periods)
		r.mu.Unlock()
		if got >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*learningv1.Period(nil), r.periods...)
}

// TestMonthEndBooksACorePeriodAndCoreDoesNotLearn: the core builds over
// September, the 30 Sep session closes the period from the first
// rebalance to month end, the row goes to learning with the IPS id and
// sleeve, and the IPS is exactly what the client set: no field, least of
// all SatellitePct, moved because of a period.
func TestMonthEndBooksACorePeriodAndCoreDoesNotLearn(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	ln := &recordingLearning{}
	f.svc.ln = ln
	before := *f.svc.ips
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
	if ln.wait(t, 0); len(ln.periods) != 0 {
		t.Fatal("the initial build is not a period boundary")
	}
	f.md.set("TCS", 1400)
	f.clk.Set(time.Date(2026, 9, 30, 15, 25, 0, 0, marketclock.Location()))
	f.tick(t)
	got := ln.wait(t, 1)
	if len(got) != 1 {
		t.Fatalf("month end should book exactly one period (core; satellite empty), got %d", len(got))
	}
	p := got[0]
	if p.IpsId != "c-1" || p.Sleeve != learn.SleeveCore || p.Method != learn.MethodCore {
		t.Fatalf("period identity %+v", p)
	}
	if time.UnixMilli(p.ToUnixMs).In(marketclock.Location()).Format("2006-01-02") != "2026-09-30" {
		t.Fatalf("period should end on the rebalance session: %+v", p)
	}
	if p.Fills == 0 {
		t.Fatal("the build's tickets belong to the first period")
	}
	// TCS rallied 40% in the core: the period is up and beat a flat
	// benchmark tape, and the drawdown is bounded.
	if p.PnlAfterCosts <= 0 || p.ExcessVsIpsPp <= 0 || p.ExcessVsNiftyPp <= 0 {
		t.Fatalf("a rally in a core name must show in the period: %+v", p)
	}
	if p.MaxDd < 0 || p.MaxDd > 0.15 {
		t.Fatalf("max dd %v out of range", p.MaxDd)
	}
	if !strings.Contains(f.log.String(), LogPeriod) {
		t.Fatal("period must be logged")
	}
	f.svc.mu.Lock()
	after := *f.svc.ips
	next := f.svc.period
	f.svc.mu.Unlock()
	if after != before {
		t.Fatalf("a period changed the IPS: %+v → %+v", before, after)
	}
	// The fresh period opened at the rebalance marks, before the session's
	// tickets: the one TCS sell is its first flow.
	if next == nil || next.core.fills != 1 || next.core.flow <= 0 {
		t.Fatalf("a fresh period must open at the rebalance marks and carry that session's tickets: %+v", next.core)
	}
	// The satellite is empty: no satellite row, and nothing here wrote a
	// weight. RecordPeriod is the only learning call the period makes.
	for _, q := range got {
		if q.Sleeve == learn.SleeveSatellite {
			t.Fatalf("empty satellite booked a period: %+v", q)
		}
	}
}

// TestCampaignEndClosesTheTailPeriodOnce: the tail from the last rebalance
// to the campaign end is booked once when the clock passes the end.
func TestCampaignEndClosesTheTailPeriodOnce(t *testing.T) {
	f := newCoreFixture(t, ips.Default("c-1", costs.StartCash), nil)
	ln := &recordingLearning{}
	f.svc.ln = ln
	for i := 0; i < 3; i++ {
		f.tick(t)
		f.nextSession()
	}
	f.clk.Set(f.clk.Now().AddDate(0, 0, 40))
	for i := 0; i < 3; i++ {
		resp, err := f.svc.Tick(context.Background(), &tradingv1.TickRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if !resp.Skipped || resp.Reason != "campaign ended" {
			t.Fatalf("tick %d: %+v", i, resp)
		}
	}
	got := ln.wait(t, 1)
	if len(got) != 1 {
		t.Fatalf("campaign end should book the tail once, got %d", len(got))
	}
	if got[0].Sleeve != learn.SleeveCore || got[0].Fills == 0 {
		t.Fatalf("tail period %+v", got[0])
	}
	f.svc.mu.Lock()
	defer f.svc.mu.Unlock()
	if f.svc.period != nil {
		t.Fatal("no period stays open after the campaign")
	}
	// Flows reconcile: the core's after-cost P&L is its marks minus what
	// it paid; on a flat tape that is minus the costs, small and negative.
	if pnl := got[0].PnlAfterCosts; pnl >= 0 || math.Abs(pnl) > 0.01*costs.StartCash {
		t.Fatalf("flat-tape core period should cost a little, got %v", pnl)
	}
}
