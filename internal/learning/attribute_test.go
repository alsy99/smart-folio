package learning

import (
	"testing"

	commonv1 "aperture/gen/common/v1"
)

func TestAttributeWinWithExcess(t *testing.T) {
	lesson, tags := Attribute(&commonv1.PaperTrade{
		StrategyId: "momentum_1d", Pnl: 1200, ExcessReturn: 0.4,
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

func TestAttributeKeepsLearnFacts(t *testing.T) {
	_, tags := Attribute(&commonv1.PaperTrade{
		Pnl: -10, ExcessReturn: -1,
		AttributionTags: []string{"mae=-0.02", "mfe=0.01", "regime=chop", "hold_ms=1000"},
	})
	if !contains(tags, "mae=-0.02") || !contains(tags, "regime=chop") || !contains(tags, "loss") {
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
