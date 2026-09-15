package asof

import (
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/marketclock"
)

func TestLateHeadlineDoesNotChangeAsOfScores(t *testing.T) {
	bar := time.Date(2026, 9, 11, 15, 25, 0, 0, marketclock.Location())
	late := time.Date(2026, 9, 11, 16, 0, 0, 0, marketclock.Location())
	base := []*commonv1.InvestigationReport{{
		Id: "am", Symbols: []string{"TCS"}, Score: 0.2, Confidence: 1,
		Sources: []*commonv1.NewsItem{{PublishedAtUnixMs: time.Date(2026, 9, 11, 11, 0, 0, 0, marketclock.Location()).UnixMilli()}},
	}}
	withLate := append(append([]*commonv1.InvestigationReport{}, base...), &commonv1.InvestigationReport{
		Id: "pm", Symbols: []string{"TCS"}, Score: 0.9, Confidence: 1, StandAside: true,
		Sources: []*commonv1.NewsItem{{PublishedAtUnixMs: late.UnixMilli()}},
	})
	a := Scores(Reports(base, bar))
	b := Scores(Reports(withLate, bar))
	if a["TCS"] != b["TCS"] {
		t.Fatalf("16:00 print moved 15:25 score: %.4f -> %.4f", a["TCS"], b["TCS"])
	}
}
