package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"

	"aperture/pkg/costs"
	"aperture/pkg/prices"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"
)

// Promotion gates — a variant joins the live roster only on out-of-sample
// evidence that survives costs, with enough trades to mean something and a
// drawdown inside the published book cap.
const (
	MinOOSTrades  = 30
	MaxNewPerSnap = 2 // cap new admissions per weekly snapshot
)

// DDCapPct mirrors the live book halt (costs.DrawdownHalt = 15%).
const DDCapPct = 15.0

type Variant struct {
	Spec      strategies.Spec
	ReturnPct float64 // out-of-sample compounded return, net of costs
	ExcessPct float64 // OOS return − Nifty OOS return
	WinRate   float64 // OOS
	Trades    int     // OOS round trips
	Wins      int
	MaxDDPct  float64 // worst OOS equity drawdown observed
	Promoted  bool
	Lesson    string
}

type Report struct {
	Years            int
	VariantsTested   int
	VariantsPromoted int
	Folds            int
	Status           string
	RanAt            time.Time
	NiftyReturnPct   float64
	Variants         []Variant
	Snapshot         RosterSnapshot
	Note             string
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

// oosWindows returns expanding-window OOS test folds: the last two years of
// the tape, each one year wide, traded only after the cut. Everything before
// the cut is in-sample warm-up (train); the strategy never sees it as a
// tradable signal, which is what keeps the fold honest.
func oosWindows(days []time.Time) [][2]int {
	if len(days) < 260 {
		return nil
	}
	end := days[len(days)-1]
	var wins [][2]int
	prev := -1
	for k := 2; k >= 1; k-- {
		cut := end.AddDate(-k, 0, 0)
		from := sort.Search(len(days), func(i int) bool { return !days[i].Before(cut) })
		if from < 60 { // need real train history behind the fold
			continue
		}
		if prev >= 0 {
			wins[len(wins)-1][1] = from
		}
		wins = append(wins, [2]int{from, len(days)})
		prev = from
	}
	return wins
}

type foldResult struct {
	retPct    float64
	excessPct float64
	trades    int
	wins      int
	maxDDPct  float64
}

// simulateFold trades closes[from:to] with full-history indicators. Equity
// is marked daily inside the window so max drawdown is measured on the OOS
// path, not on a single endpoint.
func simulateFold(spec strategies.Spec, closes, nifty []float64, from, to int) foldResult {
	if to > len(closes) {
		to = len(closes)
	}
	hold := 5
	if spec.Timeframe == "1w" {
		hold = 15
	}
	if spec.Timeframe == "1d" {
		hold = 10
	}
	need := spec.Slow
	if spec.Lookback > need {
		need = spec.Lookback
	}
	if need < 20 {
		need = 20
	}
	start := from
	if start < need {
		start = need
	}
	var r foldResult
	if start >= to-1 {
		return r
	}
	cash := costs.StartCash
	qty := 0.0
	entry := 0.0
	entryI := 0
	peak := costs.StartCash
	markDD := func(i int) {
		eq := cash + qty*closes[i]
		if eq > peak {
			peak = eq
		}
		if peak > 0 {
			if dd := (peak - eq) / peak * 100; dd > r.maxDDPct {
				r.maxDDPct = dd
			}
		}
	}
	closePos := func(i int) {
		px := closes[i]
		adv := costs.ADV(closes[:i+1], 20)
		exit, proceeds := costs.SellFill(px, qty, adv)
		charge := costs.RoundTripSell(qty*entry, proceeds, qty)
		cash += proceeds - charge.Total
		pnl := (exit - entry) * qty
		r.trades++
		if pnl > 0 {
			r.wins++
		}
		qty = 0
	}
	for i := start; i < to; i++ {
		w := window(closes, i, need+2)
		sig := strategies.Evaluate(spec, "BT", w, 0)
		px := closes[i]
		if qty > 0 && (sig.Direction <= 0 || i-entryI >= hold) {
			closePos(i)
		}
		if qty == 0 && sig.Direction > 0 && sig.Score >= 0.25 {
			adv := costs.ADV(closes[:i+1], 20)
			fill := costs.BuyFill(px, 1, adv)
			notional := cash * 0.2
			if notional > cash*0.9 {
				notional = cash * 0.9
			}
			q := math.Floor(notional / fill)
			if q >= 1 {
				charge := costs.RoundTripBuy(q*fill, q)
				if cost := q*fill + charge.Total; cost <= cash {
					cash -= cost
					qty = q
					entry = fill
					entryI = i
				}
			}
		}
		markDD(i)
	}
	if qty > 0 {
		closePos(to - 1)
	}
	r.retPct = (cash/costs.StartCash - 1) * 100
	if from < len(nifty) && nifty[from] > 0 {
		r.excessPct = r.retPct - (nifty[to-1]/nifty[from]-1)*100
	}
	return r
}

// walkForward aggregates the OOS folds for one spec across the universe.
func walkForward(spec strategies.Spec, syms []string, books map[string][]float64, nifty []float64, wins [][2]int) Variant {
	v := Variant{Spec: spec}
	if len(wins) == 0 {
		v.Lesson = "no OOS folds — not enough history for walk-forward."
		return v
	}
	niftyOOS := 1.0
	for _, w := range wins {
		if nifty[w[0]] > 0 {
			niftyOOS *= nifty[w[1]-1] / nifty[w[0]]
		}
	}
	niftyOOS = (niftyOOS - 1) * 100
	sumRet := 0.0
	for _, sym := range syms {
		closes := books[sym]
		symRet := 1.0
		for _, w := range wins {
			fr := simulateFold(spec, closes, nifty, w[0], w[1])
			symRet *= 1 + fr.retPct/100
			v.Trades += fr.trades
			v.Wins += fr.wins
			if fr.maxDDPct > v.MaxDDPct {
				v.MaxDDPct = fr.maxDDPct
			}
		}
		sumRet += (symRet - 1) * 100
	}
	v.ReturnPct = sumRet / float64(len(syms))
	v.ExcessPct = v.ReturnPct - niftyOOS
	if v.Trades > 0 {
		v.WinRate = float64(v.Wins) / float64(v.Trades)
	}
	return v
}

// passesGate is the promotion bar: out-of-sample excess vs Nifty net of
// delivery costs, enough round trips, and a drawdown inside the book cap.
func passesGate(v Variant) bool {
	return v.ExcessPct > 0 && v.Trades >= MinOOSTrades && v.MaxDDPct <= DDCapPct
}

func gateLesson(v Variant) string {
	win := "OOS walk-forward"
	switch {
	case !passesGate(v):
		return fmt.Sprintf("%s / %s: %s excess %+.2f%%, %d trades, maxDD %.1f%% — fails the gate (need excess>0, ≥%d trades, DD≤%.0f%%).",
			v.Spec.Method, v.Spec.Timeframe, win, v.ExcessPct, v.Trades, v.MaxDDPct, MinOOSTrades, DDCapPct)
	default:
		return fmt.Sprintf("%s / %s: %s excess %+.2f%% net of delivery costs, %d trades, maxDD %.1f%% — survives the gate.",
			v.Spec.Method, v.Spec.Timeframe, win, v.ExcessPct, v.Trades, v.MaxDDPct)
	}
}

func Run(years int, now time.Time) Report {
	if years <= 0 {
		years = 5
	}
	days := tradingDays(now, years)
	if len(days) < 80 {
		return Report{Years: years, Status: "error", Note: "not enough history", RanAt: now}
	}
	wins := oosWindows(days)
	if len(wins) == 0 {
		return Report{Years: years, Status: "error", Note: "not enough history for walk-forward folds", RanAt: now}
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
		v := walkForward(spec, syms, books, nifty, wins)
		v.Lesson = gateLesson(v)
		variants = append(variants, v)
	}
	sort.Slice(variants, func(i, j int) bool { return variants[i].ExcessPct > variants[j].ExcessPct })

	// The shipped defaults stay on the roster; they are the vetted book, not
	// "new" promotions. Anything beyond the defaults must pass the OOS gate,
	// and at most MaxNewPerSnap admissions land in one weekly snapshot.
	defaults := map[string]struct{}{}
	for _, d := range strategies.DefaultSpecs() {
		defaults[d.ID] = struct{}{}
	}
	snap := RosterSnapshot{
		Date:  now.Format("2006-01-02"),
		Folds: len(wins),
		Note:  "Walk-forward OOS gate: excess>0 net of delivery costs, ≥30 OOS trades, maxDD≤15%. At most 2 new admissions per weekly snapshot.",
	}
	seenMethod := map[string]struct{}{}
	promoted := 0
	for i := range variants {
		v := &variants[i]
		if _, isDef := defaults[v.Spec.ID]; isDef {
			v.Promoted = true
			promoted++
			snap.Roster = append(snap.Roster, v.Spec.ID)
			continue
		}
		if !passesGate(*v) || len(snap.Added) >= MaxNewPerSnap {
			continue
		}
		if _, dup := seenMethod[v.Spec.Method]; dup {
			continue
		}
		seenMethod[v.Spec.Method] = struct{}{}
		v.Promoted = true
		promoted++
		snap.Roster = append(snap.Roster, v.Spec.ID)
		snap.Added = append(snap.Added, Promotion{
			ID: v.Spec.ID, OOSExcessPct: v.ExcessPct, OOSTrades: v.Trades, MaxDDPct: v.MaxDDPct,
		})
	}

	return Report{
		Years: years, VariantsTested: len(variants), VariantsPromoted: promoted,
		Folds: len(wins),
		Status: "complete", RanAt: now, NiftyReturnPct: nRet * 100, Variants: variants,
		Snapshot: snap,
		Note: fmt.Sprintf("Walk-forward on the mock tape: %d expanding folds, metrics are out-of-sample and net of delivery costs. Roster is data/roster/%s.json.", len(wins), snap.Date),
	}
}
