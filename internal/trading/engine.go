package trading

import (
	"context"
	"math"
	"strconv"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/asof"
	"aperture/pkg/broker"
	"aperture/pkg/config"
	"aperture/pkg/costs"
	"aperture/pkg/excess"
	"aperture/pkg/marketclock"
	"aperture/pkg/research"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"
)

type bookPlan struct {
	at         time.Time
	signals    map[string]strategies.Signal
	standAside map[string]bool
	weights    map[string]float64
	invID      map[string]string
}

func (s *Service) Tick(ctx context.Context, _ *tradingv1.TickRequest) (*tradingv1.TickResponse, error) {
	s.mu.Lock()
	if !s.camp.active {
		s.mu.Unlock()
		return &tradingv1.TickResponse{Skipped: true, Reason: "campaign inactive"}, nil
	}
	now := s.now()
	if now.After(s.camp.end) {
		s.mu.Unlock()
		return &tradingv1.TickResponse{Skipped: true, Reason: "campaign ended"}, nil
	}
	auto := s.camp.auto
	s.mu.Unlock()

	if err := s.Plan(ctx); err != nil {
		return nil, err
	}
	if !marketclock.IsOpen(s.now()) {
		s.log.Info("paper execute skipped", "reason", "market closed", "session", marketclock.SessionStatus(s.now()))
		return &tradingv1.TickResponse{Skipped: true, Reason: "market closed"}, nil
	}
	if !auto {
		return &tradingv1.TickResponse{Skipped: true, Reason: "autopilot off"}, nil
	}
	return s.Execute(ctx)
}

func (s *Service) Plan(ctx context.Context) error {
	if wresp, err := s.ln.GetWeights(ctx, &learningv1.GetWeightsRequest{}); err != nil {
		s.log.Warn("weights", "err", err)
	} else if wresp != nil {
		s.mu.Lock()
		s.w = wresp.Weights
		s.mu.Unlock()
	}

	inv, err := s.sn.GetInvestigations(ctx, &sentimentv1.GetInvestigationsRequest{})
	if err != nil {
		s.log.Warn("investigations", "err", err)
	}
	bar := s.now()
	var reports []*commonv1.InvestigationReport
	if inv != nil {
		reports = asof.Reports(inv.Reports, bar)
	}
	scoreMap := asof.Scores(reports)
	standAside := map[string]bool{}
	tilts := map[string]float64{}
	invID := map[string]string{}
	invRank := map[string]float64{}
	newsBySym := map[string]string{}
	for _, r := range reports {
		rank := math.Abs(r.Score) * r.Confidence
		for _, sym := range r.Symbols {
			if r.StandAside {
				standAside[sym] = true
			}
			if rank >= invRank[sym] && r.Id != "" {
				invRank[sym] = rank
				invID[sym] = r.Id
			}
			newsBySym[sym] += r.Headline + " " + r.Thesis + " "
		}
		for _, t := range r.StrategyImplications {
			tilts[t.StrategyId] += t.Tilt
		}
	}

	s.mu.Lock()
	copiedW := s.w
	s.mu.Unlock()

	baseW := map[string]float64{}
	for _, w := range copiedW {
		if skipSubSession(w.StrategyId) {
			continue
		}
		baseW[w.StrategyId] = w.Weight
	}
	if len(baseW) == 0 {
		ids := strategies.IDs()
		eqw := 1.0 / float64(len(ids))
		for _, id := range ids {
			baseW[id] = eqw
		}
	}
	for id, t := range tilts {
		baseW[id] = math.Max(0.05, baseW[id]*(1+t))
	}
	norm := 0.0
	for _, v := range baseW {
		norm += v
	}
	if norm > 0 {
		for id := range baseW {
			baseW[id] /= norm
		}
	}

	picked := map[string]strategies.Signal{}
	for _, sym := range universe.EquitySymbols() {
		if standAside[sym] {
			continue
		}
		picked[sym] = s.bestSignal(ctx, sym, scoreMap[sym], baseW)
	}

	asideN := 0
	for _, v := range standAside {
		if v {
			asideN++
		}
	}
	s.mu.Lock()
	s.plan = bookPlan{
		at: s.now(), signals: picked, standAside: standAside, weights: baseW, invID: invID,
	}
	s.mu.Unlock()
	s.log.Info("strategy plan",
		"names", len(picked),
		"stand_aside", asideN,
		"session", marketclock.SessionStatus(s.now()),
	)
	go s.analyzePicks(picked, newsBySym)
	return nil
}

