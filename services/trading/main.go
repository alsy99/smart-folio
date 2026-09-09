package main

import (
	"context"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	commonv1 "aperture/go/gen/common/v1"
	learningv1 "aperture/go/gen/learning/v1"
	marketdatav1 "aperture/go/gen/marketdata/v1"
	sentimentv1 "aperture/go/gen/sentiment/v1"
	tradingv1 "aperture/go/gen/trading/v1"
	"aperture/pkg/excess"
	"aperture/pkg/grpcx"
	"aperture/pkg/marketclock"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

const startCash = 1_000_000.0
const commissionBps = 5.0
const slippageBps = 4.0

type openTrade struct {
	*commonv1.PaperTrade
}

type server struct {
	tradingv1.UnimplementedTradingServiceServer
	md   marketdatav1.MarketDataServiceClient
	ln   learningv1.LearningServiceClient
	sn   sentimentv1.SentimentServiceClient
	mu   sync.Mutex
	cash float64
	eq0  float64
	pos  map[string]*commonv1.Position
	open []*commonv1.PaperTrade
	w    []*commonv1.StrategyWeight
	camp struct {
		active, auto bool
		start, end   time.Time
		days         int
		ticks        int
	}
	benchStart map[string]float64
	beatWins   map[string]int
	beatN      map[string]int
	seq        int
}

func envAddr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (s *server) GetPortfolio(ctx context.Context, _ *tradingv1.GetPortfolioRequest) (*tradingv1.GetPortfolioResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	eq := s.markLocked()
	ret := excess.PortfolioReturn(eq, s.eq0)
	var pos []*commonv1.Position
	for _, p := range s.pos {
		if p.Qty == 0 {
			continue
		}
		p.Weight = 0
		if eq > 0 {
			p.Weight = p.MarketValue / eq
		}
		pos = append(pos, p)
	}
	return &tradingv1.GetPortfolioResponse{
		Cash: s.cash, Equity: eq, StartEquity: s.eq0, ReturnPct: ret * 100,
		Positions: pos, OpenTrades: s.open, Weights: s.w,
	}, nil
}

func (s *server) GetCampaign(ctx context.Context, _ *tradingv1.GetCampaignRequest) (*tradingv1.GetCampaignResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	elapsed := 0
	if s.camp.active {
		elapsed = int(now.Sub(s.camp.start).Hours() / 24)
	}
	return &tradingv1.GetCampaignResponse{
		Active: s.camp.active, Autopilot: s.camp.auto, MarketOpen: marketclock.IsOpen(now),
		StartedAtUnixMs: s.camp.start.UnixMilli(), EndsAtUnixMs: s.camp.end.UnixMilli(),
		DaysElapsed: int32(elapsed), DaysTotal: int32(s.camp.days),
		SessionStatus: marketclock.SessionStatus(now), Ticks: int32(s.camp.ticks),
		ClockOverride: marketclock.OverrideOpen(),
	}, nil
}

func (s *server) StartCampaign(ctx context.Context, req *tradingv1.StartCampaignRequest) (*tradingv1.GetCampaignResponse, error) {
	days := int(req.Days)
	if days <= 0 {
		days = 30
		if v := os.Getenv("CAMPAIGN_DAYS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				days = n
			}
		}
	}
	now := time.Now()
	s.mu.Lock()
	s.cash = startCash
	s.eq0 = startCash
	s.pos = map[string]*commonv1.Position{}
	s.open = nil
	s.camp.active = true
	s.camp.auto = true
	s.camp.start = now
	s.camp.end = now.Add(time.Duration(days) * 24 * time.Hour)
	s.camp.days = days
	s.camp.ticks = 0
	s.beatWins = map[string]int{}
	s.beatN = map[string]int{}
	s.mu.Unlock()
	s.captureBenchStart(ctx)
	return s.GetCampaign(ctx, &tradingv1.GetCampaignRequest{})
}

func (s *server) captureBenchStart(ctx context.Context) {
	var syms []string
	for _, b := range universe.Benchmarks() {
		syms = append(syms, b.Symbol)
	}
	q, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: syms})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.benchStart = map[string]float64{}
	for _, qq := range q.Quotes {
		s.benchStart[qq.Symbol] = qq.Last
	}
}

