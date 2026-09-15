package policy

import (
	"math"
	"strings"
	"testing"
	"time"

	"aperture/pkg/broker"
	"aperture/pkg/core"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"
)

// August 2026 on the NSE calendar: the 31st is a Monday and the last
// session of the month. The 28th is a Friday and is not.
var aug = CalendarFrom([]string{
	"2026-08-14", "2026-08-17", "2026-08-18", "2026-08-19", "2026-08-20", "2026-08-21",
	"2026-08-24", "2026-08-25", "2026-08-26", "2026-08-27", "2026-08-28", "2026-08-31",
	"2026-09-01", "2026-09-02",
})

func at(date string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", date+" 15:30", marketclock.Location())
	return t
}

func marks() map[string]float64 {
	m := map[string]float64{}
	for _, inst := range universe.Equities() {
		m[inst.Symbol] = inst.BasePrice
	}
	return m
}

func cashBook(cash float64) broker.Snapshot {
	return broker.Snapshot{Equity: cash, Peak: cash, Cash: cash, Held: map[string]float64{}, Sector: map[string]float64{}}
}

func TestCalendarPicksLastSessionOfPeriod(t *testing.T) {
	if aug.IsRebalanceSession("2026-08-28", ips.RebalanceMonthly) {
		t.Fatal("Aug 28 has a later August session")
	}
	if !aug.IsRebalanceSession("2026-08-31", ips.RebalanceMonthly) {
		t.Fatal("Aug 31 is the last August session")
	}
	if aug.IsRebalanceSession("2026-08-29", ips.RebalanceMonthly) {
		t.Fatal("a Saturday is never a session")
	}
	if aug.IsRebalanceSession("2026-08-31", ips.RebalanceQuarterly) {
		t.Fatal("August is not a quarter end")
	}
	// A holiday on the calendar's edge: Sep 30 2026 is a Wednesday. If the
	// tape shows Sep 29 as the last session and no Sep 30, the calendar
	// cannot know yet, so it falls back to the civil rule for Sep 30.
	sep := CalendarFrom([]string{"2026-09-28", "2026-09-29"})
	if sep.IsRebalanceSession("2026-09-29", ips.RebalanceMonthly) {
		t.Fatal("Sep 29 has a civil weekday after it in September")
	}
	if !sep.IsRebalanceSession("2026-09-30", ips.RebalanceMonthly) || !sep.IsRebalanceSession("2026-09-30", ips.RebalanceQuarterly) {
		t.Fatal("Sep 30 is the last weekday of the month and quarter")
	}
	// A known holiday inside the calendar: if the tape had Sep 28 then
	// Oct 1, Sep 28 is the rebalance session and Sep 30 is not.
	hol := CalendarFrom([]string{"2026-09-28", "2026-10-01"})
	if !hol.IsRebalanceSession("2026-09-28", ips.RebalanceMonthly) || hol.IsRebalanceSession("2026-09-30", ips.RebalanceMonthly) {
		t.Fatal("tape calendar must beat civil weekdays")
	}
	if got := aug.NextRebalance("2026-08-17", ips.RebalanceMonthly); got != "2026-08-31" {
		t.Fatalf("next rebalance %s", got)
	}
}

func TestNonRebalanceDayHasNoTickets(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	in := Input{IPS: p, Now: at("2026-08-18"), Cal: aug, Last: marks(), Snap: cashBook(1_000_000),
		CoreQty: map[string]float64{}, CoreHeld: map[string]float64{}, SatelliteEmpty: true, LastRebalance: "2026-08-17"}
	plan := Rebalance(in)
	if plan.Session || len(plan.Tickets) != 0 {
		t.Fatalf("Aug 18 is not a rebalance session: %+v", plan)
	}
	if plan.NextRebalance != "2026-08-31" || !strings.Contains(plan.Reason, "2026-08-31") {
		t.Fatalf("reason must say when: %s", plan.Reason)
	}
	// Closed market: nothing, even on the rebalance day.
	in.Now = at("2026-08-31").Add(2 * time.Hour)
	if plan := Rebalance(in); plan.Session || len(plan.Tickets) != 0 {
		t.Fatalf("market closed must not ticket: %+v", plan)
	}
}

func TestInitialBuildLegsInUnderTheTurnoverRail(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	in := Input{IPS: p, Now: at("2026-08-17"), Cal: aug, Last: marks(), Snap: cashBook(1_000_000),
		CoreQty: map[string]float64{}, CoreHeld: map[string]float64{}, SatelliteEmpty: true}
	plan := Rebalance(in)
	if !plan.Session || plan.Complete || len(plan.Tickets) == 0 {
		t.Fatalf("first session must start the build: %+v", plan.Reason)
	}
	buys := 0.0
	for _, tk := range plan.Tickets {
		if tk.Side != broker.Buy || tk.Reason != ReasonCoreInitial {
			t.Fatalf("initial build is buys tagged CORE_INITIAL: %+v", tk)
		}
		buys += tk.Notional
	}
	if buys > 1_000_000*costs.TurnoverCapDay+1 {
		t.Fatalf("day one buys %.0f exceed the 10%% turnover rail", buys)
	}
	if math.Abs(plan.Targets.CorePct-0.81) > 1e-6 {
		t.Fatalf("100%% core trims to 81%% under the rails, got %.4f", plan.Targets.CorePct)
	}
}

