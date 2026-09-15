package trading

import (
	"context"
	"math"
	"time"

	commonv1 "aperture/gen/common/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	tradingv1 "aperture/gen/trading/v1"
	pubcamp "aperture/pkg/campaign"
	"aperture/pkg/costs"
	"aperture/pkg/excess"
	"aperture/pkg/live"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) GetPortfolio(_ context.Context, _ *tradingv1.GetPortfolioRequest) (*tradingv1.GetPortfolioResponse, error) {
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

func (s *Service) GetCampaign(_ context.Context, _ *tradingv1.GetCampaignRequest) (*tradingv1.GetCampaignResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
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

func (s *Service) StartCampaign(ctx context.Context, req *tradingv1.StartCampaignRequest) (*tradingv1.GetCampaignResponse, error) {
	days := int(req.Days)
	if days <= 0 {
		days = s.cfg.CampaignDays
		if days <= 0 {
			days = 30
		}
	}
	now := s.now()
	s.mu.Lock()
	s.cash = costs.StartCash
	s.eq0 = costs.StartCash
	s.peak = costs.StartCash
	s.pos = map[string]*commonv1.Position{}
	s.open = nil
	s.exc = map[string]*excursion{}
	if s.desk != nil {
		s.desk.Reset()
	}
	s.camp.active = true
	s.camp.auto = true
	s.camp.start = now
	s.camp.end = now.Add(time.Duration(days) * 24 * time.Hour)
	s.camp.days = days
	s.camp.ticks = 0
	s.beatWins = map[string]int{}
	s.beatN = map[string]int{}
	s.lastTurnover = 0
	s.lastFills = 0
	s.lastCoreFills, s.lastSatFills = 0, 0
	s.coreQty = map[string]float64{}
	s.coreLastRebal = ""
	s.corePending = false
	s.coreHaltLog = false
	s.period = nil
	s.periods = nil
	s.mu.Unlock()
	s.captureBenchStart(ctx)
	s.log.Info("campaign started", "days", days)
	return s.GetCampaign(ctx, &tradingv1.GetCampaignRequest{})
}

func (s *Service) captureBenchStart(ctx context.Context) {
	var syms []string
	for _, b := range universe.Benchmarks() {
		syms = append(syms, b.Symbol)
	}
	q, err := s.md.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: syms})
	if err != nil {
		s.log.Warn("benchmark start quotes", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.benchStart = map[string]float64{}
	for _, qq := range q.Quotes {
		s.benchStart[qq.Symbol] = qq.Last
	}
}

func (s *Service) SetAutopilot(_ context.Context, req *tradingv1.SetAutopilotRequest) (*tradingv1.SetAutopilotResponse, error) {
	s.mu.Lock()
	s.camp.auto = req.Enabled
	s.mu.Unlock()
	if req.Enabled {
		_ = live.Clear()
	} else {
		_ = live.Trip()
	}
	return &tradingv1.SetAutopilotResponse{Enabled: req.Enabled}, nil
}

func (s *Service) SetStrategyWeights(_ context.Context, req *tradingv1.SetStrategyWeightsRequest) (*tradingv1.SetStrategyWeightsResponse, error) {
	if pubcamp.IsFrozen() {
		return nil, status.Error(codes.FailedPrecondition, "public 30-day campaign is frozen; weights stay equal until the published window ends")
	}
	s.mu.Lock()
	s.w = req.Weights
	s.mu.Unlock()
	return &tradingv1.SetStrategyWeightsResponse{}, nil
}

func (s *Service) GetBenchmarks(ctx context.Context, _ *tradingv1.GetBenchmarksRequest) (*tradingv1.GetBenchmarksResponse, error) {
	s.mu.Lock()
	eq := s.markLocked()
	rP := excess.PortfolioReturn(eq, s.eq0)
	start := s.camp.start
	days := math.Max(1, s.now().Sub(start).Hours()/24)
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
