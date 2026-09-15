package learn

import (
	"math"
	"strings"
	"testing"
	"time"

	"aperture/pkg/strategies"
)

func satPeriod(ipsID, method string, xsIPS float64, fills int) Period {
	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	return Period{
		IPSID: ipsID, Sleeve: SleeveSatellite, Method: method,
		From: from, To: from.AddDate(0, 1, 0),
		PnLAfterCosts: xsIPS * 1000, ExcessVsIPS: xsIPS, ExcessVsNifty: xsIPS - 0.1,
		MaxDD: 0.02, Fills: fills, MAE: -0.01, MFE: 0.02,
	}
}

func corePeriod(ipsID string, xsIPS float64) Period {
	p := satPeriod(ipsID, MethodCore, xsIPS, 12)
	p.Sleeve = SleeveCore
	return p
}

func TestOneLosingPeriodOrFillDoesNotMoveWeights(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	periods := []Period{satPeriod("ips-a", "momentum", -3, 1)}
	closes := []Close{{ID: "t-1", StrategyID: strategies.Momentum1d, Method: "momentum", PnL: -900, Excess: -3}}
	next, shrunk := ReweightPeriods(ids, eq, periods, closes)
	if len(shrunk) != 0 {
		t.Fatalf("shrunk on one losing period: %v", shrunk)
	}
	for id := range eq {
		if math.Abs(next[id]-eq[id]) > 1e-12 {
			t.Fatalf("%s moved %v → %v on a single loss", id, eq[id], next[id])
		}
	}
	// Nineteen periods and nineteen fills together still do not qualify.
	periods = nil
	closes = nil
	for i := 0; i < MinN-1; i++ {
		periods = append(periods, satPeriod("ips-a", "momentum", -3, 1))
		closes = append(closes, Close{StrategyID: strategies.Momentum1d, Method: "momentum", PnL: -10, Excess: -3})
	}
	if _, shrunk := ReweightPeriods(ids, eq, periods, closes); len(shrunk) != 0 {
		t.Fatalf("n=%d must not shrink: %v", MinN-1, shrunk)
	}
}

func TestTwentyLosingPeriodsShrinkSatelliteMethodTowardZero(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	var periods []Period
	for i := 0; i < MinN; i++ {
		periods = append(periods, satPeriod("ips-a", "momentum", -0.4, 0))
	}
	next, shrunk := ReweightPeriods(ids, eq, periods, nil)
	if len(shrunk) != 1 || shrunk[0] != "ips-a|satellite|momentum" {
		t.Fatalf("shrunk %v", shrunk)
	}
	if next[strategies.Momentum1d] >= eq[strategies.Momentum1d] {
		t.Fatalf("momentum did not shrink: %v → %v", eq[strategies.Momentum1d], next[strategies.Momentum1d])
	}
	// Repeated reviews walk it to exactly zero, not a 5% floor: the period
	// rule is "shrink toward 0".
	w := next
	for i := 0; i < 8; i++ {
		w, _ = ReweightPeriods(ids, w, periods, nil)
	}
	if w[strategies.Momentum1d] != 0 {
		t.Fatalf("momentum should reach 0 after repeated losing reviews, got %v", w[strategies.Momentum1d])
	}
	// A positive-expectancy method with the same n is untouched.
	var good []Period
	for i := 0; i < MinN; i++ {
		good = append(good, satPeriod("ips-a", "breakout", +0.2, 0))
	}
	next2, shrunk2 := ReweightPeriods(ids, eq, good, nil)
	if len(shrunk2) != 0 || next2[strategies.Breakout1d] != eq[strategies.Breakout1d] {
		t.Fatalf("positive expectancy must not shrink: %v %v", shrunk2, next2[strategies.Breakout1d])
	}
}

func TestTwentySatelliteFillsQualifyWithFewPeriods(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	periods := []Period{satPeriod("ips-a", "momentum", -1, 20)}
	var closes []Close
	for i := 0; i < MinN; i++ {
		closes = append(closes, Close{StrategyID: strategies.Momentum1d, Method: "momentum", PnL: -10, Excess: -1})
	}
	_, shrunk := ReweightPeriods(ids, eq, periods, closes)
	if len(shrunk) != 1 {
		t.Fatalf("n≥20 fills with one losing period should qualify: %v", shrunk)
	}
}

