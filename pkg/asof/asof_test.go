package asof

import (
	"testing"
	"time"

	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/marketclock"
)

func fridaySession(hour, min int) time.Time {
	loc := marketclock.Location()
	// 11 Sep 2026 is a Friday.
	return time.Date(2026, 9, 11, hour, min, 0, 0, loc)
}

func TestRejectHeadlineAfterBar(t *testing.T) {
	bar := fridaySession(15, 25)
	late := fridaySession(16, 0)
	if KnownAt(late, bar) {
		t.Fatal("Friday 16:00 must not be known at the 15:25 bar")
	}
	if !KnownAt(fridaySession(15, 0), bar) {
		t.Fatal("15:00 headline must be known at 15:25")
	}
	if KnownAt(time.Time{}, bar) {
		t.Fatal("missing published_at is not trusted")
	}
}

func TestNewsFilterDropsAfterHoursPrint(t *testing.T) {
	bar := fridaySession(15, 25)
	kept := News([]*commonv1.NewsItem{
		{Id: "late", Title: "after close", PublishedAtUnixMs: fridaySession(16, 0).UnixMilli()},
		{Id: "early", Title: "in session", PublishedAtUnixMs: fridaySession(14, 0).UnixMilli()},
	}, bar)
	if len(kept) != 1 || kept[0].Id != "early" {
		t.Fatalf("kept %+v", kept)
	}
}

func TestInvestigationWithLateSourceIsDropped(t *testing.T) {
	bar := fridaySession(15, 25)
	r := &commonv1.InvestigationReport{
		Id: "inv-late", Headline: "TCS beats after close", Symbols: []string{"TCS"},
		Score: 0.9, Confidence: 0.8, StandAside: true,
		Sources: []*commonv1.NewsItem{{
			Id: "a", Title: "TCS beats", PublishedAtUnixMs: fridaySession(16, 0).UnixMilli(),
		}},
	}
	if ReportKnown(r, bar) {
		t.Fatal("16:00 source must reject the investigation at 15:25")
	}
	if len(Reports([]*commonv1.InvestigationReport{r}, bar)) != 0 {
		t.Fatal("late investigation leaked into as-of set")
	}
}

func TestMixedClusterDropsIfAnySourceIsLate(t *testing.T) {
	bar := fridaySession(15, 25)
	r := &commonv1.InvestigationReport{
		Id: "inv-mix", Symbols: []string{"TCS"}, Score: 0.9, StandAside: true,
		Sources: []*commonv1.NewsItem{
			{Id: "am", PublishedAtUnixMs: fridaySession(11, 0).UnixMilli()},
			{Id: "pm", PublishedAtUnixMs: fridaySession(16, 0).UnixMilli()},
		},
	}
	if ReportKnown(r, bar) {
		t.Fatal("a 16:00 follow-up must poison the clustered report at 15:25")
	}
}