func (s *server) SetAutopilot(ctx context.Context, req *tradingv1.SetAutopilotRequest) (*tradingv1.SetAutopilotResponse, error) {
	s.mu.Lock()
	s.camp.auto = req.Enabled
	s.mu.Unlock()
	return &tradingv1.SetAutopilotResponse{Enabled: req.Enabled}, nil
}

func (s *server) SetStrategyWeights(ctx context.Context, req *tradingv1.SetStrategyWeightsRequest) (*tradingv1.SetStrategyWeightsResponse, error) {
	s.mu.Lock()
	s.w = req.Weights
	s.mu.Unlock()
	return &tradingv1.SetStrategyWeightsResponse{}, nil
}

func (s *server) GetBenchmarks(ctx context.Context, _ *tradingv1.GetBenchmarksRequest) (*tradingv1.GetBenchmarksResponse, error) {
	s.mu.Lock()
	eq := s.markLocked()
	rP := excess.PortfolioReturn(eq, s.eq0)
	start := s.camp.start
	days := math.Max(1, time.Since(start).Hours()/24)
	if !s.camp.active {
		days = 1
	}
	bs := s.benchStart
	bw, bn := s.beatWins, s.beatN
	s.mu.Unlock()

	var syms []string
	for _, b := range universe.Benchmarks() {
		syms = append(syms, b.Symbol)
	}
	q, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: syms})
	if err != nil {
		return nil, err
	}
	lasts := map[string]float64{}
	for _, qq := range q.Quotes {
		lasts[qq.Symbol] = qq.Last
	}
	var points []*commonv1.BenchmarkPoint
	for _, b := range universe.Benchmarks() {
		st := bs[b.Symbol]
		if st == 0 {
			st = lasts[b.Symbol]
		}
		rB := 0.0
		if st != 0 {
			rB = lasts[b.Symbol]/st - 1
		}
		ex := excess.Excess(rP, rB)
		ann := excess.Annualized(ex, days)
		br := 0.0
		if bn[b.Symbol] > 0 {
			br = float64(bw[b.Symbol]) / float64(bn[b.Symbol])
		}
		points = append(points, &commonv1.BenchmarkPoint{
			Id: b.Symbol, Name: b.Name, Last: lasts[b.Symbol], Start: st,
			ReturnPct: rB * 100, ExcessPct: ex * 100, ExcessAnnPct: ann * 100,
			OnTrack: excess.OnTrack(ann), BeatRate: br,
		})
	}
	return &tradingv1.GetBenchmarksResponse{Benchmarks: points, PortfolioReturnPct: rP * 100}, nil
}

