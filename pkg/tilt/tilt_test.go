package tilt

import (
	"math"
	"testing"

	commonv1 "aperture/gen/common/v1"
)

func TestScoreCountsOnlyLLMReports(t *testing.T) {
	cases := []struct {
		name string
		in   []*commonv1.InvestigationReport
		want float64
	}{
		{"none", nil, 0},
		{"heuristic only", []*commonv1.InvestigationReport{{Score: -0.9, Confidence: 0.9, ResearchMode: "heuristic"}}, 0},
		{"llm", []*commonv1.InvestigationReport{{Score: -0.8, Confidence: 0.5, ResearchMode: "llm"}}, -0.8},
		{"llm-text counts", []*commonv1.InvestigationReport{{Score: 0.4, Confidence: 0.5, ResearchMode: "llm-text"}}, 0.4},
		{"weighted", []*commonv1.InvestigationReport{
			{Score: 1, Confidence: 0.75, ResearchMode: "llm"},
			{Score: -1, Confidence: 0.25, ResearchMode: "llm"},
			{Score: -1, Confidence: 0.9, ResearchMode: "heuristic"},
		}, 0.5},
		{"zero confidence ignored", []*commonv1.InvestigationReport{{Score: 1, Confidence: 0, ResearchMode: "llm"}}, 0},
	}
	for _, c := range cases {
		if got := Score(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestEffectiveClipsBothWays(t *testing.T) {
	cases := []struct {
		target, max, score, want float64
	}{
		{0.20, 0.20, 0, 0.20},
		{0.20, 0.20, -0.9, 0.15},       // clip(-0.9) = -0.25 → 20% × 0.75
		{0.20, 0.20, -0.10, 0.18},      // inside the cap
		{0.20, 0.20, +0.9, 0.20},       // bullish cannot exceed SatellitePct
		{0.10, 0.20, +0.9, 0.125},      // room below the cap: 10% × 1.25
		{0.20, 0.20, -5, 0.15},         // wild score still ±25%
		{0, 0.20, -0.5, 0},             // empty target stays empty
		{0.20, 0, 0.5, 0},              // 100% core: no satellite ever
		{0.20, 0.20, math.NaN(), 0.20}, // a broken score is no tilt, not an empty sleeve
	}
	for _, c := range cases {
		got := Effective(c.target, c.max, c.score)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("Effective(%v,%v,%v)=%v want %v", c.target, c.max, c.score, got, c.want)
		}
	}
}