func (s *Service) Execute(ctx context.Context) (*tradingv1.TickResponse, error) {
	quotes, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{})
	if err != nil {
		return nil, err
	}
	last := map[string]float64{}
	for _, q := range quotes.Quotes {
		last[q.Symbol] = q.Last
	}
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	plan := s.plan
	fills, closes := 0, 0
	turnover := 0.0
	s.updateMarksLocked(last)
	niftyRet := 0.0
	if st := s.benchStart["NIFTY50"]; st > 0 && last["NIFTY50"] > 0 {
		niftyRet = last["NIFTY50"]/st - 1
	}

	remain := s.open[:0]
	for _, t := range s.open {
		px := last[t.Symbol]
		if px == 0 {
			remain = append(remain, t)
			continue
		}
		s.markExcursionLocked(t, px)
		move := (px - t.Entry) / t.Entry
		if t.Side == "SELL" {
			move = -move
		}
		hold := now.Sub(time.UnixMilli(t.OpenedAtUnixMs))
		sig := plan.signals[t.Symbol]
		if shouldExit(scalpMode(), hold, move, sig.Direction) {
			s.closeLocked(t, px, niftyRet, s.tradeADV(ctx, t.Symbol))
			closes++
			turnover += t.Qty * t.Exit
			go s.record(t)
		} else {
			remain = append(remain, t)
		}
	}
	s.open = remain

	snap := s.snapshotLocked()
	if broker.Halted(snap) {
		s.desk.NoteHalt(snap)
	} else {
		for _, sym := range universe.EquitySymbols() {
			if plan.standAside[sym] {
				continue
			}
			px := last[sym]
			if px == 0 {
				continue
			}
			sig := plan.signals[sym]
			if sig.Direction <= 0 || sig.Score < 0.25 {
				continue
			}
			if skipSubSession(sig.StrategyID) {
				continue
			}
			if s.hasOpen(sym) {
				continue
			}
			room := broker.Room(snap, sym)
			if room < costs.MinNameNotional {
				continue
			}
			w := plan.weights[sig.StrategyID]
			notional := snap.Equity * costs.NameCap * sig.Score * (0.5 + w)
			if notional > room {
				notional = room
			}
			if notional < costs.MinNameNotional {
				continue
			}
			qty := math.Floor(notional / px)
			if qty < 1 {
				continue
			}
			fillPx := costs.BuyFill(px, qty, s.tradeADV(ctx, sym))
			in := broker.Intent{Symbol: sym, Side: broker.Buy, Qty: qty, Price: fillPx}
			if dec := s.desk.Admit(in, snap); !dec.Allow {
				continue
			}
			cost := qty * fillPx
			charge := costs.RoundTripBuy(cost)
			s.cash -= cost + charge.Total
			s.seq++
			inv := plan.invID[sym]
			if inv == "" {
				inv = s.mintInvestigation(sym, sig)
			}
			t := &commonv1.PaperTrade{
				Id: "t-" + strconv.Itoa(s.seq), Symbol: sym, Side: "BUY", StrategyId: sig.StrategyID,
				Qty: qty, Entry: fillPx, Open: true, OpenedAtUnixMs: now.UnixMilli(),
				InvestigationIds: []string{inv},
				Lesson:           charge.Lesson,
			}
			_ = research.LinkFill(s.invDir(), t.Id, inv)
			s.open = append(s.open, t)
			p := s.pos[sym]
			if p == nil {
				p = &commonv1.Position{Symbol: sym}
				s.pos[sym] = p
			}
			newQty := p.Qty + qty
			p.AvgPrice = (p.AvgPrice*p.Qty + fillPx*qty) / newQty
			p.Qty = newQty
			p.Last = px
			p.MarketValue = p.Qty * px
			p.Pnl = (px - p.AvgPrice) * p.Qty
			fills++
			turnover += qty * fillPx
			snap = s.snapshotLocked()
		}
	}

	s.camp.ticks++
	s.lastTurnover = turnover
	s.lastFills = fills + closes
	eq := s.markLocked()
	rP := excess.PortfolioReturn(eq, s.eq0)
	for _, b := range universe.Benchmarks() {
		st := s.benchStart[b.Symbol]
		if st == 0 || last[b.Symbol] == 0 {
			continue
		}
		rB := last[b.Symbol]/st - 1
		s.beatN[b.Symbol]++
		if rP > rB {
			s.beatWins[b.Symbol]++
		}
	}
	s.log.Info("paper execute", "fills", fills, "closes", closes, "equity", eq)
	return &tradingv1.TickResponse{Fills: int32(fills), Closes: int32(closes), Equity: eq}, nil
}