func (s *server) Tick(ctx context.Context, _ *tradingv1.TickRequest) (*tradingv1.TickResponse, error) {
	s.mu.Lock()
	if !s.camp.active {
		s.mu.Unlock()
		return &tradingv1.TickResponse{Skipped: true, Reason: "campaign inactive"}, nil
	}
	if !s.camp.auto {
		s.mu.Unlock()
		return &tradingv1.TickResponse{Skipped: true, Reason: "autopilot off"}, nil
	}
	if time.Now().After(s.camp.end) {
		s.mu.Unlock()
		return &tradingv1.TickResponse{Skipped: true, Reason: "campaign ended"}, nil
	}
	s.mu.Unlock()
	if !marketclock.IsOpen(time.Now()) {
		return &tradingv1.TickResponse{Skipped: true, Reason: "market closed"}, nil
	}

	quotes, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{})
	if err != nil {
		return nil, err
	}
	last := map[string]float64{}
	for _, q := range quotes.Quotes {
		last[q.Symbol] = q.Last
	}

	wresp, err := s.ln.GetWeights(ctx, &learningv1.GetWeightsRequest{})
	if err == nil && wresp != nil {
		s.mu.Lock()
		s.w = wresp.Weights
		s.mu.Unlock()
	}

	inv, _ := s.sn.GetInvestigations(ctx, &sentimentv1.GetInvestigationsRequest{})
	scores, _ := s.sn.ScoreSymbols(ctx, &sentimentv1.ScoreSymbolsRequest{Symbols: universe.EquitySymbols()})
	scoreMap := map[string]float64{}
	if scores != nil {
		for _, sc := range scores.Scores {
			scoreMap[sc.Symbol] = sc.Score
		}
	}
	standAside := map[string]bool{}
	tilts := map[string]float64{}
	invIDs := map[string][]string{}
	if inv != nil {
		for _, r := range inv.Reports {
			for _, sym := range r.Symbols {
				if r.StandAside {
					standAside[sym] = true
				}
				invIDs[sym] = append(invIDs[sym], r.Id)
			}
			for _, t := range r.StrategyImplications {
				tilts[t.StrategyId] += t.Tilt
			}
		}
	}

	s.mu.Lock()
	copiedW := s.w
	s.mu.Unlock()

	baseW := map[string]float64{}
	for _, w := range copiedW {
		baseW[w.StrategyId] = w.Weight
	}
	if len(baseW) == 0 {
		eqw := 1.0 / float64(len(strategies.IDs()))
		for _, id := range strategies.IDs() {
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
	for id := range baseW {
		baseW[id] /= norm
	}

	picked := map[string]strategies.Signal{}
	for _, sym := range universe.EquitySymbols() {
		if standAside[sym] {
			continue
		}
		picked[sym] = s.bestSignal(ctx, sym, scoreMap[sym], baseW)
	}

	s.mu.Lock()
	fills, closes := 0, 0
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
		move := (px - t.Entry) / t.Entry
		if t.Side == "SELL" {
			move = -move
		}
		hold := time.Now().UnixMilli() - t.OpenedAtUnixMs
		if move > 0.012 || move < -0.008 || hold > 45*1000 {
			s.closeLocked(t, px, niftyRet)
			closes++
			go s.record(t)
		} else {
			remain = append(remain, t)
		}
	}
	s.open = remain

	eq := s.markLocked()
	for _, sym := range universe.EquitySymbols() {
		if standAside[sym] {
			continue
		}
		px := last[sym]
		if px == 0 {
			continue
		}
		sig := picked[sym]
		if sig.Direction == 0 || sig.Score < 0.25 {
			continue
		}
		if s.hasOpen(sym) {
			continue
		}
		w := baseW[sig.StrategyID]
		notional := eq * 0.08 * sig.Score * (0.5 + w)
		if notional < 15000 {
			notional = 15000
		}
		if notional > s.cash*0.25 {
			notional = s.cash * 0.25
		}
		if notional < 5000 || s.cash < notional {
			continue
		}
		qty := math.Floor(notional / px)
		if qty < 1 {
			continue
		}
		fillPx := px * (1 + slippageBps/1e4)
		if sig.Direction < 0 {
			continue
		}
		cost := qty * fillPx
		comm := cost * commissionBps / 1e4
		s.cash -= cost + comm
		s.seq++
		t := &commonv1.PaperTrade{
			Id: "t-" + strconv.Itoa(s.seq), Symbol: sym, Side: "BUY", StrategyId: sig.StrategyID,
			Qty: qty, Entry: fillPx, Open: true, OpenedAtUnixMs: time.Now().UnixMilli(),
			InvestigationIds: invIDs[sym],
		}
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
	}

	s.camp.ticks++
	eq = s.markLocked()
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
	s.mu.Unlock()
	return &tradingv1.TickResponse{Fills: int32(fills), Closes: int32(closes), Equity: eq}, nil
}

func (s *server) hasOpen(sym string) bool {
	for _, t := range s.open {
		if t.Symbol == sym && t.Open {
			return true
		}
	}
	return false
}

func (s *server) closeLocked(t *commonv1.PaperTrade, px, niftyRet float64) {
	fill := px * (1 - slippageBps/1e4)
	proceeds := t.Qty * fill
	comm := proceeds * commissionBps / 1e4
	s.cash += proceeds - comm
	t.Exit = fill
	t.Open = false
	t.ClosedAtUnixMs = time.Now().UnixMilli()
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

func (s *server) record(t *commonv1.PaperTrade) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = s.ln.RecordTrade(ctx, &learningv1.RecordTradeRequest{Trade: t})
}

