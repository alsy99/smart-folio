package learn

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"aperture/pkg/strategies"
)

const (
	MinN        = 20
	Shrink      = 0.5
	MinWeight   = 0.05
	ReviewEvery = 7 * 24 * time.Hour

	ClosesFile  = "closes.jsonl"
	WeightsFile = "weights.json"
	BackupDir   = "backup"
)

// Close is one finished round-trip. Written on every fill close; weights
// never read this file except on the weekly job.
type Close struct {
	ID         string  `json:"id"`
	StrategyID string  `json:"strategyId"`
	Method     string  `json:"method"`
	Regime     string  `json:"regime"`
	Symbol     string  `json:"symbol"`
	PnL        float64 `json:"pnl"`    // ₹ after delivery costs
	Excess     float64 `json:"excess"` // vs Nifty, after costs, percent
	HoldMs     int64   `json:"holdMs"`
	MAE        float64 `json:"mae"` // max adverse excursion, fraction of entry
	MFE        float64 `json:"mfe"` // max favorable excursion, fraction of entry
	ClosedAtMs int64   `json:"closedAtUnixMs"`
	Lesson     string  `json:"lesson,omitempty"`
}

func Tag(c Close) string {
	r := c.Regime
	if r == "" {
		r = "chop"
	}
	id := c.StrategyID
	if id == "" {
		id = "?"
	}
	return id + "|" + r
}

func MethodOf(strategyID string) string {
	m := strategies.Parse(strategyID).Method
	if m == "" {
		return "unknown"
	}
	return m
}

// Regime tags the tape at close from Nifty's campaign return in percent.
// It is a market state, not the trade's own P&L.
func Regime(niftyPct float64) string {
	switch {
	case niftyPct >= 1:
		return "bull"
	case niftyPct <= -1:
		return "bear"
	default:
		return "chop"
	}
}

func NewClose(id, strategyID, symbol string, pnl, excess float64, holdMs int64, mae, mfe float64, regime string, closedAtMs int64, lesson string) Close {
	if regime == "" {
		regime = "chop"
	}
	if holdMs < 0 {
		holdMs = 0
	}
	return Close{
		ID: id, StrategyID: strategyID, Method: MethodOf(strategyID),
		Regime: regime, Symbol: symbol, PnL: pnl, Excess: excess,
		HoldMs: holdMs, MAE: mae, MFE: mfe, ClosedAtMs: closedAtMs, Lesson: lesson,
	}
}

func FactTags(mae, mfe float64, regime string, holdMs int64) []string {
	if regime == "" {
		regime = "chop"
	}
	return []string{
		fmt.Sprintf("mae=%.6f", mae),
		fmt.Sprintf("mfe=%.6f", mfe),
		"regime=" + regime,
		fmt.Sprintf("hold_ms=%d", holdMs),
	}
}

func KeepFactTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		if isFactTag(t) {
			out = append(out, t)
		}
	}
	return out
}

func isFactTag(t string) bool {
	return strings.HasPrefix(t, "mae=") || strings.HasPrefix(t, "mfe=") ||
		strings.HasPrefix(t, "regime=") || strings.HasPrefix(t, "hold_ms=")
}

func ParseFacts(tags []string) (mae, mfe float64, regime string, holdMs int64) {
	regime = "chop"
	for _, t := range tags {
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		switch k {
		case "mae":
			mae, _ = strconv.ParseFloat(v, 64)
		case "mfe":
			mfe, _ = strconv.ParseFloat(v, 64)
		case "regime":
			if v != "" {
				regime = v
			}
		case "hold_ms":
			holdMs, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	return
}
