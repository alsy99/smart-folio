package policy

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"aperture/pkg/broker"
	"aperture/pkg/core"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"
)

const (
	ReasonCoreRebalance = "CORE_REBALANCE"
	ReasonCoreInitial   = "CORE_INITIAL"
	LogCoreRebalance    = "CORE_REBALANCE"
	LogCoreHalt         = "CORE_HALT"
)

// Input is everything the allocator may look at. Marks and holdings are
// as-of the bar; there is no headline, strategy, or LLM field on purpose.
type Input struct {
	IPS ips.IPS
	Now time.Time
	Cal Calendar
	// Last marks; ADV (₹, 20-day) for slippage, 0 = unknown.
	Last map[string]float64
	ADV  map[string]float64
	// Snap is the whole book for the rails. HaltAt is set from the IPS here.
	Snap broker.Snapshot
	// Core sleeve holdings: quantity and market value by symbol.
	CoreQty  map[string]float64
	CoreHeld map[string]float64
	// SatelliteEmpty: no method carries weight. With FoldSatellite the
	// satellite slice is treated as core at rebalance.
	SatelliteEmpty bool
	// LastRebalance is the date of the last rebalance session that was
	// worked; "" means the book has never been built (initial session).
	LastRebalance string
	// Pending: a rebalance started but a rail (turnover) left names outside
	// the band. The job keeps working it on following sessions.
	Pending bool
}

type Ticket struct {
	Symbol   string
	Side     broker.Side
	Qty      float64
	Price    float64 // expected fill after slippage
	Notional float64
	Reason   string
	Refused  string // broker reason when the rails said no
}

// Intent is the broker ticket for this core order: a calendar intent.
func (t Ticket) Intent() broker.Intent {
	return broker.Intent{Symbol: t.Symbol, Side: t.Side, Qty: t.Qty, Price: t.Price, Calendar: true}
}

type Plan struct {
	Targets       core.Targets
	Drifts        []core.Drift
	Tickets       []Ticket // admitted, sells first
	Refused       []Ticket
	Deferred      int  // buys waiting on next session's turnover room
	Session       bool // calendar session, initial build, or pending work
	Complete      bool // every name inside the band after these tickets
	Halted        bool
	Reason        string
	NextRebalance string
	CoreWanted    float64
	cashOff       float64
}

// Preview computes targets and drifts without deciding on tickets: what
// the core wants, what the book holds, and when the next session is.
func Preview(in Input) Plan {
	date := SessionDate(in.Now)
	want := in.IPS.CorePct
	if in.IPS.FoldSatellite && in.SatelliteEmpty {
		want += in.IPS.SatellitePct
	}
	tg := core.Allocate(want)
	p := Plan{Targets: tg, NextRebalance: in.Cal.NextRebalance(date, in.IPS.Rebalance), CoreWanted: want}
	// Cash from the core's point of view is everything that is not core.
	coreMV := 0.0
	for _, mv := range in.CoreHeld {
		coreMV += mv
	}
	p.Drifts, p.cashOff = core.Drifts(tg, in.CoreHeld, in.Snap.Equity-coreMV, in.Snap.Equity)
	snap := in.Snap
	snap.HaltAt = in.IPS.MaxDD
	p.Halted = broker.Halted(snap)
	p.Session = in.LastRebalance == "" || in.Cal.IsRebalanceSession(date, in.IPS.Rebalance) || in.Pending
	switch {
	case p.Halted:
		p.Reason = fmt.Sprintf("%s: drawdown %.2f%% ≥ cap %.0f%%; holding, no new buys", broker.ReasonMaxDrawdown, broker.Drawdown(snap)*100, broker.HaltThreshold(snap)*100)
	case !marketclock.IsOpen(in.Now):
		p.Reason = fmt.Sprintf("market closed: no core tickets; next rebalance %s", p.NextRebalance)
	case !p.Session:
		p.Reason = fmt.Sprintf("not a rebalance session; next %s", p.NextRebalance)
	case in.LastRebalance == "":
		p.Reason = fmt.Sprintf("%s: first session under this IPS", ReasonCoreInitial)
	default:
		p.Reason = fmt.Sprintf("%s: rebalance session", ReasonCoreRebalance)
	}
	return p
}

