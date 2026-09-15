package learning

import (
	"context"
	"os"
	"testing"
	"time"

	learningv1 "aperture/gen/learning/v1"
	"aperture/pkg/learn"
	"aperture/pkg/strategies"
)

func losingPeriod(i int) *learningv1.Period {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0)
	return &learningv1.Period{
		IpsId: "ips-a", Sleeve: learn.SleeveSatellite, Method: "momentum",
		FromUnixMs: from.UnixMilli(), ToUnixMs: from.AddDate(0, 1, 0).UnixMilli(),
		PnlAfterCosts: -400, ExcessVsIpsPp: -0.6, ExcessVsNiftyPp: -0.7, MaxDd: 0.03, Fills: 1,
	}
}

// TestRecordPeriodNeverWritesWeights: periods are evidence. Twenty losing
// satellite periods sit in periods.jsonl and weights.json is untouched
// until the scheduled review runs; then, and only then, the method shrinks.
func TestRecordPeriodNeverWritesWeights(t *testing.T) {
	t.Setenv("ROSTER_DIR", t.TempDir())
	dir := t.TempDir()
	s := NewDir(nil, dir)
	boot := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return boot }
	s.snap = learn.EqualSnapshot(strategies.IDs(), boot)
	if err := learn.SaveSnapshot(dir, s.snap); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(learn.WeightsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	eq := weightOf(t, s, strategies.Momentum1d)
	for i := 0; i < learn.MinN; i++ {
		if _, err := s.RecordPeriod(context.Background(), &learningv1.RecordPeriodRequest{Period: losingPeriod(i)}); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(learn.WeightsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("RecordPeriod rewrote weights.json")
	}
	if got := weightOf(t, s, strategies.Momentum1d); got != eq {
		t.Fatalf("periods moved the served weight %v → %v", eq, got)
	}
	periods, err := learn.LoadPeriods(dir)
	if err != nil || len(periods) != learn.MinN {
		t.Fatalf("periods stored %d %v", len(periods), err)
	}
	if periods[0].IPSID != "ips-a" || periods[0].Sleeve != learn.SleeveSatellite || periods[0].ExcessVsIPS != -0.6 {
		t.Fatalf("period fields lost: %+v", periods[0])
	}
	// Not due: the job holds.
	s.now = func() time.Time { return boot.Add(time.Hour) }
	if snap, err := s.RunWeekly(false); err != nil || snap.Moved {
		t.Fatalf("review before due moved weights: %v %v", snap.Moved, err)
	}
	// Due: the scheduled job is the one writer.
	s.now = func() time.Time { return boot.Add(learn.ReviewEvery) }
	snap, err := s.RunWeekly(false)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Moved || weightOf(t, s, strategies.Momentum1d) >= eq {
		t.Fatalf("due review should shrink momentum on 20 losing periods: %+v", snap)
	}
}

// TestCorePeriodsAreStoredNotLearned: the core sleeve reports its periods
// for the table but a losing core never moves a satellite weight.
func TestCorePeriodsAreStoredNotLearned(t *testing.T) {
	t.Setenv("ROSTER_DIR", t.TempDir())
	dir := t.TempDir()
	s := NewDir(nil, dir)
	boot := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return boot.Add(learn.ReviewEvery) }
	s.snap = learn.EqualSnapshot(strategies.IDs(), boot)
	for i := 0; i < 2*learn.MinN; i++ {
		p := losingPeriod(i)
		p.Sleeve, p.Method = learn.SleeveCore, learn.MethodCore
		if _, err := s.RecordPeriod(context.Background(), &learningv1.RecordPeriodRequest{Period: p}); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := s.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	snap, err := s.RunWeekly(true)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	if snap.Moved || !sameWeights(before.Weights, after.Weights) {
		t.Fatalf("core periods moved weights: %+v", snap)
	}
	if _, err := s.RecordPeriod(context.Background(), &learningv1.RecordPeriodRequest{}); err != nil {
		t.Fatalf("empty request should be a no-op, got %v", err)
	}
}
