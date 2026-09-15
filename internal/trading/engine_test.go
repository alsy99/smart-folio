package trading

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/learn"
	"aperture/pkg/live"
	"aperture/pkg/strategies"
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

func TestPositionalExitNeedsASession(t *testing.T) {
	if shouldExit(false, 45*time.Second, 0.05, 1) {
		t.Fatal("must not close on the 45s scalp path")
	}
	if shouldExit(false, costs.SessionHold-time.Second, -0.02, 0) {
		t.Fatal("must hold through the session even if the signal flipped")
	}
	if !shouldExit(false, costs.SessionHold, 0.01, 0) {
		t.Fatal("after a session, flat/short signal should exit")
	}
	if shouldExit(false, costs.SessionHold, 0.01, 1) {
		t.Fatal("still long after a session — keep the name")
	}
	if !shouldExit(false, costs.MaxHold, 0.01, 1) {
		t.Fatal("weeks-long cap should recycle the name")
	}
}

func TestScalpExitPath(t *testing.T) {
	if !shouldExit(true, costs.ScalpMaxHold+time.Second, 0, 1) {
		t.Fatal("45s")
	}
	if !shouldExit(true, time.Second, 0.013, 1) {
		t.Fatal("take")
	}
	if !shouldExit(true, time.Second, -0.009, 1) {
		t.Fatal("stop")
	}
	if shouldExit(true, time.Second, 0.001, 1) {
		t.Fatal("hold")
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