// Rebalance is pure: same input, same tickets. It never calls a strategy,
// never reads sentiment, and the LLM does not exist here.
func Rebalance(in Input) Plan {
	p := Preview(in)
	if p.Halted || !marketclock.IsOpen(in.Now) {
		p.Session = false
		return p
	}
	if !p.Session {
		return p
	}
	reason := ReasonCoreRebalance
	if in.LastRebalance == "" {
		reason = ReasonCoreInitial
	}
	snap := in.Snap
	snap.HaltAt = in.IPS.MaxDD
	tg := p.Targets
	cashOff := p.cashOff
	drifts := append([]core.Drift{}, p.Drifts...)

	// Sells first so the buys have cash and turnover room.
	sort.SliceStable(drifts, func(i, j int) bool {
		if (drifts[i].Drift > 0) != (drifts[j].Drift > 0) {
			return drifts[i].Drift > 0
		}
		return drifts[i].Symbol < drifts[j].Symbol
	})
	complete := true
	for _, d := range drifts {
		if !d.Ticket {
			continue
		}
		px := in.Last[d.Symbol]
		if px <= 0 {
			complete = false
			continue
		}
		wantN := math.Abs(d.Drift) * snap.Equity
		if d.Drift > 0 {
			qty := math.Floor(wantN / px)
			if held := in.CoreQty[d.Symbol]; qty > held {
				qty = held
			}
			if qty < 1 {
				continue
			}
			fill, proceeds := costs.SellFill(px, qty, in.ADV[d.Symbol])
			t := Ticket{Symbol: d.Symbol, Side: broker.Sell, Qty: qty, Price: fill, Notional: proceeds, Reason: reason}
			if dec := broker.Check(t.Intent(), snap); !dec.Allow {
				t.Refused = dec.Reason
				p.Refused = append(p.Refused, t)
				complete = false
				continue
			}
			p.Tickets = append(p.Tickets, t)
			snap = afterSell(snap, d.Symbol, proceeds, px*qty)
			continue
		}
		if broker.TurnoverRoom(snap) < costs.MinNameNotional {
			// Session new-buy rail is spent. The rest is deferred, not
			// refused: the job continues next session.
			complete = false
			p.Deferred++
			continue
		}
		room := broker.RailRoom(snap, d.Symbol)
		n := math.Min(wantN, room)
		if n < wantN-1 {
			complete = false // clamped by a rail; work the rest next session
		}
		if n < costs.MinNameNotional {
			if wantN >= costs.MinNameNotional {
				p.Refused = append(p.Refused, Ticket{Symbol: d.Symbol, Side: broker.Buy, Notional: wantN, Reason: reason, Refused: whyNoRoom(snap, d.Symbol)})
			}
			continue
		}
		qty := math.Floor(n / px)
		if qty < 1 {
			continue
		}
		adv := in.ADV[d.Symbol]
		fill := costs.BuyFill(px, qty, adv)
		if q := math.Floor(n / fill); q < qty {
			qty = q
			fill = costs.BuyFill(px, qty, adv)
		}
		if qty < 1 {
			continue
		}
		t := Ticket{Symbol: d.Symbol, Side: broker.Buy, Qty: qty, Price: fill, Notional: qty * fill, Reason: reason}
		if dec := broker.Check(t.Intent(), snap); !dec.Allow {
			t.Refused = dec.Reason
			p.Refused = append(p.Refused, t)
			complete = false
			continue
		}
		p.Tickets = append(p.Tickets, t)
		snap = afterBuy(snap, d.Symbol, t.Notional)
	}
	p.Complete = complete
	p.Reason = summary(reason, tg, p, cashOff)
	return p
}

func summary(reason string, tg core.Targets, p Plan, cashOff float64) string {
	buys, sells := 0, 0
	for _, t := range p.Tickets {
		if t.Side == broker.Buy {
			buys++
		} else {
			sells++
		}
	}
	state := "complete"
	if !p.Complete {
		state = "pending: rails left names outside the band; continues next session"
	}
	parts := []string{
		fmt.Sprintf("%s: %d buys, %d sells, %d refused, %d deferred", reason, buys, sells, len(p.Refused), p.Deferred),
		fmt.Sprintf("core target %.1f%% (%s)", tg.CorePct*100, tg.Reason),
		fmt.Sprintf("cash off target %+.2f%%", cashOff*100),
		state,
	}
	return strings.Join(parts, " · ")
}

func whyNoRoom(s broker.Snapshot, sym string) string {
	if broker.TurnoverRoom(s) < costs.MinNameNotional {
		return broker.ReasonTurnover
	}
	if s.Cash-s.Equity*broker.CashBuffer < costs.MinNameNotional {
		return broker.ReasonCashBuffer
	}
	return broker.ReasonNameCap
}

func afterBuy(s broker.Snapshot, sym string, notional float64) broker.Snapshot {
	s = cloneSnap(s)
	s.Cash -= notional
	s.Gross += notional
	s.Held[sym] += notional
	s.DayBuys += notional
	if inst, ok := lookupSector(sym); ok {
		s.Sector[inst] += notional
	}
	return s
}

func afterSell(s broker.Snapshot, sym string, proceeds, mv float64) broker.Snapshot {
	s = cloneSnap(s)
	s.Cash += proceeds
	s.Gross -= mv
	s.Held[sym] -= mv
	if inst, ok := lookupSector(sym); ok {
		s.Sector[inst] -= mv
	}
	return s
}

func lookupSector(sym string) (string, bool) {
	inst, ok := universe.Lookup(sym)
	if !ok {
		return "", false
	}
	return inst.Sector, true
}

func cloneSnap(s broker.Snapshot) broker.Snapshot {
	held := make(map[string]float64, len(s.Held))
	for k, v := range s.Held {
		held[k] = v
	}
	sec := make(map[string]float64, len(s.Sector))
	for k, v := range s.Sector {
		sec[k] = v
	}
	s.Held, s.Sector = held, sec
	return s
}
