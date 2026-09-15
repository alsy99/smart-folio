package ta

import (
	"testing"
	"time"

	"aperture/pkg/prices"
)

func TestFromBarsTrendFields(t *testing.T) {
	now := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	bars := prices.Bars("TCS", "1d", 40, now)
	s := FromBars("TCS", bars)
	if s.Last == 0 {
		t.Fatal("expected last")
	}
	if s.Trend == "" || s.Setup == "" {
		t.Fatalf("trend/setup empty: %+v", s)
	}
	if s.Summary() == "" {
		t.Fatal("summary")
	}
}
