package broker

import (
	"time"

	"aperture/pkg/costs"
	"aperture/pkg/universe"
)

type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

// Intent is the ticket the desk wants the broker to send. Paper fills and a
// future live adapter both pass through Check before any fill.
type Intent struct {
	Symbol string
	Side   Side
	Qty    float64
	Price  float64
}

func (in Intent) Notional() float64 {
	if in.Qty < 0 || in.Price < 0 {
		return 0
	}
	return in.Qty * in.Price
}

// Snapshot is the book as-of the bar, marked. Peak is the high-water equity
// used for the 15% peak-to-trough halt.
type Snapshot struct {
	Equity float64
	Peak   float64
	Cash   float64
	Gross  float64            // sum of long market values
	Held   map[string]float64 // symbol → market value
	Sector map[string]float64 // sector → market value
	// DayBuys is new-buy notional already filled in the current IST session.
	// Check refuses a buy that would push it past costs.TurnoverCapDay × Equity.
	DayBuys float64
}

// TurnoverRoom is the new-buy notional still allowed this session.
func TurnoverRoom(s Snapshot) float64 {
	if s.Equity <= 0 {
		return 0
	}
	room := s.Equity*costs.TurnoverCapDay - s.DayBuys
	if room < 0 {
		return 0
	}
	return room
}

// MayExit is the positional min-hold rail for a signal-driven close. A name
// may leave the book on a signal flip only after costs.MinHoldSessions full
// cash sessions; MaxHold always recycles it. Halt and scalp paths do not
// call this.
func MayExit(sessionsHeld int, hold time.Duration) Decision {
	if hold >= costs.MaxHold {
		return allow()
	}
	if hold < costs.SessionHold || sessionsHeld < costs.MinHoldSessions {
		return deny(ReasonMinHold)
	}
	return allow()
}

func (s Snapshot) held(sym string) float64 {
	if s.Held == nil {
		return 0
	}
	return s.Held[sym]
}

func (s Snapshot) sectorHeld(sym string) float64 {
	if s.Sector == nil {
		return 0
	}
	inst, ok := universe.Lookup(sym)
	if !ok {
		return 0
	}
	return s.Sector[inst.Sector]
}

// Halted is a 15% peak-to-trough drawdown. New buys are refused; sells still pass.
func Halted(s Snapshot) bool {
	return costs.BookHalted(s.Equity, s.Peak)
}

func Drawdown(s Snapshot) float64 {
	if s.Peak <= 0 {
		return 0
	}
	d := (s.Peak - s.Equity) / s.Peak
	if d < 0 {
		return 0
	}
	return d
}

type Decision struct {
	Allow  bool
	Reason string
}

func allow() Decision { return Decision{Allow: true} }

func deny(reason string) Decision { return Decision{Reason: reason} }

// Check is the broker-intent stop. The Go tick loop must not size around it.
func Check(in Intent, s Snapshot) Decision {
	if in.Symbol == "" || in.Qty < 1 || in.Price <= 0 {
		return deny(ReasonEmpty)
	}
	if in.Side == Sell {
		return allow()
	}
	if in.Side != Buy {
		return deny(ReasonEmpty)
	}
	if Halted(s) {
		return deny(ReasonMaxDrawdown)
	}
	notional := in.Notional()
	if notional < costs.MinNameNotional {
		return deny(ReasonMinNotional)
	}
	if notional > TurnoverRoom(s)+1e-6 {
		return deny(ReasonTurnover)
	}
	if room := Room(s, in.Symbol); notional > room+1e-6 {
		return deny(breach(s, in.Symbol, notional))
	}
	return allow()
}

func breach(s Snapshot, sym string, notional float64) string {
	eq := s.Equity
	if eq <= 0 {
		return ReasonEmpty
	}
	if s.held(sym)+notional > eq*costs.NameCap+1e-6 {
		return ReasonNameCap
	}
	if s.Gross+notional > eq*GrossCap+1e-6 {
		return ReasonGrossCap
	}
	if s.Cash-notional < eq*CashBuffer-1e-6 {
		return ReasonCashBuffer
	}
	if s.sectorHeld(sym)+notional > eq*SectorCap+1e-6 {
		return ReasonSectorCap
	}
	return ReasonNameCap
}

// Room is the largest buy notional the rails allow for symbol right now.
func Room(s Snapshot, symbol string) float64 {
	if Halted(s) || s.Equity <= 0 {
		return 0
	}
	room := s.Equity*costs.NameCap - s.held(symbol)
	if g := s.Equity*GrossCap - s.Gross; g < room {
		room = g
	}
	if c := s.Cash - s.Equity*CashBuffer; c < room {
		room = c
	}
	if slice := s.Cash * CashSlice; slice < room {
		room = slice
	}
	if sec := s.Equity*SectorCap - s.sectorHeld(symbol); sec < room {
		room = sec
	}
	if t := TurnoverRoom(s); t < room {
		room = t
	}
	if room < 1 {
		return 0
	}
	return room
}
