package trading

import (
	"context"
	"sort"
	"time"

	learningv1 "aperture/gen/learning/v1"
	"aperture/pkg/learn"
)

const LogPeriod = "PERIOD"

// sleeveTrack accounts one sleeve (the core, or one satellite method)
// over a rebalance period: value = market value + net cash flow, so PnL
// after costs is value_end − value_start and drawdown is on that path.
type sleeveTrack struct {
	mv0    float64
	flow   float64 // +proceeds −cost −charges since the period opened
	peak   float64
	maxDD  float64
	fills  int
	closes int
	mae    float64
	mfe    float64
}

func newSleeveTrack(mv float64) *sleeveTrack {
	return &sleeveTrack{mv0: mv, peak: mv}
}

func (t *sleeveTrack) mark(mv float64) {
	v := mv + t.flow
	if v > t.peak {
		t.peak = v
	}
	if dd := t.peak - v; dd > t.maxDD {
		t.maxDD = dd
	}
}

func (t *sleeveTrack) pnl(mv float64) float64 { return mv + t.flow - t.mv0 }

// periodTrack is the open period for the book under its IPS.
type periodTrack struct {
	from   time.Time
	eq0    float64
	bench0 map[string]float64
	core   *sleeveTrack
	sat    map[string]*sleeveTrack // by satellite method
}

func (s *Service) periodSat(method string) *sleeveTrack {
	if s.period == nil {
		return nil
	}
	t := s.period.sat[method]
	if t == nil {
		t = newSleeveTrack(0)
		s.period.sat[method] = t
	}
	return t
}

// satelliteMVLocked is the satellite's market value by method from open
// trades at the given marks.
func (s *Service) satelliteMVLocked(last map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for _, t := range s.open {
		px := last[t.Symbol]
		if px == 0 {
			px = t.Entry
		}
		out[learn.MethodOf(t.StrategyId)] += t.Qty * px
	}
	return out
}

// periodOpenLocked starts a period at the current marks. No IPS, no
// periods: the legacy book learns per fill only.
func (s *Service) periodOpenLocked(now time.Time, last map[string]float64) {
	if s.ips == nil {
		return
	}
	p := &periodTrack{
		from:   now,
		eq0:    s.markLocked(),
		bench0: map[string]float64{},
		core:   newSleeveTrack(s.coreValueLocked(last)),
		sat:    map[string]*sleeveTrack{},
	}
	for _, b := range []string{"NIFTY50", string(s.ips.Benchmark)} {
		if px := last[b]; px > 0 {
			p.bench0[b] = px
		}
	}
	for m, mv := range s.satelliteMVLocked(last) {
		p.sat[m] = newSleeveTrack(mv)
	}
	s.period = p
}

func (s *Service) periodCoreFlow(amount float64) {
	if s.period == nil {
		return
	}
	s.period.core.flow += amount
	s.period.core.fills++
}

func (s *Service) periodSatBuy(method string, spent float64) {
	if t := s.periodSat(method); t != nil {
		t.flow -= spent
		t.fills++
	}
}

func (s *Service) periodSatClose(method string, received, mae, mfe float64) {
	if t := s.periodSat(method); t != nil {
		t.flow += received
		t.closes++
		t.mae += mae
		t.mfe += mfe
	}
}

// periodMarkLocked walks the drawdown path once per execute.
func (s *Service) periodMarkLocked(last map[string]float64) {
	if s.period == nil {
		return
	}
	s.period.core.mark(s.coreValueLocked(last))
	mv := s.satelliteMVLocked(last)
	for m, t := range s.period.sat {
		t.mark(mv[m])
	}
}

func benchReturn(bench0 map[string]float64, last map[string]float64, sym string) (float64, bool) {
	b0, l := bench0[sym], last[sym]
	if b0 <= 0 || l <= 0 {
		return 0, false
	}
	return l/b0 - 1, true
}

// periodCloseLocked books the open period as learn.Period rows — one for
// the core, one per satellite method with activity — sends them to
// learning, and opens the next period at the same marks. Returns emitted
// rows for tests and logs.
func (s *Service) periodCloseLocked(now time.Time, last map[string]float64, why string) []learn.Period {
	if s.ips == nil || s.period == nil {
		return nil
	}
	p := s.period
	nifty, okN := benchReturn(p.bench0, last, "NIFTY50")
	ipsRet, okI := benchReturn(p.bench0, last, string(s.ips.Benchmark))
	row := func(sleeve, method string, t *sleeveTrack, mv, base float64) learn.Period {
		pnl := t.pnl(mv)
		out := learn.Period{
			IPSID: s.ips.ID, Sleeve: sleeve, Method: method, From: p.from, To: now,
			PnLAfterCosts: pnl, Fills: t.fills,
		}
		if base > 0 {
			ret := pnl / base
			out.MaxDD = t.maxDD / base
			if okI {
				out.ExcessVsIPS = (ret - ipsRet) * 100
			}
			if okN {
				out.ExcessVsNifty = (ret - nifty) * 100
			}
		}
		if t.closes > 0 {
			out.MAE = t.mae / float64(t.closes)
			out.MFE = t.mfe / float64(t.closes)
		}
		return out
	}
	var rows []learn.Period
	coreBase := p.eq0 * s.ips.CorePct
	if s.ips.FoldSatellite && len(p.sat) == 0 {
		coreBase = p.eq0 * (s.ips.CorePct + s.ips.SatellitePct)
	}
	rows = append(rows, row(learn.SleeveCore, learn.MethodCore, p.core, s.coreValueLocked(last), coreBase))
	satMV := s.satelliteMVLocked(last)
	methods := make([]string, 0, len(p.sat))
	for m := range p.sat {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	satBase := p.eq0 * s.ips.SatellitePct
	for _, m := range methods {
		t := p.sat[m]
		if t.mv0 == 0 && t.fills == 0 && t.closes == 0 && satMV[m] == 0 {
			continue
		}
		rows = append(rows, row(learn.SleeveSatellite, m, t, satMV[m], satBase))
	}
	for _, r := range rows {
		s.log.Info(LogPeriod, "why", why, "ips", r.IPSID, "sleeve", r.Sleeve, "method", r.Method,
			"from", r.From.Format("2006-01-02"), "to", r.To.Format("2006-01-02"),
			"pnl", r.PnLAfterCosts, "xs_ips_pp", r.ExcessVsIPS, "xs_nifty_pp", r.ExcessVsNifty, "max_dd", r.MaxDD, "fills", r.Fills)
		go s.recordPeriod(r)
	}
	s.periods = append(s.periods, rows...)
	s.periodOpenLocked(now, last)
	return rows
}

func toProtoPeriod(p learn.Period) *learningv1.Period {
	return &learningv1.Period{
		IpsId: p.IPSID, Sleeve: p.Sleeve, Method: p.Method,
		FromUnixMs: p.From.UnixMilli(), ToUnixMs: p.To.UnixMilli(),
		PnlAfterCosts: p.PnLAfterCosts, ExcessVsIpsPp: p.ExcessVsIPS, ExcessVsNiftyPp: p.ExcessVsNifty,
		MaxDd: p.MaxDD, Fills: int32(p.Fills), Mae: p.MAE, Mfe: p.MFE,
	}
}

func (s *Service) recordPeriod(p learn.Period) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.ln.RecordPeriod(ctx, &learningv1.RecordPeriodRequest{Period: toProtoPeriod(p)}); err != nil {
		s.log.Warn("record period", "sleeve", p.Sleeve, "method", p.Method, "err", err)
	}
}
