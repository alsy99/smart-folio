package ips

import (
	"errors"
	"testing"
)

func good() IPS { return Default("c-1", 1_000_000) }

func TestDefaultIsValid(t *testing.T) {
	if err := good().Validate(); err != nil {
		t.Fatal(err)
	}
	a := A(1_000_000)
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.ID != IDA {
		t.Fatalf("IPS A id %s", a.ID)
	}
}

func TestValidateRejectsOutOfWalls(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*IPS)
		want error
	}{
		{"max_dd 0.20", func(p *IPS) { p.MaxDD = 0.20 }, ErrMaxDD},
		{"max_dd 0", func(p *IPS) { p.MaxDD = 0 }, ErrMaxDD},
		{"core 0.5", func(p *IPS) { p.CorePct = 0.5; p.SatellitePct = 0.5 }, ErrCorePct},
		{"core 0.79", func(p *IPS) { p.CorePct = 0.79; p.SatellitePct = 0.21 }, ErrCorePct},
		{"core 1.01", func(p *IPS) { p.CorePct = 1.01; p.SatellitePct = -0.01 }, ErrCorePct},
		{"split drift", func(p *IPS) { p.CorePct = 0.9; p.SatellitePct = 0.05 }, ErrSplit},
		{"missing benchmark", func(p *IPS) { p.Benchmark = "" }, ErrBenchmark},
		{"unknown benchmark", func(p *IPS) { p.Benchmark = "DOW" }, ErrBenchmark},
		{"horizon 2", func(p *IPS) { p.HorizonYears = 2 }, ErrHorizon},
		{"goal empty", func(p *IPS) { p.Goal = "" }, ErrGoal},
		{"goal free text", func(p *IPS) { p.Goal = "make 10 crore" }, ErrFreeText},
		{"goal promise", func(p *IPS) { p.Goal = "guaranteed 10%" }, ErrPromissory},
		{"goal 10% a month", func(p *IPS) { p.Goal = "10% a month" }, ErrPromissory},
		{"rebalance weekly", func(p *IPS) { p.Rebalance = "weekly" }, ErrRebalance},
		{"cash 0", func(p *IPS) { p.StartCash = 0 }, ErrStartCash},
		{"no id", func(p *IPS) { p.ID = " " }, ErrID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := good()
			c.mut(&p)
			err := p.Validate()
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	// Satellite 20% is the cap; 80/20 is valid, 79/21 is not (core wall).
	p := good()
	p.CorePct, p.SatellitePct = 0.80, 0.20
	if err := p.Validate(); err != nil {
		t.Fatalf("80/20 must validate: %v", err)
	}
}

func TestBenchmarkMustExistOnTape(t *testing.T) {
	p := good()
	p.Benchmark = BenchNifty500
	onlyNifty := func(b Benchmark) bool { return b == BenchNifty50 }
	if err := p.ValidateOn(onlyNifty); !errors.Is(err, ErrNoSeries) {
		t.Fatalf("want ErrNoSeries, got %v", err)
	}
	p.Benchmark = BenchNifty50
	if err := p.ValidateOn(onlyNifty); err != nil {
		t.Fatal(err)
	}
}

func TestHashIsStableAndFieldSensitive(t *testing.T) {
	a, b := good(), good()
	if a.Hash() != b.Hash() || a.Hash() == "" {
		t.Fatal("same statement must hash the same")
	}
	// Pinned: a restart, another machine, another year must agree.
	if got := good().Hash(); got != "ed41b6b74ba85bd0bb07ead8" {
		t.Fatalf("default IPS hash drifted to %q; a frozen manifest would no longer verify", got)
	}
	b.MaxDD = 0.10
	if a.Hash() == b.Hash() {
		t.Fatal("max dd must move the hash")
	}
	c := good()
	c.ID = "c-2"
	if a.Hash() == c.Hash() {
		t.Fatal("two clients with identical settings must not share a hash")
	}
}
