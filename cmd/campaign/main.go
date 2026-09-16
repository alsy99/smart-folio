// Command campaign freezes, replays and verifies the public 30-day paper
// campaign on the checked-in INDstocks daily tape.
//
// Strangers:
//
//	go run ./cmd/campaign -verify     # replay bars.json + roster.json, match the published equity line
//
// Maintainers (need an INDstocks token), in this order, on a clean commit:
//
//	go run ./cmd/campaign -fetch      # write campaign/public-30d/bars.json (warm-up → window end)
//	go run ./cmd/campaign -roster     # run the lab as-of the last session before the window → roster.json
//	go run ./cmd/campaign -run -force # write ledger.json; then commit it ALONE
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	runcamp "aperture/internal/campaign"
	"aperture/pkg/backtest"
	"aperture/pkg/campaign"
	"aperture/pkg/indstocks"
	"aperture/pkg/prices"
	"aperture/pkg/tape"
)

func main() {
	run := flag.Bool("run", false, "write ledger.json in -dir / -name (refused once frozen unless -force)")
	verify := flag.Bool("verify", false, "replay the frozen tape and match the published equity line")
	force := flag.Bool("force", false, "allow -run to overwrite a frozen ledger")
	fetch := flag.Bool("fetch", false, "maintainers: fetch INDstocks daily bars into bars.json")
	roster := flag.Bool("roster", false, "maintainers: run the walk-forward as-of the window start and write roster.json")
	barsDir := flag.String("bars", tape.DefaultDir, "daily-bar cache for the as-of roster run")
	dir := flag.String("dir", campaign.DefaultDir, "campaign directory")
	name := flag.String("name", "", "campaign name under campaign/ (e.g. public-30d-core); overrides -dir")
	flag.Parse()
	if *name != "" {
		*dir = "campaign/" + *name
	}

	if *fetch {
		if err := fetchBars(*dir); err != nil {
			fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
			os.Exit(1)
		}
	}
	if *roster {
		if err := asOfRoster(*dir, *barsDir); err != nil {
			fmt.Fprintf(os.Stderr, "roster: %v\n", err)
			os.Exit(1)
		}
	}
	if !*run && !*verify {
		if *fetch || *roster {
			return
		}
		*verify = true
	}

	got, err := runcamp.ReplayDir(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}

	if *run {
		if campaign.IsFrozenDir(*dir) && !*force {
			fmt.Fprintf(os.Stderr, "%s is frozen. Clone the SHA and use -verify. Maintainers: -run -force.\n", *dir)
			os.Exit(2)
		}
		if err := campaign.Save(*dir, got); err != nil {
			fmt.Fprintf(os.Stderr, "save: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s sha=%s dirty=%v tape=%s ips=%s/%s roster=%d failing=%d days=%d fills=%d equity=%.2f halted=%v\n",
			campaign.Path(*dir), got.Manifest.GitSHA, got.Manifest.Dirty, got.Manifest.Tape, got.Manifest.IPSID, got.Manifest.IPSHash,
			len(got.Manifest.Roster), len(got.Manifest.Failing), len(got.Days), fills(got), lastEquity(got), lastHalt(got))
	}

	if *verify {
		want, err := campaign.Load(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		if want.Manifest.BarsSHA256 != got.Manifest.BarsSHA256 {
			fmt.Fprintf(os.Stderr, "bars.json sha256 %s vs published %s — the tape changed\n", got.Manifest.BarsSHA256, want.Manifest.BarsSHA256)
			os.Exit(1)
		}
		if want.Manifest.SettingsHash != got.Manifest.SettingsHash {
			fmt.Fprintf(os.Stderr, "settings hash %s vs published %s — freeze is broken\n", got.Manifest.SettingsHash, want.Manifest.SettingsHash)
			os.Exit(1)
		}
		if err := campaign.MatchEquity(want.Days, got.Days); err != nil {
			fmt.Fprintf(os.Stderr, "equity line: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ok %s sha=%s tape=%s bars=%s settings=%s ips=%s roster=%d failing=%d days=%d fills=%d last_equity=%.2f excess_nifty50=%+.2f%% halted=%v\n",
			want.Manifest.Name, want.Manifest.GitSHA, want.Manifest.Tape, short(want.Manifest.BarsSHA256), want.Manifest.SettingsHash, want.Manifest.IPSHash,
			len(want.Manifest.Roster), len(want.Manifest.Failing), len(want.Days), fills(want), lastEquity(want), lastExcess(want), lastHalt(want))
	}
}

// fetchBars pulls INDstocks 1day candles for the universe and benchmarks
// from the warm-up start through the window end and writes bars.json.
func fetchBars(dir string) error {
	c := indstocks.Shared()
	if c == nil {
		return fmt.Errorf("INDstocks token not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	from := campaign.WarmupStart()
	_, end := campaign.Window()
	bars := &campaign.Bars{
		Source: campaign.Tape, FetchedAt: time.Now().UTC().Format(time.RFC3339),
		From: from.Format("2006-01-02"), To: end.Format("2006-01-02"), Series: map[string][]campaign.DailyBar{},
	}
	for _, inst := range campaign.TapeInstruments() {
		var (
			raw []prices.Bar
			err error
			src = campaign.Tape
		)
		if alt, ok := benchmarkFallback[inst.Symbol]; ok {
			// INDstocks' history endpoint rejects this index; the benchmark
			// (never traded) comes from a public daily series, named in the file.
			raw, err = yahooDaily(ctx, alt, from, end)
			src = "yahoo-1d:" + alt
		} else {
			raw, err = c.DailyHistory(ctx, inst.Symbol, from, end)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", inst.Symbol, err)
		}
		bars.Series[inst.Symbol] = campaign.FromBars(raw)
		if src != campaign.Tape {
			if bars.Sources == nil {
				bars.Sources = map[string]string{}
			}
			bars.Sources[inst.Symbol] = src
		}
		fmt.Printf("fetched %-12s %4d sessions %s..%s  %s\n", inst.Symbol, len(bars.Series[inst.Symbol]),
			bars.Series[inst.Symbol][0].Date, bars.Series[inst.Symbol][len(bars.Series[inst.Symbol])-1].Date, src)
	}
	if err := campaign.SaveBars(dir, bars); err != nil {
		return err
	}
	sum, _ := campaign.BarsSHA256(dir)
	fmt.Printf("wrote %s/%s sha256=%s\n", campaign.Dir(dir), campaign.BarsFile, sum)
	return nil
}

// benchmarkFallback maps a benchmark INDstocks cannot serve history for to
// a public daily series. Only benchmarks — a traded name must come from the
// broker tape or the campaign does not run.
var benchmarkFallback = map[string]string{
	"NIFTY500": "^CRSLDX",
}

// yahooDaily reads daily closes from Yahoo's chart endpoint (no auth).
func yahooDaily(ctx context.Context, ticker string, from, to time.Time) ([]prices.Bar, error) {
	u := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d",
		url.PathEscape(ticker), from.Unix(), to.Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Aperture campaign fetch)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo %s: http %d", ticker, resp.StatusCode)
	}
	var body struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open, High, Low, Close, Volume []*float64
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Chart.Result) == 0 || len(body.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("yahoo %s: empty chart", ticker)
	}
	r := body.Chart.Result[0]
	q := r.Indicators.Quote[0]
	f := func(xs []*float64, i int) float64 {
		if i < len(xs) && xs[i] != nil {
			return *xs[i]
		}
		return 0
	}
	var out []prices.Bar
	for i, ts := range r.Timestamp {
		c := f(q.Close, i)
		if c <= 0 {
			continue // Yahoo pads holidays with nulls
		}
		out = append(out, prices.Bar{Ts: time.Unix(ts, 0), Open: f(q.Open, i), High: f(q.High, i), Low: f(q.Low, i), Close: c, Volume: f(q.Volume, i)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("yahoo %s: no closes", ticker)
	}
	return out, nil
}

// asOfRoster runs the five-year walk-forward with the clock stopped at the
// last session before the window, so the gate cannot see into the campaign.
func asOfRoster(dir, barsDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	tp := tape.Pick(ctx, barsDir)
	if backtest.IsMockTape(tp.Name()) {
		return fmt.Errorf("no INDstocks token and no cached bars under %s; the campaign roster needs a real tape", barsDir)
	}
	asOf := campaign.RosterAsOf()
	rep := backtest.RunOn(tp, 5, asOf)
	if rep.Status != "complete" {
		return fmt.Errorf("%s: %s", rep.Status, rep.Note)
	}
	snap := rep.Snapshot
	snap.GitSHA, _ = campaign.GitSHA()
	if err := campaign.SaveRoster(dir, snap); err != nil {
		return err
	}
	fmt.Printf("wrote %s/%s asOf=%s tape=%s closes=%d roster=[%s] failing=[%s]\n",
		campaign.Dir(dir), campaign.RosterFile, snap.Date, snap.Tape, snap.Days,
		strings.Join(snap.Roster, " "), strings.Join(snap.Failing, " "))
	return nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func fills(led *campaign.Ledger) int {
	n := 0
	for _, d := range led.Days {
		n += d.Fills
	}
	return n
}

func lastEquity(led *campaign.Ledger) float64 {
	if led == nil || len(led.Days) == 0 {
		return 0
	}
	return led.Days[len(led.Days)-1].Equity
}

func lastExcess(led *campaign.Ledger) float64 {
	if led == nil || len(led.Days) == 0 {
		return 0
	}
	return led.Days[len(led.Days)-1].ExcessNifty50Pct
}

func lastHalt(led *campaign.Ledger) bool {
	if led == nil || len(led.Days) == 0 {
		return false
	}
	return led.Days[len(led.Days)-1].Halted
}
