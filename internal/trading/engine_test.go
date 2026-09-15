package trading

import (
	"testing"
	"time"

	"aperture/pkg/costs"
	"aperture/pkg/strategies"
)

func TestDefaultRosterHoldsASession(t *testing.T) {
	for _, spec := range strategies.DefaultSpecs() {
		if strategies.HoldsUnderSession(spec.Timeframe) {
			t.Fatalf("%s timeframe %s holds under a session", spec.ID, spec.Timeframe)
		}
	}
	for _, id := range strategies.IDs() {
		if strategies.HoldsUnderSession(strategies.Parse(id).Timeframe) {
			t.Fatalf("shipped id %s", id)
		}
	}
}

func TestPositionalExitNeedsASession(t *testing.T) {
	if shouldExit(false, 45*time.Second, 0.05, 1) {
		t.Fatal("must not close on the 45s scalp path")
	}
	if shouldExit(false, costs.SessionHold-time.Second, -0.02, 0) {
		t.Fatal("must hold through the session even if the signal flipped")
	}
	if !shouldExit(false, costs.SessionHold, 0.01, 0) {
		t.Fatal("after a session, flat/short signal should exit")
	}
	if shouldExit(false, costs.SessionHold, 0.01, 1) {
		t.Fatal("still long after a session — keep the name")
	}
	if !shouldExit(false, costs.MaxHold, 0.01, 1) {
		t.Fatal("weeks-long cap should recycle the name")
	}
}

func TestScalpExitPath(t *testing.T) {
	if !shouldExit(true, costs.ScalpMaxHold+time.Second, 0, 1) {
		t.Fatal("45s")
	}
	if !shouldExit(true, time.Second, 0.013, 1) {
		t.Fatal("take")
	}
	if !shouldExit(true, time.Second, -0.009, 1) {
		t.Fatal("stop")
	}
	if shouldExit(true, time.Second, 0.001, 1) {
		t.Fatal("hold")
	}
}

func TestNameCapAndDrawdown(t *testing.T) {
	if got := costs.NameRoom(1_000_000, 0); got != 80_000 {
		t.Fatalf("room %v", got)
	}
	if got := costs.NameRoom(1_000_000, 80_000); got != 0 {
		t.Fatalf("full name %v", got)
	}
	if costs.BookHalted(900_000, 1_000_000) {
		t.Fatal("10% is not a halt")
	}
	if !costs.BookHalted(850_000, 1_000_000) {
		t.Fatal("15% book drawdown should halt")
	}
}

func TestSkipSubSessionUnlessScalp(t *testing.T) {
	if !skipSubSession("momentum_5m") {
		t.Fatal("5m should be off the shipped roster")
	}
	t.Setenv("SCALP_MODE", "true")
	if skipSubSession("momentum_5m") {
		t.Fatal("SCALP_MODE may keep a short frame")
	}
}
