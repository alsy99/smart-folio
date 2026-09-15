package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"aperture/pkg/backtest"
	"aperture/pkg/marketclock"
	"aperture/pkg/prices"
	"aperture/pkg/universe"
)

const (
	// BarsFile is the checked-in daily tape the campaign replays: one close
	// per NSE session for every universe symbol and benchmark, from a
	// warm-up start before the window through its end. It is the input a
	// stranger needs to reproduce the equity line without a broker token.
	BarsFile = "bars.json"
	// RosterFile is the gate verdict the campaign traded on, frozen as-of
	// the last session before the window opened (no look-ahead).
	RosterFile = "roster.json"
	// WarmupDays of daily bars before the window so 40-bar indicators and
	// 20-day ADV are warm on day one.
	WarmupDays = 110
)

// DailyBar is one session's OHLCV, keyed by IST date.
type DailyBar struct {
	Date   string  `json:"d"`
	Open   float64 `json:"o"`
	High   float64 `json:"h"`
	Low    float64 `json:"l"`
	Close  float64 `json:"c"`
	Volume float64 `json:"v"`
}

// Bars is the campaign's daily tape.
type Bars struct {
	Source    string `json:"source"`    // "indstocks-1d" for every traded name
	FetchedAt string `json:"fetchedAt"` // RFC3339 UTC
	From      string `json:"from"`      // YYYY-MM-DD
	To        string `json:"to"`
	// Sources names any series that did not come from Source (a benchmark
	// the broker's history endpoint does not carry). Traded names are never
	// in here.
	Sources map[string]string     `json:"sources,omitempty"`
	Series  map[string][]DailyBar `json:"series"`
}

// Benchmarks the ledger publishes excess against.
var Benchmarks = []string{"NIFTY50", "NIFTY500", "SENSEX"}

// TapeInstruments is every series the campaign needs: the traded equities
// and the three index benchmarks. Proxy benchmarks (MF peers) have no tape
// and are not part of the public number.
func TapeInstruments() []universe.Instrument {
	out := append([]universe.Instrument{}, universe.Equities()...)
	for _, b := range Benchmarks {
		if inst, ok := universe.Lookup(b); ok {
			out = append(out, inst)
		}
	}
	return out
}

func barsPath(dir string) string { return filepath.Join(Dir(dir), BarsFile) }

func rosterPath(dir string) string { return filepath.Join(Dir(dir), RosterFile) }

// FromBars converts fetched candles to the checked-in shape, sorted and
// de-duplicated by IST date.
func FromBars(raw []prices.Bar) []DailyBar {
	byDay := map[string]DailyBar{}
	for _, b := range raw {
		d := marketclock.SessionDate(b.Ts)
		byDay[d] = DailyBar{Date: d, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume}
	}
	out := make([]DailyBar, 0, len(byDay))
	for _, b := range byDay {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func LoadBars(dir string) (*Bars, error) {
	b, err := os.ReadFile(barsPath(dir))
	if err != nil {
		return nil, err
	}
	var bars Bars
	if err := json.Unmarshal(b, &bars); err != nil {
		return nil, err
	}
	if len(bars.Series) == 0 {
		return nil, fmt.Errorf("%s has no series", barsPath(dir))
	}
	return &bars, nil
}

func SaveBars(dir string, bars *Bars) error {
	d := Dir(dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(bars)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, BarsFile), append(b, '\n'), 0o644)
}

// BarsSHA256 fingerprints the tape file so the manifest can pin it.
func BarsSHA256(dir string) (string, error) {
	b, err := os.ReadFile(barsPath(dir))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func LoadRoster(dir string) (backtest.RosterSnapshot, error) {
	b, err := os.ReadFile(rosterPath(dir))
	if err != nil {
		return backtest.RosterSnapshot{}, err
	}
	var snap backtest.RosterSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return backtest.RosterSnapshot{}, err
	}
	if backtest.IsMockTape(snap.Tape) {
		return backtest.RosterSnapshot{}, fmt.Errorf("%s is a %q roster; the public campaign needs a real-tape verdict", rosterPath(dir), snap.Tape)
	}
	return snap, nil
}

func SaveRoster(dir string, snap backtest.RosterSnapshot) error {
	d := Dir(dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, RosterFile), append(b, '\n'), 0o644)
}

// RosterAsOf is the last NSE session before the window opens: the latest
// close the gate may have seen without looking into the campaign.
func RosterAsOf() time.Time {
	start, _ := Window()
	loc := marketclock.Location()
	d := time.Date(start.Year(), start.Month(), start.Day(), 18, 0, 0, 0, loc).AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// WarmupStart is the first date the bars file must cover.
func WarmupStart() time.Time {
	start, _ := Window()
	return start.AddDate(0, 0, -WarmupDays)
}
