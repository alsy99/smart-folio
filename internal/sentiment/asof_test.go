package sentiment

import (
	"context"
	"testing"
	"time"

	"aperture/pkg/marketclock"
)

type timedSource struct {
	articles []article
}

func (t timedSource) Name() string { return "timed" }
func (t timedSource) Fetch(context.Context) ([]article, error) {
	return t.articles, nil
}

func TestGatherDropsHeadlinesAfterBar(t *testing.T) {
	loc := marketclock.Location()
	bar := time.Date(2026, 9, 11, 15, 25, 0, 0, loc)
	src := timedSource{articles: []article{
		{ID: "late", Title: "after close", Published: time.Date(2026, 9, 11, 16, 0, 0, 0, loc), Symbols: []string{"TCS"}},
		{ID: "early", Title: "TCS wins mandate", Published: time.Date(2026, 9, 11, 14, 0, 0, 0, loc), Symbols: []string{"TCS"}},
	}}
	got, mode := gather(context.Background(), []Source{src}, bar)
	if mode != "live" {
		t.Fatalf("mode %s", mode)
	}
	if len(got) != 1 || got[0].ID != "early" {
		t.Fatalf("late headline leaked: %+v", got)
	}
}
