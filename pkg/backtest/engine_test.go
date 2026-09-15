package backtest

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"aperture/pkg/costs"
	"aperture/pkg/strategies"
)

func TestOOSWindowsAreExpanding(t *testing.T) {
	end := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	days := tradingDays(end, 5)
	wins := oosWindows(days)
	if len(wins) != 2 {
		t.Fatalf("5y tape should yield 2 OOS folds, got %d", len(wins))
	}
	// fold 1: train ends ~2024-09, test 2024-09→2025-09
	// fold 2: train ends ~2025-09, test 2025-09→2026-09
	if wins[0][0] >= wins[1][0] {
		t.Fatalf("folds must roll forward: %v", wins)
	}
	if wins[0][1] != wins[1][0] {
		t.Fatalf("fold 1 test must end where fold 2 test starts: %v", wins)
	}
	if wins[1][1] != len(days) {
		t.Fatalf("last fold must run to tape end: %v", wins[1])
	}
	trainYears := days[wins[0][0]].Sub(days[0]).Hours() / 24 / 365
	if trainYears < 2.8 {
		t.Fatalf("first fold needs ~3y train, got %.1fy", trainYears)
	}
}

func TestPromotionGate(t *testing.T) {
	good := Variant{Spec: strategies.Spec{ID: "x"}, ExcessPct: 1.5, ReturnPct: 2.0, Trades: 42, MaxDDPct: 9.0}
	if !passesGate(good) {
		t.Fatal("qualifying variant must pass the gate")
	}
	cases := map[string]Variant{
		"no excess":   {ExcessPct: -0.1, ReturnPct: 2.0, Trades: 42, MaxDDPct: 9.0},
		"lost money":  {ExcessPct: 5.0, ReturnPct: -3.3, Trades: 180, MaxDDPct: 4.5}, // beat a falling Nifty by losing less
		"too few":     {ExcessPct: 1.5, ReturnPct: 2.0, Trades: MinOOSTrades - 1, MaxDDPct: 9.0},
		"dd over cap": {ExcessPct: 1.5, ReturnPct: 2.0, Trades: 42, MaxDDPct: DDCapPct + 0.1},
	}
	for name, v := range cases {
		if passesGate(v) {
			t.Fatalf("%s must not pass the gate: %+v", name, v)
		}
	}
	if DDCapPct != costs.DrawdownHalt*100 {
		t.Fatalf("gate cap %.0f%% must mirror the book halt %.0f%%", DDCapPct, costs.DrawdownHalt*100)
	}
}

func TestRunProducesDatedSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	rep := Run(5, now)
	if rep.Status != "complete" {
		t.Fatalf("status %s note %s", rep.Status, rep.Note)
	}
	snap := rep.Snapshot
	if snap.Date != "2026-09-15" {
		t.Fatalf("snapshot must be dated to the run day, got %q", snap.Date)
	}
	if len(snap.Roster) == 0 {
		t.Fatal("roster must not be empty")
	}
	if len(snap.Added) > MaxNewPerSnap {
		t.Fatalf("new admissions capped at %d, got %d", MaxNewPerSnap, len(snap.Added))
	}
	// Mock evidence never promotes and never reaches disk.
	if rep.Promotable() || rep.Tape != TapeMock {
		t.Fatalf("mock report must not be promotable: tape=%q", rep.Tape)
	}
	if len(snap.Added) != 0 {
		t.Fatalf("mock tape admitted %v", snap.Added)
	}
	if _, err := SaveRoster(t.TempDir(), snap); err == nil {
		t.Fatal("SaveRoster must refuse a mock-tape snapshot")
	}
	// on the mock tape (plumbing check) every default spec ships on the roster
	for _, d := range strategies.DefaultSpecs() {
		found := false
		for _, id := range snap.Roster {
			if id == d.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("default %s missing from roster %v", d.ID, snap.Roster)
		}
	}
	// every new admission must have passed the gate
	byID := map[string]Variant{}
	for _, v := range rep.Variants {
		byID[v.Spec.ID] = v
	}
	for _, p := range snap.Added {
		if !passesGate(byID[p.ID]) {
			t.Fatalf("admission %s failed the gate: %+v", p.ID, byID[p.ID])
		}
	}
}

// stubRealTape is a deterministic non-mock tape: a drifting, oscillating
// price so methods trade, without being the sine wave the mock uses.
type stubRealTape struct{}

