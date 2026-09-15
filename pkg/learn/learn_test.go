package learn

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aperture/pkg/strategies"
)

func TestRegimeFromNiftyNotFromPnL(t *testing.T) {
	if Regime(1.2) != "bull" || Regime(-1.2) != "bear" || Regime(0.2) != "chop" {
		t.Fatalf("regime %s %s %s", Regime(1.2), Regime(-1.2), Regime(0.2))
	}
}

func TestSingleFillNeverMovesWeights(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	closes := []Close{{
		ID: "t-1", StrategyID: strategies.Momentum1d, Method: "momentum",
		Regime: "chop", PnL: -500, Excess: -1, HoldMs: 8 * 3600 * 1000,
	}}
	next, shrunk := Reweight(ids, eq, closes)
	if len(shrunk) != 0 {
		t.Fatalf("shrunk on one fill: %v", shrunk)
	}
	if next[strategies.Momentum1d] != eq[strategies.Momentum1d] {
		t.Fatalf("weight moved on a single fill: %v → %v", eq[strategies.Momentum1d], next[strategies.Momentum1d])
	}
}

func TestNineteenNegativeFillsDoNotShrink(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	var closes []Close
	for i := 0; i < MinN-1; i++ {
		closes = append(closes, Close{
			StrategyID: strategies.Momentum1d, Method: "momentum", Regime: "chop", PnL: -10,
		})
	}
	_, shrunk := Reweight(ids, eq, closes)
	if len(shrunk) != 0 {
		t.Fatalf("n=%d must not shrink: %v", MinN-1, shrunk)
	}
}

func TestTwentyNegativeTagShrinksOnWeeklyOnly(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	var closes []Close
	for i := 0; i < MinN; i++ {
		closes = append(closes, Close{
			StrategyID: strategies.Momentum1d, Method: "momentum", Regime: "chop", PnL: -10, Excess: -0.5,
		})
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	boot := EqualSnapshot(ids, now)
	held := ApplyWeekly(ids, boot, closes, now.Add(24*time.Hour), false)
	if held.Moved {
		t.Fatal("weights must not move before NextDue")
	}
	if SnapshotMap(held)[strategies.Momentum1d] != eq[strategies.Momentum1d] {
		t.Fatal("mid-week apply moved a weight")
	}
	weekly := ApplyWeekly(ids, boot, closes, now.Add(ReviewEvery), false)
	if !weekly.Moved {
		t.Fatal("weekly job should shrink the negative tag")
	}
	if weekly.Weights == nil {
		t.Fatal("missing weights")
	}
	got := SnapshotMap(weekly)[strategies.Momentum1d]
	if got >= eq[strategies.Momentum1d] {
		t.Fatalf("momentum should shrink: eq=%v got=%v shrunk=%v", eq[strategies.Momentum1d], got, weekly.Shrunk)
	}
}

func TestReviewPrintsExpectancyByMethod(t *testing.T) {
	closes := []Close{
		{StrategyID: strategies.Momentum1d, Method: "momentum", PnL: 100, Excess: 0.4, HoldMs: 8 * 3_600_000, MAE: -0.01, MFE: 0.03},
		{StrategyID: strategies.Momentum1d, Method: "momentum", PnL: -50, Excess: -0.2, HoldMs: 8 * 3_600_000, MAE: -0.02, MFE: 0.01},
		{StrategyID: strategies.SMACross1d, Method: "sma_cross", PnL: -20, Excess: -0.1, HoldMs: 24 * 3_600_000, MAE: -0.03, MFE: 0.00},
	}
	table := FormatTable(ByMethod(closes))
	if !strings.Contains(table, "method") || !strings.Contains(table, "expectancy") {
		t.Fatalf("header:\n%s", table)
	}
	if !strings.Contains(table, "momentum") || !strings.Contains(table, "sma_cross") {
		t.Fatalf("methods:\n%s", table)
	}
	t.Logf("\n%s", table)
}

func TestLedgerRoundTripAndPythonReview(t *testing.T) {
	dir := t.TempDir()
	c := NewClose("t-1", strategies.Momentum1d, "TCS", -12.5, -0.3, 8_000_000, -0.02, 0.01, "chop", 1, "comment")
	if err := AppendClose(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCloses(dir)
	if err != nil || len(got) != 1 {
		t.Fatalf("load %v %v", got, err)
	}
	if got[0].StrategyID != strategies.Momentum1d || got[0].MAE == 0 || got[0].HoldMs == 0 {
		t.Fatalf("facts %+v", got[0])
	}
	snap := EqualSnapshot(strategies.IDs(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err := SaveSnapshot(dir, snap); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(dir)
	if err != nil || len(loaded.Weights) == 0 {
		t.Fatal(err)
	}

	table := FormatTable(ByMethod(got))
	_, this, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(this), "../.."))
	script := filepath.Join(root, "scripts/review.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("python review missing: %v", err)
	}
	cmd := exec.Command("python3", script, ClosesPath(dir))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python review: %v\n%s", err, out)
	}
	py := string(out)
	if !strings.Contains(py, "method") || !strings.Contains(py, "expectancy") || !strings.Contains(py, "momentum") {
		t.Fatalf("python table:\n%s", py)
	}
	if !strings.Contains(table, "momentum") {
		t.Fatalf("go table:\n%s", table)
	}
}

func TestFactTagsRoundTrip(t *testing.T) {
	tags := FactTags(-0.0125, 0.04, "bear", 12345)
	mae, mfe, regime, hold := ParseFacts(tags)
	if mae != -0.0125 || mfe != 0.04 || regime != "bear" || hold != 12345 {
		t.Fatalf("%v %v %s %d", mae, mfe, regime, hold)
	}
	kept := KeepFactTags(append(tags, "win", "hurt_vs_nifty"))
	if len(kept) != 4 {
		t.Fatalf("kept %v", kept)
	}
}

func TestBackupCloses(t *testing.T) {
	dir := t.TempDir()
	c := Close{ID: "t-1", StrategyID: strategies.Momentum1d, PnL: -1}
	if err := AppendClose(dir, c); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	bak := filepath.Join(dir, BackupDir, "closes-"+day+".jsonl")
	b, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"id":"t-1"`) {
		t.Fatalf("backup missing close: %s", b)
	}
}