// Month-end, flat equal book slightly off target: only names outside the
// 100 bps band trade, and every ticket is costed like the lab.
func TestMonthEndTicketsOnlyOutsideTheBand(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	tg := core.Allocate(1.0)
	eq := 1_000_000.0
	last := marks()
	held, qty := map[string]float64{}, map[string]float64{}
	gross := 0.0
	sector := map[string]float64{}
	for s, w := range tg.Core {
		mv := w * eq
		switch s {
		case "TCS":
			mv += 0.02 * eq // 200 bps rich → sell
		case "ITC":
			mv -= 0.015 * eq // 150 bps poor → buy
		case "RELIANCE":
			mv += 0.005 * eq // inside the band → nothing
		}
		held[s] = mv
		qty[s] = mv / last[s]
		gross += mv
		inst, _ := universe.Lookup(s)
		sector[inst.Sector] += mv
	}
	cash := eq - gross
	snap := broker.Snapshot{Equity: eq, Peak: eq, Cash: cash, Gross: gross, Held: held, Sector: sector}
	in := Input{IPS: p, Now: at("2026-08-31"), Cal: aug, Last: last, Snap: snap, CoreQty: qty, CoreHeld: held,
		SatelliteEmpty: true, LastRebalance: "2026-07-31"}
	plan := Rebalance(in)
	if !plan.Session {
		t.Fatal("Aug 31 is the rebalance session")
	}
	got := map[string]broker.Side{}
	for _, tk := range plan.Tickets {
		got[tk.Symbol] = tk.Side
		if tk.Reason != ReasonCoreRebalance {
			t.Fatalf("reason %s", tk.Reason)
		}
	}
	if got["TCS"] != broker.Sell || got["ITC"] != broker.Buy || len(got) != 2 {
		t.Fatalf("band rule: %+v (%s)", got, plan.Reason)
	}
	if plan.Tickets[0].Side != broker.Sell {
		t.Fatal("sells must come first so buys have cash")
	}
	// Same cost function as the lab: the sell's expected fill carries slippage.
	sell := plan.Tickets[0]
	fill, proceeds := costs.SellFill(last["TCS"], sell.Qty, 0)
	if sell.Price != fill || math.Abs(sell.Notional-proceeds) > 1e-6 {
		t.Fatalf("ticket must be priced by pkg/costs: %+v", sell)
	}
	if !plan.Complete {
		t.Fatalf("two small tickets fit inside every rail: %s", plan.Reason)
	}
}

// Synthetic −16% path with max_dd 0.15: the core halts, holds, and emits
// no buys — the same MAX_DRAWDOWN the broker logs for the satellite.
func TestClientDrawdownCapStopsTheCore(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	snap := cashBook(840_000)
	snap.Peak = 1_000_000
	in := Input{IPS: p, Now: at("2026-08-31"), Cal: aug, Last: marks(), Snap: snap,
		CoreQty: map[string]float64{}, CoreHeld: map[string]float64{}, SatelliteEmpty: true}
	plan := Rebalance(in)
	if !plan.Halted || len(plan.Tickets) != 0 || !strings.Contains(plan.Reason, broker.ReasonMaxDrawdown) {
		t.Fatalf("−16%% must halt the core: %+v", plan)
	}
	// A tighter client cap halts sooner than the book's 15%.
	p.MaxDD = 0.10
	snap = cashBook(890_000)
	snap.Peak = 1_000_000
	in.IPS, in.Snap = p, snap
	if plan := Rebalance(in); !plan.Halted {
		t.Fatal("11% drawdown must halt under a 10% cap")
	}
	// The book's 15% is a wall the IPS cannot loosen: MaxDD above it is
	// refused by ips.Validate, and the broker mins it anyway.
	snap.HaltAt = 0.5
	if broker.HaltThreshold(snap) != costs.DrawdownHalt {
		t.Fatal("HaltAt above 15% must not loosen the halt")
	}
}

func TestFoldSatelliteOnlyWhenEmpty(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	base := Input{IPS: p, Now: at("2026-08-17"), Cal: aug, Last: marks(), Snap: cashBook(1_000_000),
		CoreQty: map[string]float64{}, CoreHeld: map[string]float64{}}
	fold := base
	fold.SatelliteEmpty = true
	if got := Rebalance(fold).CoreWanted; got != 1.0 {
		t.Fatalf("empty satellite folds into core: wanted %.2f", got)
	}
	live := base
	live.SatelliteEmpty = false
	if got := Rebalance(live).CoreWanted; got != 0.80 {
		t.Fatalf("a live satellite keeps its slice: wanted %.2f", got)
	}
	p.FoldSatellite = false
	noFold := fold
	noFold.IPS = p
	if got := Rebalance(noFold).CoreWanted; got != 0.80 {
		t.Fatalf("FoldSatellite=false leaves the slice in cash: wanted %.2f", got)
	}
}
