package trading

import (
	"aperture/pkg/costs"

	commonv1 "aperture/gen/common/v1"
)

func (s *Service) hasOpen(sym string) bool {
	for _, t := range s.open {
		if t.Symbol == sym && t.Open {
			return true
		}
	}
	return false
}

func (s *Service) closeLocked(t *commonv1.PaperTrade, px, niftyRet float64) {
	fill := costs.SellFill(px)
	proceeds := t.Qty * fill
	comm := costs.Commission(proceeds)
	s.cash += proceeds - comm
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
