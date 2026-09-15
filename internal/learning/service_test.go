package learning

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	"aperture/pkg/learn"
	"aperture/pkg/strategies"
)

func TestRecordTradeDoesNotMoveWeights(t *testing.T) {
	s := NewDir(nil, t.TempDir())
	before, err := s.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordTrade(context.Background(), &learningv1.RecordTradeRequest{
		Trade: &commonv1.PaperTrade{
			Id: "t-1", StrategyId: strategies.Momentum1d, Symbol: "TCS",
			Pnl: -400, ExcessReturn: -1.2, OpenedAtUnixMs: 1, ClosedAtUnixMs: 8_000_001,
			AttributionTags: learn.FactTags(-0.03, 0.01, "chop", 8_000_000),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !sameWeights(before.Weights, after.Weights) {
		t.Fatal("a single close must not retrain weights")
	}
	closes, err := learn.LoadCloses(s.dir)
	if err != nil || len(closes) != 1 {
		t.Fatalf("stored closes %v %v", closes, err)
	}
	c := closes[0]
	if c.StrategyID != strategies.Momentum1d || c.Regime != "chop" || c.PnL != -400 || c.HoldMs == 0 || c.MAE == 0 {
		t.Fatalf("facts %+v", c)
	}
}

func TestWeeklyScheduleShrinksNegativeTag(t *testing.T) {
	dir := t.TempDir()
	s := NewDir(nil, dir)
	boot := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return boot }
	s.snap = learn.EqualSnapshot(strategies.IDs(), boot)
	if err := learn.SaveSnapshot(dir, s.snap); err != nil {
		t.Fatal(err)
	}
	eq := weightOf(t, s, strategies.Momentum1d)
	for i := 0; i < learn.MinN; i++ {
		_, err := s.RecordTrade(context.Background(), &learningv1.RecordTradeRequest{
			Trade: &commonv1.PaperTrade{
				Id: "t-" + strconv.Itoa(i), StrategyId: strategies.Momentum1d, Symbol: "TCS",
				Pnl: -25, ExcessReturn: -0.4,
				AttributionTags: learn.FactTags(-0.02, 0.0, "chop", 8_000_000),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := weightOf(t, s, strategies.Momentum1d); got != eq {
		t.Fatalf("fills moved weight %v → %v", eq, got)
	}
	s.now = func() time.Time { return boot.Add(24 * time.Hour) }
	mid, err := s.RunWeekly(false)
	if err != nil {
		t.Fatal(err)
	}
	if mid.Moved {
		t.Fatal("weekly job must wait for NextDue")
	}
	if weightOf(t, s, strategies.Momentum1d) != eq {
		t.Fatal("mid-week RunWeekly moved weights")
	}
	s.now = func() time.Time { return boot.Add(learn.Period) }
	done, err := s.RunWeekly(false)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Moved {
		t.Fatal("due weekly job should shrink the n≥20 negative tag")
	}
	got := weightOf(t, s, strategies.Momentum1d)
	if got >= eq {
		t.Fatalf("momentum weight should drop: %v → %v", eq, got)
	}
}

func TestReviewTableFromServiceCloses(t *testing.T) {
	s := NewDir(nil, t.TempDir())
	for _, pnl := range []float64{80, -20, -20} {
		if _, err := s.RecordTrade(context.Background(), &learningv1.RecordTradeRequest{
			Trade: &commonv1.PaperTrade{Id: "x", StrategyId: strategies.Momentum1d, Pnl: pnl, ExcessReturn: pnl / 100},
		}); err != nil {
			t.Fatal(err)
		}
	}
	closes, err := learn.LoadCloses(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	table := learn.FormatTable(learn.ByMethod(closes))
	if table == "" || len(closes) != 3 {
		t.Fatalf("table %q closes %d", table, len(closes))
	}
	t.Logf("\n%s", table)
}

// TestNoRosterFileBootsDefaultsAndSaysSo: with data/roster/ empty the desk
// runs the shipped daily specs and the report's first line says exactly that,
// so the hero can print it instead of implying a lab vetted the book.
func TestNoRosterFileBootsDefaultsAndSaysSo(t *testing.T) {
	t.Setenv("ROSTER_DIR", t.TempDir())
	s := NewDir(nil, t.TempDir())
	if s.rosterSrc != RosterDefault {
		t.Fatalf("roster source %q", s.rosterSrc)
	}
	rep, err := s.GetBacktest(context.Background(), &learningv1.GetBacktestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(rep.Note, "\n", 2)[0]
	if first != "Roster: "+RosterDefault {
		t.Fatalf("hero line %q", first)
	}
	want := strategies.IDs()
	if len(s.roster) != len(want) {
		t.Fatalf("default roster %v", s.roster)
	}
}

// TestMockTapeNeverWritesRoster: with no INDstocks token and no cached bars
// the lab runs on the mock tape, reports, and leaves data/roster/ empty.
func TestMockTapeNeverWritesRoster(t *testing.T) {
	rosterDir := t.TempDir()
	t.Setenv("ROSTER_DIR", rosterDir)
	t.Setenv("BARS_DIR", filepath.Join(t.TempDir(), "none"))
	for _, k := range []string{"INDSTOCKS_ACCESS_TOKEN", "INDMONEY_API_TOKEN", "INDMONEY_ACCESS_TOKEN", "INDSTOCKS_API_KEY", "INDMONEY_CLIENT_ID"} {
		t.Setenv(k, "")
	}
	s := NewDir(nil, t.TempDir())
	rep, err := s.RunBacktest(context.Background(), &learningv1.RunBacktestRequest{Years: 5})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "complete" {
		t.Fatalf("status %s: %s", rep.Status, rep.Note)
	}
	if !strings.Contains(rep.Note, "MOCK") {
		t.Fatalf("note must name the mock tape: %q", rep.Note)
	}
	ents, _ := os.ReadDir(rosterDir)
	if len(ents) != 0 {
		t.Fatalf("mock tape wrote a roster: %v", ents)
	}
	if s.rosterSrc != RosterDefault {
		t.Fatalf("roster source moved on mock evidence: %q", s.rosterSrc)
	}
}

func sameWeights(a, b []*commonv1.StrategyWeight) bool {
	if len(a) != len(b) {
		return false
	}
	am := map[string]float64{}
	for _, w := range a {
		am[w.StrategyId] = w.Weight
	}
	for _, w := range b {
		if math.Abs(am[w.StrategyId]-w.Weight) > 1e-12 {
			return false
		}
	}
	return true
}

func weightOf(t *testing.T, s *Service, id string) float64 {
	t.Helper()
	resp, err := s.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range resp.Weights {
		if w.StrategyId == id {
			return w.Weight
		}
	}
	t.Fatalf("missing %s", id)
	return 0
}
