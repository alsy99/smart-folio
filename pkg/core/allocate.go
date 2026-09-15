// Package core is the always-invested sleeve: equal weight over the
// twelve large caps the public campaign already trades, scaled to the
// client's core percentage, then trimmed to the broker rails. No signal,
// no sentiment, no strategy spec. It changes only when the IPS changes.
package core

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/universe"
)

// Universe is the v1 core: the twelve names in pkg/universe. Not an index
// ETF (none on the tape), so this is a Nifty-50-ish large-cap proxy with
// real tracking error against the benchmark. Do not invent tickers.
func Universe() []string {
	return universe.EquitySymbols()
}

// Drift bands (spec §4.3): a name trades only if its weight is more than
// 100 bps off target, or cash is more than 2% of equity off target.
const (
	NameBand = 0.01
	CashBand = 0.02
)

// Targets is what the core wants to hold, as portfolio weights.
type Targets struct {
	Core    map[string]float64 // symbol → weight; sum = CorePct
	CorePct float64            // achieved after rails; ≤ requested
	Wanted  float64            // requested core weight before rails
	Cash    float64            // 1 - CorePct
	Reason  string
}

// Allocate spreads corePct equally over the universe and trims each name
// to the rails it will be checked against anyway: name cap, sector cap,
// gross cap. Trimmed weight stays in cash — it is not pushed into other
// names, which would stop the mix being equal weight. The Reason string
// says exactly what was trimmed so the desk can show it.
func Allocate(corePct float64) Targets {
	syms := Universe()
	out := Targets{Core: map[string]float64{}, Wanted: corePct}
	if corePct <= 0 || len(syms) == 0 {
		out.Cash = 1
		out.Reason = "core 0%: nothing to hold"
		return out
	}
	eq := corePct / float64(len(syms))
	perName := math.Min(eq, costs.NameCap)
	// Sector cap: names in one sector split the sector's ceiling equally.
	bySector := map[string][]string{}
	for _, s := range syms {
		inst, ok := universe.Lookup(s)
		if !ok {
			continue
		}
		bySector[inst.Sector] = append(bySector[inst.Sector], s)
	}
	trims := []string{}
	if perName < eq {
		trims = append(trims, fmt.Sprintf("%s %.0f%% trims %.2f%%→%.2f%% per name", broker.ReasonNameCap, costs.NameCap*100, eq*100, perName*100))
	}
	sectors := make([]string, 0, len(bySector))
	for sec := range bySector {
		sectors = append(sectors, sec)
	}
	sort.Strings(sectors)
	for _, sec := range sectors {
		names := bySector[sec]
		cap := broker.SectorCap / float64(len(names))
		w := perName
		if cap < w {
			w = cap
			trims = append(trims, fmt.Sprintf("%s %.0f%% (%s, %d names) trims to %.2f%%", broker.ReasonSectorCap, broker.SectorCap*100, sec, len(names), w*100))
		}
		for _, s := range names {
			out.Core[s] = w
		}
	}
	total := 0.0
	for _, w := range out.Core {
		total += w
	}
	if total > broker.GrossCap {
		scale := broker.GrossCap / total
		for s := range out.Core {
			out.Core[s] *= scale
		}
		total = broker.GrossCap
		trims = append(trims, fmt.Sprintf("%s %.0f%% scales the mix", broker.ReasonGrossCap, broker.GrossCap*100))
	}
	out.CorePct = round4(total)
	out.Cash = round4(1 - total)
	head := fmt.Sprintf("equal-weight %d names × core %.0f%% = %.2f%% per name", len(syms), corePct*100, eq*100)
	if len(trims) == 0 {
		out.Reason = head + "; inside every rail"
	} else {
		out.Reason = fmt.Sprintf("%s; %s; invested %.1f%%, cash %.1f%%", head, strings.Join(trims, "; "), out.CorePct*100, out.Cash*100)
	}
	return out
}

// Weights turns market values into portfolio weights.
func Weights(held map[string]float64, equity float64) map[string]float64 {
	out := map[string]float64{}
	if equity <= 0 {
		return out
	}
	for s, mv := range held {
		out[s] = mv / equity
	}
	return out
}

// Drift is a name's held weight minus its target.
type Drift struct {
	Symbol string
	Target float64
	Actual float64
	Drift  float64
	Ticket bool
}

// Drifts compares held weights to targets and marks which names would
// trade under the band rule. cashOff is how far cash sits from target as
// a fraction of equity; when it exceeds CashBand every off-target name is
// a ticket, otherwise only names beyond NameBand.
func Drifts(t Targets, held map[string]float64, cash, equity float64) (drifts []Drift, cashOff float64) {
	w := Weights(held, equity)
	if equity > 0 {
		cashOff = cash/equity - t.Cash
	}
	wide := math.Abs(cashOff) > CashBand
	syms := make([]string, 0, len(t.Core))
	for s := range t.Core {
		syms = append(syms, s)
	}
	for s := range w {
		if _, ok := t.Core[s]; !ok && w[s] > 0 {
			syms = append(syms, s) // held but not a target: sell down
		}
	}
	sort.Strings(syms)
	for _, s := range syms {
		d := Drift{Symbol: s, Target: t.Core[s], Actual: w[s]}
		d.Drift = d.Actual - d.Target
		if math.Abs(d.Drift) > NameBand || (wide && math.Abs(d.Drift) > 1e-4) {
			d.Ticket = true
		}
		drifts = append(drifts, d)
	}
	return drifts, cashOff
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }
