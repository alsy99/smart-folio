package learning

import commonv1 "aperture/gen/common/v1"

func Attribute(t *commonv1.PaperTrade) (string, []string) {
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
		lesson = t.StrategyId + " captured a move that outpaced Nifty — keep weight if regime persists."
	case t.Pnl <= 0 && t.ExcessReturn <= 0:
		lesson = t.StrategyId + " lagged Nifty on this name; downweight until expectancy recovers."
	case t.Pnl > 0:
		lesson = "Absolute profit but no excess vs Nifty — size down versus the index, not just P&L."
	default:
		lesson = "Loss in rupees but relative tape was not the issue; review stop placement."
	}
	return lesson, tags
}
