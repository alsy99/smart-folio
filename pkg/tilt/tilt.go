// Package tilt sizes the satellite sleeve from as-of sentiment.
//
//	sat_effective = sat_target * (1 + clip(score, -Cap, +Cap))
//	sat_effective = clip(sat_effective, 0, ips.SatellitePct)
//
// Only the satellite is tilted; the core never sees a score. Only reports
// whose thesis came from the LLM (research_mode llm*) carry a score, so
// with the LLM off, over budget, or the roster empty the tilt is zero and
// the ledger is byte-identical to a run without sentiment.
package tilt

import (
	"math"
	"strings"

	commonv1 "aperture/gen/common/v1"
)

// Cap bounds the tilt at ±25% of the target slice.
const Cap = 0.25

// LLMMode reports whether a report's thesis came from the LLM path.
func LLMMode(mode string) bool {
	return strings.HasPrefix(mode, "llm")
}

// Score is the confidence-weighted mean score of LLM-mode reports. The
// caller has already filtered the reports to those known at the bar
// (pkg/asof). No LLM-mode report → 0.
func Score(reports []*commonv1.InvestigationReport) float64 {
	sum, conf := 0.0, 0.0
	for _, r := range reports {
		if r == nil || !LLMMode(r.ResearchMode) || r.Confidence <= 0 {
			continue
		}
		sum += r.Score * r.Confidence
		conf += r.Confidence
	}
	if conf == 0 {
		return 0
	}
	return sum / conf
}

// Clip bounds a score to ±Cap.
func Clip(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return math.Max(-Cap, math.Min(Cap, score))
}

// Effective is the tilted satellite slice as a fraction of equity. target
// is the slice the book is working toward and maxPct the IPS SatellitePct;
// the result never exceeds maxPct and never goes negative, so a bullish
// score cannot grow the satellite past the client's cap.
func Effective(target, maxPct, score float64) float64 {
	eff := target * (1 + Clip(score))
	return math.Max(0, math.Min(maxPct, eff))
}