func (s *server) updateMarksLocked(last map[string]float64) {
	for sym, p := range s.pos {
		if px, ok := last[sym]; ok {
			p.Last = px
			p.MarketValue = p.Qty * px
			p.Pnl = (px - p.AvgPrice) * p.Qty
		}
	}
}

func (s *server) markLocked() float64 {
	eq := s.cash
	for _, p := range s.pos {
		eq += p.MarketValue
	}
	return eq
}

func (s *server) bestSignal(ctx context.Context, sym string, sent float64, weights map[string]float64) strategies.Signal {
	type pack struct {
		c, h, l []float64
	}
	load := func(interval string) pack {
		resp, err := s.md.GetBars(ctx, &marketdatav1.GetBarsRequest{Symbol: sym, Interval: interval, Count: 40})
		if err != nil {
			return pack{}
		}
		c := make([]float64, len(resp.Bars))
		h := make([]float64, len(resp.Bars))
		l := make([]float64, len(resp.Bars))
		for i, b := range resp.Bars {
			c[i], h[i], l[i] = b.Close, b.High, b.Low
		}
		return pack{c, h, l}
	}
	p15, p5, p1h, pd := load("15m"), load("5m"), load("1h"), load("1d")
	sigs := []strategies.Signal{
		strategies.SMACross(sym, p15.c),
		strategies.Momentum(sym, p5.c),
		strategies.MeanRevert(sym, p15.c),
		strategies.Breakout(sym, p1h.h, p1h.c),
		strategies.Swing(sym, pd.c),
		strategies.Sentiment(sym, sent),
		strategies.OpeningRangeBreak(sym, p5.h, p5.l, p5.c),
	}
	best := strategies.Signal{}
	bestScore := -1.0
	for _, sg := range sigs {
		w := weights[sg.StrategyID]
		sc := sg.Score * (0.3 + w)
		if sc > bestScore {
			bestScore = sc
			best = sg
		}
	}
	return best
}

func (s *server) loop() {
	tickEvery := 8 * time.Second
	for {
		time.Sleep(tickEvery)
		s.mu.Lock()
		run := s.camp.active && s.camp.auto
		s.mu.Unlock()
		if !run {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		_, _ = s.Tick(ctx, &tradingv1.TickRequest{})
		cancel()
	}
}

func main() {
	md := marketdatav1.NewMarketDataServiceClient(grpcx.MustDial(envAddr("MARKETDATA_ADDR", "127.0.0.1:9081")))
	ln := learningv1.NewLearningServiceClient(grpcx.MustDial(envAddr("LEARNING_ADDR", "127.0.0.1:9083")))
	sn := sentimentv1.NewSentimentServiceClient(grpcx.MustDial(envAddr("SENTIMENT_ADDR", "127.0.0.1:9084")))
	s := &server{
		md: md, ln: ln, sn: sn, cash: startCash, eq0: startCash,
		pos: map[string]*commonv1.Position{}, benchStart: map[string]float64{},
		beatWins: map[string]int{}, beatN: map[string]int{},
	}
	wresp, err := ln.GetWeights(context.Background(), &learningv1.GetWeightsRequest{})
	if err == nil {
		s.w = wresp.Weights
	}
	go s.loop()
	if os.Getenv("AUTOSTART_CAMPAIGN") == "true" {
		go func() {
			time.Sleep(2 * time.Second)
			_, _ = s.StartCampaign(context.Background(), &tradingv1.StartCampaignRequest{Days: 30})
			log.Printf("autostarted 30-day paper campaign")
		}()
	}

	addr := os.Getenv("TRADING_BIND")
	if addr == "" {
		addr = ":9082"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer()
	tradingv1.RegisterTradingServiceServer(gs, s)
	log.Printf("trading listening on %s", addr)
	log.Fatal(gs.Serve(lis))
}
