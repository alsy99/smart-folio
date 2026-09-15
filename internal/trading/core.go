package trading

import (
	"context"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/internal/policy"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/llm"
	"aperture/pkg/tilt"
)

const LogSatelliteCap = "SATELLITE_CAP"

// satellitePct is the IPS satellite slice; with no IPS the legacy book is
// all satellite and the cap is the gross cap.
func (s *Service) satellitePct() float64 {
	if s.ips == nil {
		return broker.GrossCap
	}
	return s.ips.SatellitePct
}

// coreValueLocked is the core sleeve's market value at the given marks.
func (s *Service) coreValueLocked(last map[string]float64) float64 {
	mv := 0.0
	for sym, q := range s.coreQty {
		mv += q * last[sym]
	}
	return mv
}

// satelliteRoomLocked is how much more satellite notional the IPS allows:
// the tilted slice × equity minus what the satellite already holds. The
// tilt only applies under an IPS; the legacy all-satellite book keeps the
// gross cap.
func (s *Service) satelliteRoomLocked(snap broker.Snapshot, last map[string]float64, score float64) float64 {
	pct := s.satellitePct()
	if s.ips != nil {
		pct = tilt.Effective(s.ips.SatellitePct, s.ips.SatellitePct, score)
	}
	held := snap.Gross - s.coreValueLocked(last)
	room := snap.Equity*pct - held
	if room < 0 {
		return 0
	}
	return room
}

const LogLLMBudget = "LLM_BUDGET"

// satelliteScore is the sentiment input to the satellite tilt for this
// bar. It is zero, and the tilt a no-op, when no method carries weight
// (nothing to tilt), when the LLM's IST-day budget is spent (logged once
// per session as LLM_BUDGET), or when no LLM-mode report is known at the
// bar. Reports have already passed pkg/asof, so a headline published
// after the bar is not in the slice.
func (s *Service) satelliteScore(bar time.Time, reports []*commonv1.InvestigationReport, weights map[string]float64) float64 {
	roster := false
	for _, w := range weights {
		if w > 0 {
			roster = true
			break
		}
	}
	if !roster {
		return 0
	}
	day := llm.ISTDay(bar)
	st := llm.LoadDay(s.invDir(), day)
	if st.MaxCalls > 0 && st.MaxTokens > 0 && (st.Calls >= st.MaxCalls || st.Tokens >= st.MaxTokens) {
		s.mu.Lock()
		logIt := s.budgetLogDay != day
		s.budgetLogDay = day
		s.mu.Unlock()
		if logIt {
			s.log.Warn(LogLLMBudget, "day", day, "calls", st.Calls, "max_calls", st.MaxCalls, "tokens", st.Tokens, "max_tokens", st.MaxTokens, "tilt", 0)
		}
		return 0
	}
	return tilt.Score(reports)
}

// coreInputLocked assembles the allocator's view of the book. Marks are
// the caller's; the calendar is NIFTY50 candle dates from the tape.
func (s *Service) coreInputLocked(ctx context.Context, now time.Time, last map[string]float64) policy.Input {
	held := map[string]float64{}
	qty := map[string]float64{}
	for sym, q := range s.coreQty {
		if q <= 0 {
			continue
		}
		qty[sym] = q
		held[sym] = q * last[sym]
	}
	adv := map[string]float64{}
	for _, sym := range s.ips.CoreSymbols() {
		adv[sym] = s.tradeADV(ctx, sym)
	}
	return policy.Input{
		IPS:            *s.ips,
		Now:            now,
		Cal:            s.calendar(ctx),
		Last:           last,
		ADV:            adv,
		Snap:           s.snapshotLocked(),
		CoreQty:        qty,
		CoreHeld:       held,
		SatelliteEmpty: satelliteEmpty(s.plan.weights, s.w),
		LastRebalance:  s.coreLastRebal,
		Pending:        s.corePending,
	}
}

// satelliteEmpty: learning has spoken and nothing carries weight. Silence
// (no weights ever) is not empty — the book then trades shipped defaults.
func satelliteEmpty(plan map[string]float64, w []*commonv1.StrategyWeight) bool {
	if len(plan) > 0 {
		for _, v := range plan {
			if v > 0 {
				return false
			}
		}
		return true
	}
	if len(w) == 0 {
		return false
	}
	for _, x := range w {
		if x.Weight > 0 {
			return false
		}
	}
	return true
}

