package strategies

import "testing"

func TestMomentumRegistry(t *testing.T) {
	spec := Parse(Momentum1d)
	closes := []float64{100, 100.2, 100.4, 101.2, 102, 103, 104, 105, 106, 108, 110}
	sig := Evaluate(spec, "TCS", Window{Close: closes}, 0)
	if sig.Direction <= 0 {
		t.Fatalf("expected long momentum, got %+v", sig)
	}
	if sig.StrategyID != Momentum1d {
		t.Fatalf("id %s", sig.StrategyID)
	}
}

func TestUnknownMethodIsFlat(t *testing.T) {
	sig := Evaluate(Spec{ID: "x", Method: "nope"}, "INFY", Window{}, 0)
	if sig.Direction != 0 {
		t.Fatalf("got %+v", sig)
	}
}

func TestParseDefaultSpec(t *testing.T) {
	s := Parse(SMACross1d)
	if s.Method != "sma_cross" || s.Fast != 10 || s.Timeframe != "1d" {
		t.Fatalf("got %+v", s)
	}
}

func TestIDHoldsUnderSession(t *testing.T) {
	if HoldsUnderSession("1d") || IDHoldsUnderSession(Momentum1d) {
		t.Fatal("daily roster must hold a session")
	}
	if !IDHoldsUnderSession("momentum_5m") || !HoldsUnderSession("15m") {
		t.Fatal("intraday ids must be flagged")
	}
}
