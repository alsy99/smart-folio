package strategies

import "testing"

func TestMomentumRegistry(t *testing.T) {
	spec := Parse(Momentum5m)
	closes := []float64{100, 100.2, 100.4, 101.2}
	sig := Evaluate(spec, "TCS", Window{Close: closes}, 0)
	if sig.Direction <= 0 {
		t.Fatalf("expected long momentum, got %+v", sig)
	}
	if sig.StrategyID != Momentum5m {
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
	s := Parse(SMACross15m)
	if s.Method != "sma_cross" || s.Fast != 5 {
		t.Fatalf("got %+v", s)
	}
}
