package trading

import (
	"context"
	"math"

	"aperture/pkg/costs"

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

func (s *Service) closeLocked(t *commonv1.PaperTrade, px, niftyRet, adv float64) {
	fill, proceeds := costs.SellFill(px, t.Qty, adv)
	charge := costs.RoundTripSell(t.Entry*t.Qty, proceeds)
	s.cash += proceeds - charge.Total
	t.Exit = fill
	t.Open = false
	t.ClosedAtUnixMs = s.now().UnixMilli()
	t.Pnl = (fill - t.Entry) * t.Qty
	if t.Entry > 0 {
		t.PnlPct = (fill/t.Entry - 1) * 100
	}
	t.NiftyReturn = niftyRet * 100
	portRet := 0.0
	if t.Entry > 0 {
		portRet = fill/t.Entry - 1
	}
	t.ExcessReturn = (portRet - niftyRet) * 100
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
