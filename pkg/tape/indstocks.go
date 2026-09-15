// Package tape gives the method lab real daily closes. The walk-forward
// engine in pkg/backtest is tape-agnostic; this package is the only place
// that knows how to turn INDstocks history into aligned close series and
// cache them on disk so a rerun is cheap and a stranger can diff the input.
package tape

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aperture/pkg/backtest"
	"aperture/pkg/indstocks"
	"aperture/pkg/marketclock"
	"aperture/pkg/prices"
)

// NameINDstocks is the tape name recorded on reports and roster snapshots.
const NameINDstocks = "indstocks-1d"

// DefaultDir is where daily closes are cached (one JSON per symbol).
const DefaultDir = "data/bars/1d"

// History is the slice of the INDstocks client the tape needs.
type History interface {
	DailyHistory(ctx context.Context, symbol string, from, to time.Time) ([]prices.Bar, error)
}

// INDstocks is a disk-cached daily tape. Days are NIFTY50's candle dates,
// which is the exchange calendar (holidays fall out for free); every equity
// is aligned to those dates and forward-filled across a missing print.
type INDstocks struct {
	Client History
	Dir    string
	Ctx    context.Context
	// Refresh forces a fetch even when the cache already covers the window.
	Refresh bool
}

type cached struct {
	Symbol  string       `json:"symbol"`
	Source  string       `json:"source"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Fetched string       `json:"fetchedAt"`
	Bars    []prices.Bar `json:"bars"`
}

func (t *INDstocks) Name() string { return NameINDstocks }

func (t *INDstocks) ctx() context.Context {
	if t.Ctx != nil {
		return t.Ctx
	}
	return context.Background()
}

func (t *INDstocks) dir() string {
	if strings.TrimSpace(t.Dir) != "" {
		return t.Dir
	}
	return DefaultDir
}

func (t *INDstocks) path(symbol string) string {
	return filepath.Join(t.dir(), symbol+".json")
}

// dateKey is the IST calendar day of a candle open.
func dateKey(ts time.Time) string { return marketclock.SessionDate(ts) }

func (t *INDstocks) bars(symbol string, from, to time.Time) ([]prices.Bar, error) {
	if !t.Refresh {
		if b, err := os.ReadFile(t.path(symbol)); err == nil {
			var c cached
			if json.Unmarshal(b, &c) == nil && len(c.Bars) > 0 {
				first, last := c.Bars[0].Ts, c.Bars[len(c.Bars)-1].Ts
				// Covered if the cache starts within ~2 weeks of from and ends
				// within ~1 week of to (holidays and weekends explain the slack).
				if !first.After(from.Add(14*24*time.Hour)) && !last.Before(to.Add(-7*24*time.Hour)) {
					return c.Bars, nil
				}
			}
		}
	}
	if t.Client == nil {
		return nil, fmt.Errorf("no INDstocks client and no cached bars for %s under %s", symbol, t.dir())
	}
	bars, err := t.Client.DailyHistory(t.ctx(), symbol, from, to)
	if err != nil {
		return nil, err
	}
	c := cached{
		Symbol: symbol, Source: NameINDstocks,
		From: from.Format("2006-01-02"), To: to.Format("2006-01-02"),
		Fetched: time.Now().UTC().Format(time.RFC3339), Bars: bars,
	}
	if err := os.MkdirAll(t.dir(), 0o755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(t.path(symbol), b, 0o644); err != nil {
		return nil, err
	}
	return bars, nil
}

func window(years int, now time.Time) (from, to time.Time) {
	loc := marketclock.Location()
	n := now.In(loc)
	to = time.Date(n.Year(), n.Month(), n.Day(), 23, 59, 59, 0, loc)
	from = to.AddDate(-years, 0, 0)
	return from, to
}

// Days returns the NIFTY50 candle dates in [now-years, now], as IST midnights.
func (t *INDstocks) Days(years int, now time.Time) ([]time.Time, error) {
	from, to := window(years, now)
	bars, err := t.bars("NIFTY50", from, to)
	if err != nil {
		return nil, err
	}
	loc := marketclock.Location()
	seen := map[string]struct{}{}
	var days []time.Time
	for _, b := range bars {
		k := dateKey(b.Ts)
		if _, dup := seen[k]; dup {
			continue
		}
		d, err := time.ParseInLocation("2006-01-02", k, loc)
		if err != nil {
			continue
		}
		if d.Before(from) || d.After(to) {
			continue
		}
		seen[k] = struct{}{}
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	return days, nil
}

// Closes aligns symbol's daily closes to days. A day without a print
// carries the previous close forward; leading days before the first print
// take the first close so indicators warm up flat rather than on zeros.
func (t *INDstocks) Closes(symbol string, days []time.Time) ([]float64, error) {
	if len(days) == 0 {
		return nil, fmt.Errorf("no days")
	}
	from := days[0].Add(-24 * time.Hour)
	to := days[len(days)-1].Add(24*time.Hour - time.Second)
	bars, err := t.bars(symbol, from, to)
	if err != nil {
		return nil, err
	}
	byDay := map[string]float64{}
	for _, b := range bars {
		byDay[dateKey(b.Ts)] = b.Close
	}
	out := make([]float64, len(days))
	prev := 0.0
	missing := 0
	for i, d := range days {
		if c, ok := byDay[dateKey(d)]; ok && c > 0 {
			prev = c
		} else {
			missing++
		}
		out[i] = prev
	}
	if prev == 0 {
		return nil, fmt.Errorf("%s: no closes on the tape", symbol)
	}
	for i := range out { // leading gap → first real close
		if out[i] == 0 {
			out[i] = firstPositive(out)
		}
	}
	if missing > len(days)/4 {
		return nil, fmt.Errorf("%s: %d of %d days missing — tape too thin to trust", symbol, missing, len(days))
	}
	return out, nil
}

func firstPositive(xs []float64) float64 {
	for _, x := range xs {
		if x > 0 {
			return x
		}
	}
	return 0
}

// Pick returns the real tape when an INDstocks token is configured, or the
// mock tape otherwise. Callers must check backtest.IsMockTape before writing
// anything to data/roster/.
func Pick(ctx context.Context, dir string) backtest.Tape {
	if c := indstocks.Shared(); c != nil {
		return &INDstocks{Client: c, Dir: dir, Ctx: ctx}
	}
	if cached, err := os.ReadDir(dirOr(dir)); err == nil && len(cached) > 0 {
		// No token, but a previous run left daily bars on disk: still real evidence.
		return &INDstocks{Dir: dir, Ctx: ctx}
	}
	return backtest.MockTape{}
}

func dirOr(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return DefaultDir
	}
	return dir
}
