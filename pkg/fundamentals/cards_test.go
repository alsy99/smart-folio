package fundamentals

import (
	"testing"
	"time"
)

func TestCardRevisionAfterBarIsRejected(t *testing.T) {
	before := time.Date(2020, 6, 1, 15, 25, 0, 0, time.UTC)
	c := LookupAsOf("TCS", before)
	if c.Model != "No fundamental card as-of this bar." {
		t.Fatalf("2024 revision must not apply on a 2020 bar: %+v", c)
	}
	after := time.Date(2025, 6, 1, 15, 25, 0, 0, time.UTC)
	c = LookupAsOf("TCS", after)
	if c.Model == "" || c.Model == "No fundamental card as-of this bar." {
		t.Fatalf("current card should be known in 2025: %+v", c)
	}
}
