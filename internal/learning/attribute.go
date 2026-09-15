package learning

import (
	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/learn"
)

// Attribute is a journal caption. It does not change strategy weights.
func Attribute(t *commonv1.PaperTrade) (string, []string) {
	facts := learn.KeepFactTags(t.AttributionTags)
	var tags []string
	if t.Pnl > 0 {
		tags = append(tags, "win")
	} else {
		tags = append(tags, "loss")
	}
	if t.ExcessReturn > 0 {
		tags = append(tags, "helped_vs_nifty")
	} else {
		tags = append(tags, "hurt_vs_nifty")
	}
	if len(t.InvestigationIds) > 0 {
		tags = append(tags, "investigation_linked")
	}
	lesson := ""
	switch {
	case t.Pnl > 0 && t.ExcessReturn > 0:
		lesson = t.StrategyId + " captured a move that outpaced Nifty — keep an eye on the regime at the weekly review."
	case t.Pnl <= 0 && t.ExcessReturn <= 0:
		lesson = t.StrategyId + " lagged Nifty on this name. Stored for the weekly review; a single fill does not retrain."
	case t.Pnl > 0:
		lesson = "Absolute profit but no excess vs Nifty — size down versus the index, not just P&L."
	default:
		lesson = "Loss in rupees but relative tape was not the issue; review stop placement."
	}
	return lesson, append(facts, tags...)
}
