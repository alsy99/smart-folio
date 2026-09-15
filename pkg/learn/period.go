package learn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	PeriodsFile = "periods.jsonl"

	SleeveCore      = "core"
	SleeveSatellite = "satellite"
	// MethodCore is the core sleeve's method label. The core is a fixed
	// mix; it appears in the table so a client can read it, and nowhere in
	// the weight update.
	MethodCore = "core"
)

// Period is one sleeve's result over one rebalance period (or the tail of
// a campaign). It is the unit the product learns on: not a fill, a period
// judged against the client's own benchmark after costs.
type Period struct {
	IPSID  string    `json:"ipsId"`
	Sleeve string    `json:"sleeve"` // core | satellite
	Method string    `json:"method"` // core, or the satellite method
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	// PnLAfterCosts is ₹ over the period: mark-to-market change plus cash
	// flows, every ticket charged by pkg/costs.
	PnLAfterCosts float64 `json:"pnlAfterCosts"`
	// ExcessVsIPS and ExcessVsNifty are percentage points: sleeve return on
	// its target notional minus the benchmark's return over the same bars.
	ExcessVsIPS   float64 `json:"excessVsIps"`
	ExcessVsNifty float64 `json:"excessVsNifty"`
	// MaxDD is the worst peak-to-trough of the sleeve's value path over the
	// period as a fraction of its target notional.
	MaxDD float64 `json:"maxDd"`
	Fills int     `json:"fills"`
	// MAE and MFE are the mean excursions of the sleeve's closes in the
	// period (satellite only; 0 for the core).
	MAE float64 `json:"mae"`
	MFE float64 `json:"mfe"`
}

func PeriodsPath(dir string) string {
	return filepath.Join(dir, PeriodsFile)
}

// AppendPeriod is the only writer of periods.jsonl. Like AppendClose it
// never touches weights.json.
func AppendPeriod(dir string, p Period) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(PeriodsPath(dir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func LoadPeriods(dir string) ([]Period, error) {
	f, err := os.Open(PeriodsPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Period
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var p Period
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			continue // one bad line does not lose the table
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

// PeriodRow is one (IPS, sleeve, method) group of periods.
type PeriodRow struct {
	IPSID, Sleeve, Method string
	N, Fills              int
	SumPnL                float64
	SumExcessIPS          float64
	SumExcessNifty        float64
	WorstDD               float64
	SumMAE, SumMFE        float64
}

func (r PeriodRow) Key() string { return r.IPSID + "|" + r.Sleeve + "|" + r.Method }

// ExpectancyVsIPS is the mean excess over the client's benchmark per
// period, in percentage points, after costs.
func (r PeriodRow) ExpectancyVsIPS() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumExcessIPS / float64(r.N)
}

func (r PeriodRow) ExcessVsNifty() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumExcessNifty / float64(r.N)
}

func (r PeriodRow) MAE() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumMAE / float64(r.N)
}

func (r PeriodRow) MFE() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumMFE / float64(r.N)
}

// ByIPSSleeve groups periods by IPS id, sleeve and method. Core rows sort
// before satellite rows within an IPS.
func ByIPSSleeve(periods []Period) []PeriodRow {
	m := map[string]PeriodRow{}
	for _, p := range periods {
		r := PeriodRow{IPSID: p.IPSID, Sleeve: p.Sleeve, Method: p.Method}
		k := r.Key()
		r = m[k]
		r.IPSID, r.Sleeve, r.Method = p.IPSID, p.Sleeve, p.Method
		r.N++
		r.Fills += p.Fills
		r.SumPnL += p.PnLAfterCosts
		r.SumExcessIPS += p.ExcessVsIPS
		r.SumExcessNifty += p.ExcessVsNifty
		r.SumMAE += p.MAE
		r.SumMFE += p.MFE
		if p.MaxDD > r.WorstDD {
			r.WorstDD = p.MaxDD
		}
		m[k] = r
	}
	rows := make([]PeriodRow, 0, len(m))
	for _, r := range m {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IPSID != rows[j].IPSID {
			return rows[i].IPSID < rows[j].IPSID
		}
		if rows[i].Sleeve != rows[j].Sleeve {
			return rows[i].Sleeve == SleeveCore
		}
		return rows[i].Method < rows[j].Method
	})
	return rows
}

const periodHeader = "ips              sleeve     method              n  fills      pnl_₹  xs_ips_pp  xs_nifty_pp  max_dd      mae      mfe"

// FormatPeriodTable prints the period table grouped by IPS id and sleeve.
func FormatPeriodTable(rows []PeriodRow) string {
	var b strings.Builder
	b.WriteString(periodHeader)
	b.WriteByte('\n')
	if len(rows) == 0 {
		b.WriteString("(no periods)\n")
		return b.String()
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "%-16s %-10s %-18s %3d %6d %10.0f %10.2f %12.2f %7.3f %8.4f %8.4f\n",
			trunc(r.IPSID, 16), r.Sleeve, trunc(r.Method, 18), r.N, r.Fills, r.SumPnL,
			r.ExpectancyVsIPS(), r.ExcessVsNifty(), r.WorstDD, r.MAE(), r.MFE())
	}
	return b.String()
}

// satelliteFills counts closed satellite fills per method from the close
// ledger; a method with n ≥ MinN fills qualifies even with few periods.
func satelliteFills(closes []Close) map[string]int {
	out := map[string]int{}
	for _, c := range closes {
		m := c.Method
		if m == "" {
			m = MethodOf(c.StrategyID)
		}
		out[m]++
	}
	return out
}

// ReweightPeriods shrinks satellite methods toward 0 when they have run
// for n ≥ MinN periods or closed n ≥ MinN fills and their expectancy vs
// the client's benchmark, after costs, is negative. Core rows never move
// a weight: the core mix is the IPS, and only a new IPS changes it. The
// IPS itself is not an input here, so nothing in this file can raise
// SatellitePct.
func ReweightPeriods(ids []string, prev map[string]float64, periods []Period, closes []Close) (map[string]float64, []string) {
	next := copyWeights(prev, ids)
	fills := satelliteFills(closes)
	var shrunk []string
	for _, row := range ByIPSSleeve(periods) {
		if row.Sleeve != SleeveSatellite || row.Method == MethodCore {
			continue
		}
		if row.N < MinN && fills[row.Method] < MinN {
			continue
		}
		if row.ExpectancyVsIPS() >= 0 {
			continue
		}
		moved := false
		for _, id := range ids {
			if MethodOf(id) != row.Method {
				continue
			}
			w := next[id] * Shrink
			if w < MinWeight {
				w = 0
			}
			next[id] = w
			moved = true
		}
		if moved {
			shrunk = append(shrunk, row.Key())
		}
	}
	sort.Strings(shrunk)
	return renormalize(next), shrunk
}
