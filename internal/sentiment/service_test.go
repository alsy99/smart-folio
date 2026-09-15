package sentiment

import (
	"context"
	"log/slog"
	"testing"
	"time"

	sentimentv1 "aperture/gen/sentiment/v1"
)

type countSource struct {
	n int
}

func (c *countSource) Name() string { return "stub" }

func (c *countSource) Fetch(context.Context) ([]article, error) {
	c.n++
	return []article{{
		ID: "1", Provider: "stub", Title: "TCS wins multi-year mandate",
		Summary: "Deal size raised.", Published: time.Now(), Symbols: []string{"TCS"},
	}}, nil
}

type nopLLM struct{}

func (nopLLM) Enabled() bool { return false }
func (nopLLM) Complete(context.Context, string, string) (string, error) {
	return "", nil
}

func TestForceRefreshIgnoresCache(t *testing.T) {
	src := &countSource{}
	svc := New(Config{RefreshEvery: time.Hour, MaxWorkers: 2, MinMateriality: 0}, slog.Default(), nopLLM{}, []Source{src})
	ctx := context.Background()
	svc.Refresh(ctx, false)
	if src.n != 1 {
		t.Fatalf("first fetch: got %d", src.n)
	}
	svc.Refresh(ctx, false)
	if src.n != 1 {
		t.Fatalf("cached fetch: got %d", src.n)
	}
	svc.Refresh(ctx, true)
	if src.n != 2 {
		t.Fatalf("forced fetch: got %d", src.n)
	}
	if _, err := svc.ListNews(ctx, &sentimentv1.ListNewsRequest{Limit: -1}); err != nil {
		t.Fatal(err)
	}
	if src.n != 3 {
		t.Fatalf("campaign-style ListNews: got %d", src.n)
	}
}