func (s *Service) record(t *commonv1.PaperTrade) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.ln.RecordTrade(ctx, &learningv1.RecordTradeRequest{Trade: t}); err != nil {
		s.log.Warn("record trade", "id", t.Id, "err", err)
	}
}

func (s *Service) bestSignal(ctx context.Context, sym string, sent float64, weights map[string]float64) strategies.Signal {
	type pack struct {
		c, h, l []float64
	}
	cache := map[string]pack{}
	load := func(interval string) pack {
		if interval == "" || interval == "session" {
			interval = "1d"
		}
		if p, ok := cache[interval]; ok {
			return p
		}
		resp, err := s.md.GetBars(ctx, &marketdatav1.GetBarsRequest{Symbol: sym, Interval: interval, Count: 40})
		if err != nil {
			s.log.Warn("bars", "symbol", sym, "interval", interval, "err", err)
			return pack{}
		}
		c := make([]float64, len(resp.Bars))
		h := make([]float64, len(resp.Bars))
		l := make([]float64, len(resp.Bars))
		for i, b := range resp.Bars {
			c[i], h[i], l[i] = b.Close, b.High, b.Low
		}
		p := pack{c, h, l}
		cache[interval] = p
		return p
	}
	ids := make([]string, 0, len(weights))
	for id := range weights {
		if skipSubSession(id) {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		ids = strategies.IDs()
	}
	best := strategies.Signal{}
	bestScore := -1.0
	for _, id := range ids {
		spec := strategies.Parse(id)
		p := load(spec.Timeframe)
		sg := strategies.Evaluate(spec, sym, strategies.Window{Close: p.c, High: p.h, Low: p.l}, sent)
		w := weights[sg.StrategyID]
		sc := sg.Score * (0.3 + w)
		if sc > bestScore {
			bestScore = sc
			best = sg
		}
	}
	return best
}

func scalpMode() bool {
	return config.Bool("SCALP_MODE")
}

func skipSubSession(id string) bool {
	if scalpMode() {
		return false
	}
	return strategies.IDHoldsUnderSession(id)
}

func shouldExit(scalp bool, hold time.Duration, move float64, signalDir int) bool {
	if scalp {
		return move > costs.ScalpTake || move < costs.ScalpStop || hold > costs.ScalpMaxHold
	}
	if hold < costs.SessionHold {
		return false
	}
	if hold >= costs.MaxHold {
		return true
	}
	return signalDir <= 0
}

func (s *Service) Loop(ctx context.Context) {
	tickEvery := time.Minute
	if scalpMode() {
		tickEvery = 8 * time.Second
	}
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			run := s.camp.active
			s.mu.Unlock()
			if !run {
				continue
			}
			tickCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			if _, err := s.Tick(tickCtx, &tradingv1.TickRequest{}); err != nil {
				s.log.Warn("tick", "err", err)
			}
			cancel()
		}
	}
}

func (s *Service) Autostart(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}
	days := int32(s.cfg.CampaignDays)
	if days <= 0 {
		days = 30
	}
	if _, err := s.StartCampaign(ctx, &tradingv1.StartCampaignRequest{Days: days}); err != nil {
		s.log.Error("autostart campaign", "err", err)
		return
	}
	s.log.Info("autostarted paper campaign", "days", days, "session", marketclock.SessionStatus(s.now()))
}
