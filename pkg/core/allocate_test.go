package core

import (
	"math"
	"strings"
	"testing"

	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/universe"
)

func TestAllocateIsEqualWeightInsideTheRails(t *testing.T) {
	for _, corePct := range []float64{0.80, 0.90, 1.00} {
		tg := Allocate(corePct)
		if len(tg.Core) != 12 {
			t.Fatalf("core must hold the 12 campaign names, got %d", len(tg.Core))
		}
		sector := map[string]float64{}
		sum := 0.0
		for s, w := range tg.Core {
			if w > costs.NameCap+1e-9 {
				t.Fatalf("%s %.4f breaks the name cap", s, w)
			}
			inst, _ := universe.Lookup(s)
			sector[inst.Sector] += w
			sum += w
		}
		for sec, w := range sector {
			if w > broker.SectorCap+1e-9 {
				t.Fatalf("%s %.4f breaks the sector cap", sec, w)
			}
		}
		if sum > broker.GrossCap+1e-9 || math.Abs(sum-tg.CorePct) > 1e-4 || math.Abs(tg.Cash-(1-sum)) > 1e-4 {
			t.Fatalf("core %.2f: sum %.4f pct %.4f cash %.4f", corePct, sum, tg.CorePct, tg.Cash)
		}
		if tg.CorePct > corePct+1e-9 {
			t.Fatal("rails may only trim, never add")
		}
		// Non-bank names are equal to each other; banks are equal to each other.
		if tg.Core["RELIANCE"] != tg.Core["TCS"] || tg.Core["HDFCBANK"] != tg.Core["SBIN"] {
			t.Fatalf("mix must stay equal weight within a rail class: %+v", tg.Core)
		}
	}
	// 100% core cannot be 100% invested: 12 × 8.33% breaks the 8% name cap
	// and five banks × 5% is the 25% sector wall. The honest number is 81%.
	tg := Allocate(1.0)
	if math.Abs(tg.CorePct-0.81) > 1e-6 {
		t.Fatalf("core 100%% must trim to 81%%, got %.4f (%s)", tg.CorePct, tg.Reason)
	}
	if !strings.Contains(tg.Reason, broker.ReasonNameCap) || !strings.Contains(tg.Reason, broker.ReasonSectorCap) {
		t.Fatalf("reason must name the rails: %s", tg.Reason)
	}
	if Allocate(0).CorePct != 0 {
		t.Fatal("core 0 holds nothing")
	}
}

func TestDriftsHonourTheBands(t *testing.T) {
	tg := Allocate(1.0)
	eq := 1_000_000.0
	held := map[string]float64{}
	for s, w := range tg.Core {
		held[s] = w * eq
	}
	cash := eq * tg.Cash
	// On target: no tickets.
	ds, cashOff := Drifts(tg, held, cash, eq)
	for _, d := range ds {
		if d.Ticket {
			t.Fatalf("%s on target must not ticket: %+v", d.Symbol, d)
		}
	}
	if math.Abs(cashOff) > 1e-9 {
		t.Fatalf("cash off %.6f", cashOff)
	}
	// One name 150 bps rich, another 50 bps poor: only the first trades.
	held["TCS"] += 0.015 * eq
	held["ITC"] -= 0.005 * eq
	cash -= 0.010 * eq
	ds, _ = Drifts(tg, held, cash, eq)
	got := map[string]bool{}
	for _, d := range ds {
		got[d.Symbol] = d.Ticket
	}
	if !got["TCS"] || got["ITC"] || got["RELIANCE"] {
		t.Fatalf("band rule: %+v", got)
	}
	// Cash 3% over target (fresh money): every off-target name tickets.
	cash = eq*tg.Cash + 0.03*eq
	for s := range held {
		held[s] -= 0.0025 * eq
	}
	ds, cashOff = Drifts(tg, held, cash, eq)
	if math.Abs(cashOff) < CashBand {
		t.Fatalf("cash off should exceed the band, got %.4f", cashOff)
	}
	for _, d := range ds {
		if !d.Ticket {
			t.Fatalf("wide cash drift must ticket every off-target name, %s did not", d.Symbol)
		}
	}
	// Held but not a target is a sell.
	held["ZZZ"] = 0.02 * eq
	ds, _ = Drifts(tg, held, cash, eq)
	found := false
	for _, d := range ds {
		if d.Symbol == "ZZZ" {
			found = d.Ticket && d.Target == 0 && d.Drift > 0
		}
	}
	if !found {
		t.Fatal("a non-target holding must be marked for sale")
	}
}
