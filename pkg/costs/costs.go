package costs

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	StartCash     = 1_000_000.0
	CommissionBps = 5.0
	SlippageBps   = 4.0

	// Positional cash rails (the shipped book).
	NameCap         = 0.08
	DrawdownHalt    = 0.15
	SessionHold     = 6*time.Hour + 15*time.Minute // NSE cash 09:15–15:30
	MaxHold         = 15 * 24 * time.Hour
	MinNameNotional = 5_000.0

	// Positional turnover discipline. Delivery STT is 0.1% each way, so a
	// name flipped daily loses ~0.2% + brokerage per round trip before the
	// signal is even wrong. A signal flip may close a name only after
	// MinHoldSessions completed cash sessions (MaxHold still recycles it),
	// and new-buy notional in one IST session is capped at TurnoverCapDay
	// of equity. Both are broker-intent rails (see broker.Check), not copy.
	MinHoldSessions = 3
	TurnoverCapDay  = 0.10

	// SCALP_MODE=true only.
	ScalpTake    = 0.012
	ScalpStop    = -0.008
	ScalpMaxHold = 45 * time.Second

	// India delivery charges (mirror the INDstocks margin/charges payload).
	// Rates are current as of the NSE cash-market circulars used by discount
	// brokers; keep as constants so tests pin exact contract-note math.
	RefSTTBpsBuy  = 10.0 // delivery buy  : 0.1%
	RefSTTBpsSell = 10.0 // delivery sell : 0.1%
	RefExchBps    = 0.0297
	RefSEBIBps    = 0.0001
	RefStampBps   = 1.5 // delivery buy only
	RefGSTRate    = 0.18
	RefBrokerage  = 10.0 // ₹10 per executed order, INDstocks API tariff

	// Slippage scales with 20-day ADV (₹). Constants chosen so RELIANCE-scale
	// ADV (≈ ₹7.5e9) yields ≈ 4 bps one-way — matching the old flat 4 bps on
	// the mock tape — while thin books move toward the cap.
	ADVBaseBps  = 2.0
	ADVKappa    = 1.0e11
	ADVMinBps   = 2.0
	ADVMaxBps   = 40.0
	ADVWindow   = 20
	DayADVClamp = 1.0e10 // never let a single noisy print move the tape beyond this
)

// Side of a cash-market execution; delivery charges differ buy vs sell.
type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

// Line is one row of the charge ledger (for logging / contract-note tests).
type Line struct {
	Name string
	Amt  float64
}

// Charges is the full delivery-cost ledger for a round trip.
// Buy-side rows appear first, then sell-side; Total is in ₹.
type Charges struct {
	Adv       float64 // 20-day ADV used for slippage scaling (₹)
	BuyLines  []Line
	SellLines []Line
	BuyTotal  float64 // Σ buy lines (already includes buy slippage)
	SellTotal float64 // Σ sell lines (includes sell slippage)
	Slippage  float64 // total slippage in ₹ (informational)
	Statutory float64 // STT + stamp duty + SEBI + exchange
	GST       float64
	Brokerage float64
	Total     float64 // BuyTotal + SellTotal (charges only, excl. slippage)
	NetCost   float64 // Total + Slippage  — what the book actually pays
	Lesson    string  // compact ledger line for the journal
}

// ADV returns the n-day average daily value (₹) of a price×volume series.
// closes should already be value-traded per bar (close × volume).
func ADV(closes []float64, n int) float64 {
	if len(closes) == 0 {
		return 0
	}
	if n <= 0 {
		n = ADVWindow
	}
	start := len(closes) - n
	if start < 0 {
		start = 0
	}
	var sum float64
	for _, v := range closes[start:] {
		sum += v
	}
	return sum / float64(len(closes)-start)
}

// ADVSlippageBps converts 20-day ADV into one-way slippage in bps.
// adv==0 (no history) falls back to the flat SlippageBps so the mock tape
// and early paper fills behave as before.
func ADVSlippageBps(adv float64) float64 {
	if adv <= 0 {
		return SlippageBps
	}
	bps := ADVBaseBps + math.Sqrt(ADVKappa/adv)
	return math.Min(math.Max(bps, ADVMinBps), ADVMaxBps)
}

// BuyFill returns the execution price for a buy of qty shares at mark px,
// given 20-day ADV in ₹. qty==0 keeps the legacy flat-slip path.
func BuyFill(px, qty, adv float64) float64 {
	return px * (1 + slippageFrac(qty, adv)/1e4)
}

// SellFill returns the execution price for a sell, same convention.
// The second return value is the cash proceeds (qty × fill).
func SellFill(px, qty, adv float64) (float64, float64) {
	fill := px * (1 - slippageFrac(qty, adv)/1e4)
	return fill, qty * fill
}

// RoundTripBuy is the buy-side delivery ledger; when qty>0 the slippage is
// ADV-scaled to the actual order size, else the legacy flat 4 bps.
func RoundTripBuy(notional float64, qty ...float64) Charges {
	q := 1.0
	if len(qty) > 0 {
		q = qty[0]
	}
	return roundTrip(Buy, notional, 0, q)
}

