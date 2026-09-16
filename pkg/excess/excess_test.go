package excess

import "testing"

func almost(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func TestPortfolioReturn(t *testing.T) {
	if !almost(PortfolioReturn(110, 100), 0.1) {
		t.Fatalf("got %v", PortfolioReturn(110, 100))
	}
	if got := PortfolioReturn(50, 0); got != 0 {
		t.Fatalf("zero start: %v", got)
	}
}

func TestOnTrack(t *testing.T) {
	if !OnTrack(0.10) {
		t.Fatal("10pp should be on track")
	}
	if OnTrack(0.09) {
		t.Fatal("9pp should be off pace")
	}
}

func TestExcessAndAnnualized(t *testing.T) {
	if !almost(Excess(0.12, 0.02), 0.10) {
		t.Fatalf("got %v", Excess(0.12, 0.02))
	}
	if got := Annualized(0, 30); got != 0 {
		t.Fatalf("got %v", got)
	}
	if got := Annualized(0.01, 0.04); got != 0 {
		t.Fatalf("day-0 noise must not annualise: %v", got)
	}
	if got := Annualized(0.01, 0); got != 0 {
		t.Fatalf("zero days: %v", got)
	}
	if Annualized(0.10, 365) == 0 {
		t.Fatal("a full year of 10% excess should annualise")
	}
}