func (stubRealTape) Name() string { return "stub-real-1d" }
func (stubRealTape) Days(years int, now time.Time) ([]time.Time, error) {
	return tradingDays(now, years), nil
}
func (stubRealTape) Closes(symbol string, days []time.Time) ([]float64, error) {
	seed := float64(len(symbol))
	out := make([]float64, len(days))
	for i := range days {
		x := float64(i)
		out[i] = 1000 * (1 - 0.00015*x) * (1 + 0.06*math.Sin(x/23+seed) + 0.02*math.Sin(x/5+seed*2))
	}
	return out, nil
}

// TestRealTapeSplitsRosterFromFailing: on a real tape a shipped default is
// not grandfathered. It is on the roster iff it clears the gate; otherwise
// it is under failing, and the two sets are disjoint. The paper book gives
// failing weight 0 — otherwise the gate is decoration.
func TestRealTapeSplitsRosterFromFailing(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	rep := RunOn(stubRealTape{}, 5, now)
	if rep.Status != "complete" {
		t.Fatalf("status %s note %s", rep.Status, rep.Note)
	}
	if !rep.Promotable() {
		t.Fatal("a non-mock tape is promotable")
	}
	byID := map[string]Variant{}
	for _, v := range rep.Variants {
		byID[v.Spec.ID] = v
	}
	on := map[string]bool{}
	for _, id := range rep.Snapshot.Roster {
		on[id] = true
		if !passesGate(byID[id]) {
			t.Fatalf("%s is on the roster but fails the gate: %+v", id, byID[id])
		}
	}
	for _, id := range rep.Snapshot.Failing {
		if on[id] {
			t.Fatalf("%s is both on the roster and failing", id)
		}
		if passesGate(byID[id]) {
			t.Fatalf("%s passes the gate but is listed failing", id)
		}
	}
	for _, d := range strategies.DefaultSpecs() {
		inFail := false
		for _, id := range rep.Snapshot.Failing {
			inFail = inFail || id == d.ID
		}
		if on[d.ID] == inFail {
			t.Fatalf("default %s must be in exactly one of roster/failing (roster=%v failing=%v)", d.ID, on[d.ID], inFail)
		}
	}
	// A snapshot that admits nothing but judged the defaults is still a
	// valid file: the book holds cash rather than trading unvetted specs.
	dir := t.TempDir()
	judged := RosterSnapshot{Date: "2026-09-15", Roster: nil, Failing: []string{"momentum_1d"}, Tape: "stub-real-1d"}
	if _, err := SaveRoster(dir, judged); err != nil {
		t.Fatalf("empty roster with failing must save: %v", err)
	}
	got, _, err := LoadLatestRoster(dir)
	if err != nil || len(got.Roster) != 0 || len(got.Failing) != 1 {
		t.Fatalf("round trip %+v %v", got, err)
	}
	if _, err := SaveRoster(dir, RosterSnapshot{Date: "2026-09-16", Tape: "stub-real-1d"}); err == nil {
		t.Fatal("a snapshot that judged nothing must be refused")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := RosterSnapshot{Date: "2026-09-01", Roster: []string{"a"}, Folds: 2, Tape: "indstocks-1d"}
	fresh := RosterSnapshot{Date: "2026-09-15", Roster: []string{"a", "b"}, Folds: 2, Tape: "indstocks-1d"}
	if _, err := SaveRoster(dir, old); err != nil {
		t.Fatal(err)
	}
	path, err := SaveRoster(dir, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2026-09-15.json" {
		t.Fatalf("snapshot path must be dated, got %s", path)
	}
	got, gotPath, err := LoadLatestRoster(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != "2026-09-15" || len(got.Roster) != 2 {
		t.Fatalf("latest snapshot %+v from %s", got, gotPath)
	}
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if RosterStale(got, now, 7) {
		t.Fatal("same-day snapshot is fresh")
	}
	if !RosterStale(got, now.AddDate(0, 0, 8), 7) {
		t.Fatal("8-day-old snapshot is stale")
	}
}

func TestSimulateFoldOnlyTradesOOS(t *testing.T) {
	end := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	days := tradingDays(end, 5)
	closes := series("RELIANCE", days)
	nifty := series("NIFTY50", days)
	wins := oosWindows(days)
	spec := strategies.Parse(strategies.Momentum1d)
	fr := simulateFold(spec, closes, nifty, wins[0][0], wins[0][1])
	if fr.trades == 0 {
		t.Fatal("momentum on 1d should trade in a 1y OOS window")
	}
	if fr.maxDDPct < 0 || fr.maxDDPct > 100 {
		t.Fatalf("maxDD out of range: %.2f", fr.maxDDPct)
	}
}
