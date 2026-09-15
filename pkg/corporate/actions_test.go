package corporate

import (
	"testing"
	"time"

	"aperture/pkg/marketclock"
)

func TestActionAfterBarIsRejected(t *testing.T) {
	bar := time.Date(2021, 1, 1, 15, 25, 0, 0, marketclock.Location())
	if got := At("ITC", bar); len(got) != 0 {
		t.Fatalf("2023 ITC demerger must not be known in 2021, got %+v", got)
	}
	after := time.Date(2024, 6, 1, 15, 25, 0, 0, marketclock.Location())
	got := At("ITC", after)
	if len(got) != 1 || !got[0].Effective(after) {
		t.Fatalf("expected effective ITC demerger, got %+v", got)
	}
}