// calendar is the tape's NIFTY50 session dates visible now.
func (s *Service) calendar(ctx context.Context) policy.Calendar {
	resp, err := s.md.GetBars(ctx, &marketdatav1.GetBarsRequest{Symbol: "NIFTY50", Interval: "1d", Count: 400})
	if err != nil || resp == nil {
		return nil
	}
	dates := make([]string, 0, len(resp.Bars))
	for _, b := range resp.Bars {
		dates = append(dates, policy.SessionDate(time.UnixMilli(b.TsUnixMs)))
	}
	return policy.CalendarFrom(dates)
}

// coreLocked runs the policy allocator and executes its tickets through
// the same paper broker and cost model as the satellite. Returns fills
// and turnover. Nothing here reads a signal, a headline, or a weight
// other than "is the satellite empty".
func (s *Service) coreLocked(ctx context.Context, now time.Time, last map[string]float64) (fills int, turnover float64) {
	if s.ips == nil {
		return 0, 0
	}
	in := s.coreInputLocked(ctx, now, last)
	plan := policy.Rebalance(in)
	s.lastCorePlan = plan
	date := policy.SessionDate(now)
	if plan.Halted {
		// The broker logs MAX_DRAWDOWN once for the book; this is the core's
		// own once-per-halt note that it is holding, not rebalancing.
		if !s.coreHaltLog {
			s.coreHaltLog = true
			s.log.Info(policy.LogCoreHalt, "session", date, "drawdown", broker.Drawdown(in.Snap), "cap", broker.HaltThreshold(in.Snap), "action", "hold; no core tickets")
		}
		return 0, 0
	}
	s.coreHaltLog = false
	if !plan.Session {
		return 0, 0
	}
	for _, t := range plan.Tickets {
		px := last[t.Symbol]
		switch t.Side {
		case broker.Sell:
			s.sellCoreLocked(t, px)
			fills++
			turnover += t.Notional
		case broker.Buy:
			snap := s.snapshotLocked()
			if dec := s.desk.Admit(t.Intent(), snap); !dec.Allow {
				s.log.Info("core ticket refused", "symbol", t.Symbol, "reason", dec.Reason)
				plan.Complete = false
				continue
			}
			s.buyCoreLocked(t, px)
			fills++
			turnover += t.Notional
		}
	}
	s.coreLastRebal = date
	s.corePending = !plan.Complete
	if s.coreLogKey != date+plan.Reason {
		s.coreLogKey = date + plan.Reason
		s.log.Info(policy.LogCoreRebalance, "session", date, "fills", fills, "turnover", turnover, "reason", plan.Reason, "next", plan.NextRebalance)
	}
	return fills, turnover
}

func (s *Service) buyCoreLocked(t policy.Ticket, px float64) {
	cost := t.Qty * t.Price
	charge := costs.RoundTripBuy(cost)
	s.cash -= cost + charge.Total
	s.dayBuys += cost
	s.coreQty[t.Symbol] += t.Qty
	p := s.pos[t.Symbol]
	if p == nil {
		p = &commonv1.Position{Symbol: t.Symbol}
		s.pos[t.Symbol] = p
	}
	newQty := p.Qty + t.Qty
	p.AvgPrice = (p.AvgPrice*p.Qty + t.Price*t.Qty) / newQty
	p.Qty = newQty
	p.Last = px
	p.MarketValue = p.Qty * px
	p.Pnl = (px - p.AvgPrice) * p.Qty
}

func (s *Service) sellCoreLocked(t policy.Ticket, px float64) {
	p := s.pos[t.Symbol]
	buyNotional := t.Qty * px
	if p != nil && p.AvgPrice > 0 {
		buyNotional = t.Qty * p.AvgPrice
	}
	charge := costs.RoundTripSell(buyNotional, t.Notional)
	s.cash += t.Notional - charge.Total
	s.coreQty[t.Symbol] -= t.Qty
	if s.coreQty[t.Symbol] <= 0 {
		delete(s.coreQty, t.Symbol)
	}
	if p == nil {
		return
	}
	p.Qty -= t.Qty
	if p.Qty <= 0 {
		delete(s.pos, t.Symbol)
		return
	}
	p.Last = px
	p.MarketValue = p.Qty * px
	p.Pnl = (px - p.AvgPrice) * p.Qty
}

// CoreInput implements policy.BookSource for PreviewTargets: the live
// book as allocator input, marked at the current quotes.
func (s *Service) CoreInput(ctx context.Context) (policy.Input, error) {
	quotes, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{})
	if err != nil {
		return policy.Input{}, err
	}
	last := map[string]float64{}
	for _, q := range quotes.Quotes {
		last[q.Symbol] = q.Last
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ips == nil {
		return policy.Input{}, policy.ErrNoIPS
	}
	s.updateMarksLocked(last)
	return s.coreInputLocked(ctx, s.now(), last), nil
}
