package trading

import (
	"context"
	"math"

	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/learn"
	"aperture/pkg/universe"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
)

func (s *Service) hasOpen(sym string) bool {
	for _, t := range s.open {
		if t.Symbol == sym && t.Open {
			return true
		}
	}
	return false
}

type excursion struct{ mae, mfe float64 }

func updateExcursion(e *excursion, side string, entry, px float64) {
	if e == nil || entry <= 0 || px <= 0 {
		return
	}
	move := (px - entry) / entry
	if side == "SELL" {
		move = -move
	}
	if move < e.mae {
		e.mae = move
	}
	if move > e.mfe {
		e.mfe = move
	}
}

func afterCostPnL(entry, exit, qty float64) float64 {
	buyN := entry * qty
	sellN := exit * qty
	buyC := costs.RoundTripBuy(buyN)
	sellC := costs.RoundTripSell(buyN, sellN)
	return (exit-entry)*qty - buyC.Total - sellC.Total
}

func (s *Service) markExcursionLocked(t *commonv1.PaperTrade, px float64) {
	if t == nil {
		return
	}
	if s.exc == nil {
		s.exc = map[string]*excursion{}
	}
	e := s.exc[t.Id]
	if e == nil {
		e = &excursion{}
		s.exc[t.Id] = e
	}
	updateExcursion(e, t.Side, t.Entry, px)
}

func (s *Service) closeLocked(t *commonv1.PaperTrade, px, niftyRet, adv float64) {
	fill, proceeds := costs.SellFill(px, t.Qty, adv)
	charge := costs.RoundTripSell(t.Entry*t.Qty, proceeds)
	s.cash += proceeds - charge.Total
	t.Exit = fill
	t.Open = false
	t.ClosedAtUnixMs = s.now().UnixMilli()
	t.Pnl = afterCostPnL(t.Entry, fill, t.Qty)
	if t.Entry > 0 && t.Qty > 0 {
		t.PnlPct = t.Pnl / (t.Entry * t.Qty) * 100
	}
	t.NiftyReturn = niftyRet * 100
	portRet := 0.0
	if t.Entry > 0 && t.Qty > 0 {
		portRet = t.Pnl / (t.Entry * t.Qty)
	}
	t.ExcessReturn = (portRet - niftyRet) * 100
	hold := t.ClosedAtUnixMs - t.OpenedAtUnixMs
	if hold < 0 {
		hold = 0
	}
	mae, mfe := 0.0, 0.0
	if e := s.exc[t.Id]; e != nil {
		mae, mfe = e.mae, e.mfe
		delete(s.exc, t.Id)
	}
	t.AttributionTags = append(learn.FactTags(mae, mfe, learn.Regime(t.NiftyReturn), hold), t.AttributionTags...)
	if t.Lesson != "" {
		t.Lesson += "; "
	}
	t.Lesson += charge.Lesson
	p := s.pos[t.Symbol]
	if p != nil {
		p.Qty -= t.Qty
		if p.Qty <= 0 {
			delete(s.pos, t.Symbol)
		} else {
			p.MarketValue = p.Qty * px
		}
	}
}

// markTrade appends an open trade's mark-to-market to a position.
func (s *Service) markTrade(p *commonv1.Position, t *commonv1.PaperTrade) {
	if p == nil || t == nil {
		return
	}
	p.Qty += t.Qty
	p.MarketValue = p.Qty * t.Entry
}

func (s *Service) updateMarksLocked(last map[string]float64) {
	for sym, p := range s.pos {
		if px, ok := last[sym]; ok {
			p.Last = px
			p.MarketValue = p.Qty * px
			p.Pnl = (px - p.AvgPrice) * p.Qty
		}
	}
}

func (s *Service) markLocked() float64 {
	eq := s.cash
	for _, p := range s.pos {
		eq += p.MarketValue
	}
	return eq
}

func (s *Service) snapshotLocked() broker.Snapshot {
	eq := s.markLocked()
	if eq > s.peak {
		s.peak = eq
	}
	held := map[string]float64{}
	sector := map[string]float64{}
	gross := 0.0
	for sym, p := range s.pos {
		if p == nil {
			continue
		}
		held[sym] = p.MarketValue
		gross += p.MarketValue
		if inst, ok := universe.Lookup(sym); ok {
			sector[inst.Sector] += p.MarketValue
		}
	}
	return broker.Snapshot{
		Equity: eq, Peak: s.peak, Cash: s.cash,
		Gross: gross, Held: held, Sector: sector,
	}
}

// tradeADV is 20-day ADV in ₹ from daily bars; 0 until the desk has history.
func (s *Service) tradeADV(ctx context.Context, sym string) float64 {
	resp, err := s.md.GetBars(ctx, &marketdatav1.GetBarsRequest{Symbol: sym, Interval: "1d", Count: 22})
	if err != nil || resp == nil || len(resp.Bars) < 3 {
		return 0
	}
	closes := make([]float64, len(resp.Bars))
	for i, b := range resp.Bars {
		closes[i] = b.Close * math.Max(b.Volume, 1)
	}
	return costs.ADV(closes, 20)
}
