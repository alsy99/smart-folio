package marketdata

import (
	"context"
	"testing"
	"time"

	marketdatav1 "aperture/gen/marketdata/v1"
)

func TestGetQuotesMock(t *testing.T) {
	s := &Service{now: func() time.Time { return time.Date(2024, 6, 3, 10, 0, 0, 0, time.UTC) }}
	resp, err := s.GetQuotes(context.Background(), &marketdatav1.GetQuotesRequest{Symbols: []string{"RELIANCE"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Quotes) != 1 || resp.Quotes[0].Last <= 0 {
		t.Fatalf("%+v", resp.Quotes)
	}
}