func TestCoreDoesNotLearn(t *testing.T) {
	ids := strategies.IDs()
	eq := EqualWeights(ids)
	var periods []Period
	for i := 0; i < 3*MinN; i++ {
		periods = append(periods, corePeriod("ips-a", -5))
	}
	next, shrunk := ReweightPeriods(ids, eq, periods, nil)
	if len(shrunk) != 0 {
		t.Fatalf("core periods moved a weight: %v", shrunk)
	}
	for id := range eq {
		if next[id] != eq[id] {
			t.Fatalf("%s moved on core periods", id)
		}
	}
	// A satellite row mislabelled with the core method is ignored too.
	bad := satPeriod("ips-a", MethodCore, -5, 50)
	if _, shrunk := ReweightPeriods(ids, eq, []Period{bad, bad, bad}, nil); len(shrunk) != 0 {
		t.Fatalf("core method under the satellite sleeve moved a weight: %v", shrunk)
	}
	// The table still shows the core so the client can read it.
	table := FormatPeriodTable(ByIPSSleeve(periods))
	if !strings.Contains(table, "ips-a") || !strings.Contains(table, "core") {
		t.Fatalf("table must list the core row:\n%s", table)
	}
}

func TestPeriodTableGroupsByIPSAndSleeve(t *testing.T) {
	periods := []Period{
		satPeriod("ips-b", "momentum", -1, 2),
		corePeriod("ips-b", 0.1),
		satPeriod("ips-a", "breakout", 0.3, 1),
		corePeriod("ips-a", -0.2),
		satPeriod("ips-a", "breakout", 0.5, 1),
	}
	rows := ByIPSSleeve(periods)
	want := []string{"ips-a|core|core", "ips-a|satellite|breakout", "ips-b|core|core", "ips-b|satellite|momentum"}
	if len(rows) != len(want) {
		t.Fatalf("rows %d want %d", len(rows), len(want))
	}
	for i, r := range rows {
		if r.Key() != want[i] {
			t.Fatalf("row %d = %s want %s", i, r.Key(), want[i])
		}
	}
	if rows[1].N != 2 || rows[1].Fills != 2 || math.Abs(rows[1].ExpectancyVsIPS()-0.4) > 1e-9 {
		t.Fatalf("breakout row %+v", rows[1])
	}
	table := FormatPeriodTable(rows)
	lines := strings.Split(strings.TrimSpace(table), "\n")
	if len(lines) != 1+len(want) {
		t.Fatalf("table lines %d:\n%s", len(lines), table)
	}
	if !strings.HasPrefix(lines[0], "ips") || !strings.Contains(lines[0], "sleeve") || !strings.Contains(lines[0], "xs_ips_pp") {
		t.Fatalf("header: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "ips-a") || !strings.Contains(lines[1], "core") {
		t.Fatalf("first row should be ips-a core: %s", lines[1])
	}
	if FormatPeriodTable(nil) != periodHeader+"\n(no periods)\n" {
		t.Fatal("empty table copy")
	}
}

func TestPeriodLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if ps, err := LoadPeriods(dir); err != nil || len(ps) != 0 {
		t.Fatalf("missing file must read as empty: %v %v", ps, err)
	}
	p := satPeriod("ips-a", "momentum", -1.25, 3)
	if err := AppendPeriod(dir, p); err != nil {
		t.Fatal(err)
	}
	if err := AppendPeriod(dir, corePeriod("ips-a", 0.4)); err != nil {
		t.Fatal(err)
	}
	ps, err := LoadPeriods(dir)
	if err != nil || len(ps) != 2 {
		t.Fatalf("%v %v", ps, err)
	}
	if ps[0].ExcessVsIPS != -1.25 || ps[0].Fills != 3 || !ps[0].From.Equal(p.From) || ps[1].Sleeve != SleeveCore {
		t.Fatalf("round trip lost fields: %+v", ps)
	}
	// Writing periods never creates or touches weights.json.
	if _, err := LoadSnapshot(dir); err == nil {
		t.Fatal("AppendPeriod must not write weights.json")
	}
}

func TestReviewAppliesPeriodsOnScheduleOnly(t *testing.T) {
	ids := strategies.IDs()
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	prev := EqualSnapshot(ids, now)
	var periods []Period
	for i := 0; i < MinN; i++ {
		periods = append(periods, satPeriod("ips-a", "momentum", -0.4, 0))
	}
	held := ApplyReview(ids, prev, nil, periods, now.Add(time.Hour), false)
	if held.Moved || SnapshotMap(held)[strategies.Momentum1d] != SnapshotMap(prev)[strategies.Momentum1d] {
		t.Fatal("periods must not move weights before the job is due")
	}
	due := ApplyReview(ids, prev, nil, periods, now.Add(ReviewEvery+time.Hour), false)
	if !due.Moved || SnapshotMap(due)[strategies.Momentum1d] >= SnapshotMap(prev)[strategies.Momentum1d] {
		t.Fatalf("due job should shrink momentum: %+v", due)
	}
	if !strings.Contains(due.Note, "ips-a|satellite|momentum") {
		t.Fatalf("note should name the period tag: %s", due.Note)
	}
}
