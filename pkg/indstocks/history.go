package indstocks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"

	"aperture/pkg/prices"
)

// dailyMaxRange is the per-call ceiling on /market/historical/1day.
const dailyMaxRange = 365 * 24 * time.Hour

// DailyHistory returns 1-day candles for symbol over [from, to], walking the
// API one-year window at a time (the documented per-call maximum). Works
// for equities and for the mapped benchmark indices (NIFTY50, SENSEX, …),
// which Bars refuses because they are not quoteable as instruments.
func (c *Client) DailyHistory(ctx context.Context, symbol string, from, to time.Time) ([]prices.Bar, error) {
	if err := c.EnsureScrips(ctx); err != nil {
		return nil, err
	}
	s, ok := c.scrip(symbol)
	if !ok {
		return nil, fmt.Errorf("indstocks: no scrip for %s", symbol)
	}
	if !to.After(from) {
		return nil, fmt.Errorf("indstocks: empty window %s..%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	byTs := map[int64]prices.Bar{}
	for end := to; end.After(from); {
		start := end.Add(-dailyMaxRange)
		if start.Before(from) {
			start = from
		}
		q := url.Values{
			"scrip-codes": {s.Code},
			"start_time":  {fmt.Sprintf("%d", start.UnixMilli())},
			"end_time":    {fmt.Sprintf("%d", end.UnixMilli())},
		}
		b, err := c.get(ctx, "/market/historical/1day", q)
		if err != nil {
			return nil, err
		}
		env, data, err := decodeEnvelope(b)
		if err != nil {
			return nil, err
		}
		if !env.ok() {
			return nil, fmt.Errorf("indstocks: %s", env.err())
		}
		var wrap map[string]struct {
			Candles []candle `json:"candles"`
		}
		if err := json.Unmarshal(data, &wrap); err != nil {
			return nil, err
		}
		for _, row := range wrap {
			for _, cd := range row.Candles {
				if cd.C <= 0 {
					continue
				}
				byTs[cd.Ts] = prices.Bar{
					Ts: time.Unix(cd.Ts, 0), Open: cd.O, High: cd.H, Low: cd.L, Close: cd.C, Volume: cd.V,
				}
			}
		}
		end = start.Add(-time.Second)
	}
	out := make([]prices.Bar, 0, len(byTs))
	for _, b := range byTs {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	if len(out) == 0 {
		return nil, fmt.Errorf("indstocks: no daily candles for %s in %s..%s", symbol, from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	return out, nil
}
