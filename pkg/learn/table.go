package learn

import (
	"fmt"
	"sort"
	"strings"
)

type Row struct {
	Key        string
	Method     string
	StrategyID string
	Regime     string
	N          int
	Wins       int
	SumPnL     float64
	SumExcess  float64
	SumHoldMs  int64
	SumMAE     float64
	SumMFE     float64
}

func (r Row) Expectancy() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumPnL / float64(r.N)
}

func (r Row) Excess() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumExcess / float64(r.N)
}

func (r Row) WinRate() float64 {
	if r.N == 0 {
		return 0
	}
	return float64(r.Wins) / float64(r.N)
}

func (r Row) HoldHours() float64 {
	if r.N == 0 {
		return 0
	}
	return (float64(r.SumHoldMs) / float64(r.N)) / 3_600_000
}

func (r Row) MAE() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumMAE / float64(r.N)
}

func (r Row) MFE() float64 {
	if r.N == 0 {
		return 0
	}
	return r.SumMFE / float64(r.N)
}

func add(dst map[string]Row, key string, c Close, method, strategy, regime string) {
	r := dst[key]
	r.Key = key
	r.Method = method
	r.StrategyID = strategy
	r.Regime = regime
	r.N++
	if c.PnL > 0 {
		r.Wins++
	}
	r.SumPnL += c.PnL
	r.SumExcess += c.Excess
	r.SumHoldMs += c.HoldMs
	r.SumMAE += c.MAE
	r.SumMFE += c.MFE
	dst[key] = r
}

func sorted(m map[string]Row) []Row {
	out := make([]Row, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func ByMethod(closes []Close) []Row {
	m := map[string]Row{}
	for _, c := range closes {
		method := c.Method
		if method == "" {
			method = MethodOf(c.StrategyID)
		}
		add(m, method, c, method, "", "")
	}
	return sorted(m)
}

func ByTag(closes []Close) []Row {
	m := map[string]Row{}
	for _, c := range closes {
		add(m, Tag(c), c, MethodOf(c.StrategyID), c.StrategyID, c.Regime)
	}
	return sorted(m)
}

func ByStrategy(closes []Close) []Row {
	m := map[string]Row{}
	for _, c := range closes {
		add(m, c.StrategyID, c, MethodOf(c.StrategyID), c.StrategyID, "")
	}
	return sorted(m)
}

const tableHeader = "method               n   expectancy     excess  win_rate  hold_h      mae      mfe"

func FormatTable(rows []Row) string {
	var b strings.Builder
	b.WriteString(tableHeader)
	b.WriteByte('\n')
	if len(rows) == 0 {
		b.WriteString("(no closes)\n")
		return b.String()
	}
	for _, r := range rows {
		name := r.Method
		if name == "" {
			name = r.Key
		}
		fmt.Fprintf(&b, "%-18s %5d %12.2f %10.2f %9.2f %7.1f %8.4f %8.4f\n",
			trunc(name, 18), r.N, r.Expectancy(), r.Excess(), r.WinRate(),
			r.HoldHours(), r.MAE(), r.MFE())
	}
	return b.String()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