// RoundTripSell is the sell-side ledger; buyNotional keeps the ₹10-per-order
// brokerage convention when the two legs differ in size.
func RoundTripSell(buyNotional, sellNotional float64, qty ...float64) Charges {
	q := 1.0
	if len(qty) > 0 {
		q = qty[0]
	}
	return roundTrip(Sell, sellNotional, buyNotional, q)
}

// RoundTrip is the full delivery cost for buyNotional→sellNotional.
// Callers that only have one side should use RoundTripBuy/Sell.
func RoundTrip(buyNotional, sellNotional, adv float64) Charges {
	c := Charges{Adv: adv}
	// Slippage is priced into fills, not a cash line; we still track it so
	// the journal can show "slippage vs the flat 4 bps model".
	c.BuyLines = ledgerLines(Buy, buyNotional, adv)
	c.SellLines = ledgerLines(Sell, sellNotional, adv)
	c.BuyTotal = sum(c.BuyLines)
	c.SellTotal = sum(c.SellLines)
	for _, l := range c.BuyLines {
		c.addStat(l)
	}
	for _, l := range c.SellLines {
		c.addStat(l)
	}
	c.Total = c.BuyTotal + c.SellTotal
	// Slippage is priced into fills, not a cash line; we still track it so
	// the journal can show "slippage vs the flat 4 bps model".
	c.Slippage = slippageINR(buyNotional, adv) + slippageINR(sellNotional, adv)
	c.NetCost = c.Total + c.Slippage
	c.Lesson = c.lesson()
	return c
}

func slippageINR(notional, adv float64) float64 {
	if notional <= 0 || adv <= 0 {
		return 0
	}
	bps := ADVSlippageBps(adv)
	return notional * bps / 1e4
}

func (c *Charges) addStat(l Line) {
	switch l.Name {
	case "STT", "StampDuty", "SEBI", "Exchange":
		c.Statutory += l.Amt
	case "GST":
		c.GST += l.Amt
	case "Brokerage":
		c.Brokerage += l.Amt
	case "Slippage":
		c.Slippage += l.Amt
	}
}

func (c Charges) lesson() string {
	var b strings.Builder
	b.WriteString("delivery ledger ")
	fmt.Fprintf(&b, "adv=%.2e ", c.Adv)
	for _, l := range c.BuyLines {
		fmt.Fprintf(&b, "buy.%s=%.2f ", l.Name, l.Amt)
	}
	for _, l := range c.SellLines {
		fmt.Fprintf(&b, "sell.%s=%.2f ", l.Name, l.Amt)
	}
	fmt.Fprintf(&b, "charges=%.2f slip=%.2f net=%.2f", c.Total, c.Slippage, c.NetCost)
	return b.String()
}

func sum(ls []Line) float64 {
	var t float64
	for _, l := range ls {
		t += l.Amt
	}
	return t
}

// roundTrip builds the per-side ledger without slippage (slippage is a fill
// adjustment, not a cash charge).  buyNotional is only used on the sell leg
// to mirror the ₹10-per-order brokerage convention.  qty scales the ADV
// slippage term; 0 keeps the flat 4 bps fallback.
func roundTrip(side Side, notional, buyNotional, qty float64) Charges {
	var c Charges
	if side == Buy {
		c.BuyLines = ledgerLines(Buy, notional, 0)
		c.BuyTotal = sum(c.BuyLines)
	} else {
		c.SellLines = ledgerLines(Sell, notional, buyNotional)
		c.SellTotal = sum(c.SellLines)
	}
	c.Total = c.BuyTotal + c.SellTotal
	c.NetCost = c.Total
	c.Lesson = c.lesson()
	return c
}

func ledgerLines(side Side, notional, otherNotional float64) []Line {
	if notional <= 0 {
		return nil
	}
	var out []Line
	sttBps := RefSTTBpsBuy
	if side == Sell {
		sttBps = RefSTTBpsSell
	}
	out = append(out, Line{"STT", notional * sttBps / 1e4})
	out = append(out, Line{"Exchange", notional * RefExchBps / 1e4})
	out = append(out, Line{"SEBI", notional * RefSEBIBps / 1e4})
	if side == Buy {
		out = append(out, Line{"StampDuty", notional * RefStampBps / 1e4})
	}
	brokerage := RefBrokerage
	if side == Sell && otherNotional > 0 && otherNotional != notional {
		// INDstocks books ₹10 per executed order; if the sell size differs
		// from the buy the two legs still cost ₹10 each.
		brokerage = RefBrokerage
	}
	out = append(out, Line{"Brokerage", brokerage})
	taxable := brokerage + notional*RefExchBps/1e4 + notional*RefSEBIBps/1e4
	out = append(out, Line{"GST", taxable * RefGSTRate})
	return out
}

func slippageFrac(qty, adv float64) float64 {
	if qty <= 0 || adv <= 0 {
		return SlippageBps
	}
	return ADVSlippageBps(adv)
}

func NameRoom(equity, held float64) float64 {
	if equity <= 0 {
		return 0
	}
	room := equity*NameCap - held
	if room < 0 {
		return 0
	}
	return room
}

func BookHalted(equity, peak float64) bool {
	if peak <= 0 {
		return false
	}
	return (peak-equity)/peak >= DrawdownHalt
}
