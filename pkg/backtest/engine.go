package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"

	"aperture/pkg/prices"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"
)

const (
	commissionBps = 5.0
	slippageBps   = 4.0
	startCash     = 1_000_000.0
)

type Variant struct {
	Spec       strategies.Spec
	ReturnPct  float64
	ExcessPct  float64
	WinRate    float64
	Trades     int
	Wins       int
	Promoted   bool
	Lesson     string
}

type Report struct {
	Years           int
	VariantsTested  int
	VariantsPromoted int
	Status          string
	RanAt           time.Time
	NiftyReturnPct  float64
	Variants        []Variant
	Note            string
}

func tradingDays(end time.Time, years int) []time.Time {
	start := end.AddDate(-years, 0, 0)
	var days []time.Time
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			continue
		}
		days = append(days, t)
	}
	return days
}

func series(symbol string, days []time.Time) []float64 {
	out := make([]float64, len(days))
	for i, d := range days {
		out[i] = prices.Last(symbol, d)
	}
	return out
}

func window(closes []float64, i, n int) strategies.Window {
	lo := i + 1 - n
	if lo < 0 {
		lo = 0
	}
	c := closes[lo : i+1]
	h := make([]float64, len(c))
	l := make([]float64, len(c))
	copy(h, c)
	copy(l, c)
	for j := range h {
		h[j] *= 1.003
		l[j] *= 0.997
	}
	return strategies.Window{Close: c, High: h, Low: l}
}

func simulate(spec strategies.Spec, closes []float64, nifty []float64) Variant {
	hold := 5
	if spec.Timeframe == "1w" {
		hold = 10
	}
	if spec.Timeframe == "1d" || spec.Timeframe == "1h" {
		hold = 7
	}
	cash := startCash
	qty := 0.0
	entry := 0.0
	entryI := 0
	trades, wins := 0, 0
	pnlSum := 0.0
	need := spec.Slow
	if spec.Lookback > need {
		need = spec.Lookback
	}
	if need < 20 {
		need = 20
	}
	for i := need; i < len(closes); i++ {
		w := window(closes, i, need+2)
		sig := strategies.Evaluate(spec, "BT", w, 0)
		px := closes[i]
		if qty > 0 && (sig.Direction <= 0 || i-entryI >= hold) {
			exit := px * (1 - slippageBps/1e4)
			proceeds := qty * exit
			cash += proceeds * (1 - commissionBps/1e4)
			pnl := (exit - entry) * qty
			pnlSum += pnl
			trades++
			if pnl > 0 {
				wins++
			}
			qty = 0
		}
		if qty == 0 && sig.Direction > 0 && sig.Score >= 0.25 {
			fill := px * (1 + slippageBps/1e4)
			notional := cash * 0.2
			if notional > cash*0.9 {
				notional = cash * 0.9
			}
			q := math.Floor(notional / fill)
			if q < 1 {
				continue
			}
			cost := q * fill * (1 + commissionBps/1e4)
			if cost > cash {
				continue
			}
			cash -= cost
			qty = q
			entry = fill
			entryI = i
		}
	}
	if qty > 0 {
		px := closes[len(closes)-1]
		exit := px * (1 - slippageBps/1e4)
		cash += qty * exit * (1 - commissionBps/1e4)
		pnl := (exit - entry) * qty
		pnlSum += pnl
		trades++
		if pnl > 0 {
			wins++
		}
	}
	ret := cash/startCash - 1
	nRet := 0.0
	if len(nifty) > 0 && nifty[0] > 0 {
		nRet = nifty[len(nifty)-1]/nifty[0] - 1
	}
	wr := 0.0
	if trades > 0 {
		wr = float64(wins) / float64(trades)
	}
	v := Variant{
		Spec: spec, ReturnPct: ret * 100, ExcessPct: (ret - nRet) * 100,
		WinRate: wr, Trades: trades, Wins: wins,
	}
	if v.ExcessPct > 0 && wr >= 0.45 && trades >= 8 {
		v.Lesson = fmt.Sprintf("%s on %s beat Nifty in the 5y mock tape — promote into the live roster.", spec.Method, spec.Timeframe)
	} else if v.ExcessPct < 0 {
		v.Lesson = fmt.Sprintf("%s / %s lagged Nifty over 5y; keep as exploration only.", spec.Method, spec.Timeframe)
	} else {
		v.Lesson = fmt.Sprintf("%s / %s was mixed versus Nifty; needs more live evidence.", spec.Method, spec.Timeframe)
	}
	return v
}

func Run(years int, now time.Time) Report {
	if years <= 0 {
		years = 5
	}
	days := tradingDays(now, years)
	if len(days) < 80 {
		return Report{Years: years, Status: "error", Note: "not enough history", RanAt: now}
	}
	nifty := series("NIFTY50", days)
	nRet := 0.0
	if nifty[0] > 0 {
		nRet = nifty[len(nifty)-1]/nifty[0] - 1
	}
	syms := universe.EquitySymbols()
	books := map[string][]float64{}
	for _, sym := range syms {
		books[sym] = series(sym, days)
	}

	grid := strategies.SearchGrid()
	var variants []Variant
	for _, spec := range grid {
		agg := Variant{Spec: spec}
		totalRet := 0.0
		for _, sym := range syms {
			one := simulate(spec, books[sym], nifty)
			totalRet += one.ReturnPct
			agg.Trades += one.Trades
			agg.Wins += one.Wins
		}
		n := float64(len(syms))
		agg.ReturnPct = totalRet / n
		agg.ExcessPct = agg.ReturnPct - nRet*100
		if agg.Trades > 0 {
			agg.WinRate = float64(agg.Wins) / float64(agg.Trades)
		}
		agg.Lesson = simulate(spec, books[syms[0]], nifty).Lesson
		if agg.ExcessPct > 0 && agg.WinRate >= 0.45 && agg.Trades >= 8 {
			agg.Lesson = spec.Method + " / " + spec.Timeframe + " beat Nifty on the 5y mock book — promote."
		} else if agg.ExcessPct < 0 {
			agg.Lesson = spec.Method + " / " + spec.Timeframe + " lagged Nifty over 5y."
		}
		variants = append(variants, agg)
	}
	sort.Slice(variants, func(i, j int) bool { return variants[i].ExcessPct > variants[j].ExcessPct })

	defaults := map[string]struct{}{}
	for _, d := range strategies.DefaultSpecs() {
		defaults[d.ID] = struct{}{}
	}
	extraKeys := map[string]struct{}{}
	promoted := 0
	extra := 0
	for i := range variants {
		_, isDef := defaults[variants[i].Spec.ID]
		if isDef {
			variants[i].Promoted = true
			promoted++
			continue
		}
		key := variants[i].Spec.Method
		if extra < 5 && variants[i].ExcessPct > 0 && variants[i].Trades >= 8 {
			if _, ok := extraKeys[key]; ok {
				continue
			}
			extraKeys[key] = struct{}{}
			variants[i].Promoted = true
			promoted++
			extra++
		}
	}

	return Report{
		Years: years, VariantsTested: len(variants), VariantsPromoted: promoted,
		Status: "complete", RanAt: now, NiftyReturnPct: nRet * 100, Variants: variants,
		Note: "Mock 5-year NSE-like path (weekdays). Not a live exchange backtest. Promoted variants seed live weights.",
	}
}
