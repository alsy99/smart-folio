package learning

import (
	"testing"

	commonv1 "aperture/gen/common/v1"
)

func TestAttributeWinWithExcess(t *testing.T) {
	lesson, tags := Attribute(&commonv1.PaperTrade{
		StrategyId: "momentum_5m", Pnl: 1200, ExcessReturn: 0.4,
	})
	if lesson == "" {
		t.Fatal("empty lesson")
	}
	if !contains(tags, "win") || !contains(tags, "helped_vs_nifty") {
		t.Fatalf("tags %v", tags)
	}
}

func TestAttributeLoss(t *testing.T) {
	_, tags := Attribute(&commonv1.PaperTrade{Pnl: -10, ExcessReturn: -1})
	if !contains(tags, "loss") || !contains(tags, "hurt_vs_nifty") {
		t.Fatalf("tags %v", tags)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
