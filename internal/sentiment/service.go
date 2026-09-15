package sentiment

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	commonv1 "aperture/gen/common/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	"aperture/pkg/config"
	"aperture/pkg/indstocks"
	"aperture/pkg/llm"
	"aperture/pkg/research"
	"aperture/pkg/universe"
)

type Config struct {
	Bind           string
	MaxWorkers     int
	RefreshEvery   time.Duration
	MinMateriality float64
}

func LoadConfig() Config {
	return Config{
		Bind:           config.String("SENTIMENT_BIND", ":9084"),
		MaxWorkers:     config.Int("MAX_INVESTIGATION_WORKERS", 16),
		RefreshEvery:   config.Duration("NEWS_REFRESH_SECONDS", 900*time.Second),
		MinMateriality: config.Float("INVESTIGATION_MATERIALITY_MIN", 0.35),
	}
}

type Service struct {
	sentimentv1.UnimplementedSentimentServiceServer
	log     *slog.Logger
	cfg     Config
	llm     llm.Completer
	desk    *research.Desk
	sources []Source
	clock   func() time.Time
	refresh sync.Mutex

	mu          sync.RWMutex
	news        []article
	reports     []*commonv1.InvestigationReport
	scores      map[string]*commonv1.SentimentScore
	lastRefresh time.Time
	mode        string
	workers     int
	maxWorkers  int
}

type Completer = llm.Completer

func New(cfg Config, log *slog.Logger, completer Completer, sources []Source) *Service {
	if log == nil {
		log = slog.Default()
	}
	if completer == nil {
		completer = llm.FromEnv()
	}
	if len(sources) == 0 {
		sources = DefaultSources()
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 16
	}
	if cfg.RefreshEvery <= 0 {
		cfg.RefreshEvery = 15 * time.Minute
	}
	names := make([]string, 0, len(sources))
	for _, src := range sources {
		names = append(names, src.Name())
	}
	log.Info("sentiment", "llm", completer.Enabled(), "sources", names,
		"llm_max", llm.MaxCalls(), "tokens_day", llm.MaxTokens())
	return &Service{
		log: log, cfg: cfg, llm: completer, sources: sources, clock: time.Now,
		scores:     map[string]*commonv1.SentimentScore{},
		maxWorkers: cfg.MaxWorkers,
		desk:       research.NewDesk(""),
	}
}

func (s *Service) ScoreSymbols(ctx context.Context, req *sentimentv1.ScoreSymbolsRequest) (*sentimentv1.ScoreSymbolsResponse, error) {
	s.maybeRefresh(ctx, false)
	s.mu.RLock()
	defer s.mu.RUnlock()
	syms := req.Symbols
	if len(syms) == 0 {
		syms = universe.EquitySymbols()
	}
	out := make([]*commonv1.SentimentScore, 0, len(syms))
	for _, sym := range syms {
		if sc, ok := s.scores[sym]; ok {
			out = append(out, sc)
		} else {
			out = append(out, &commonv1.SentimentScore{Symbol: sym, Label: "neutral", Confidence: 0.4})
		}
	}
	return &sentimentv1.ScoreSymbolsResponse{Scores: out}, nil
}

func (s *Service) ListNews(ctx context.Context, req *sentimentv1.ListNewsRequest) (*sentimentv1.ListNewsResponse, error) {
	// Negative limit means "return all" and also force a live ingest (campaign restart).
	s.maybeRefresh(ctx, req.GetLimit() < 0)
	s.mu.RLock()
	defer s.mu.RUnlock()
	lim := int(req.Limit)
	if lim <= 0 || lim > len(s.news) {
		lim = len(s.news)
	}
	items := make([]*commonv1.NewsItem, 0, lim)
	for i := 0; i < lim; i++ {
		items = append(items, toNewsItem(s.news[i]))
	}
	return &sentimentv1.ListNewsResponse{Items: items, Mode: s.mode, LastRefreshUnixMs: s.lastRefresh.UnixMilli()}, nil
}

func (s *Service) GetInvestigations(ctx context.Context, _ *sentimentv1.GetInvestigationsRequest) (*sentimentv1.GetInvestigationsResponse, error) {
	s.maybeRefresh(ctx, false)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &sentimentv1.GetInvestigationsResponse{
		Reports:           s.reports,
		WorkersActive:     int32(s.workers),
		WorkersMax:        int32(s.maxWorkers),
		LastRefreshUnixMs: s.lastRefresh.UnixMilli(),
		Mode:              s.mode,
	}, nil
}

func (s *Service) maybeRefresh(ctx context.Context, force bool) {
	if !force {
		s.mu.RLock()
		fresh := time.Since(s.lastRefresh) < s.cfg.RefreshEvery && len(s.reports) > 0
		s.mu.RUnlock()
		if fresh {
			return
		}
	}
	s.Refresh(ctx, force)
}

func (s *Service) Refresh(ctx context.Context, force bool) {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if !force {
		s.mu.RLock()
		fresh := time.Since(s.lastRefresh) < s.cfg.RefreshEvery && len(s.reports) > 0
		s.mu.RUnlock()
		if fresh {
			return
		}
	}
	s.log.Info("news refresh begin", "force", force)
	now := s.clock()
	items, mode := gather(ctx, s.sources, now)
	for _, n := range items {
		s.log.Info("news item",
			"mode", mode,
			"provider", n.Provider,
			"symbols", n.Symbols,
			"published", n.Published.UTC().Format(time.RFC3339),
			"title", n.Title,
		)
	}
	clusters := clusterNews(items)
	maxW := s.cfg.MaxWorkers
	minMat := s.cfg.MinMateriality

	var jobs []ranked
	for k, ns := range clusters {
		mat := materiality(ns)
		if mat >= minMat {
			jobs = append(jobs, ranked{k, ns, mat})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].mat > jobs[j].mat })
	if len(jobs) > maxW {
		jobs = jobs[:maxW]
	}
	s.log.Info("news analysis start",
		"mode", mode,
		"ingested", len(items),
		"clusters", len(clusters),
		"jobs", len(jobs),
		"min_materiality", minMat,
	)

	s.mu.Lock()
	s.workers = len(jobs)
	s.maxWorkers = maxW
	s.mu.Unlock()

	reports := s.runQueue(ctx, jobs, mode)

	scores := map[string]*commonv1.SentimentScore{}
	for _, r := range reports {
		for _, sym := range r.Symbols {
			cur := scores[sym]
			if cur == nil {
				cur = &commonv1.SentimentScore{Symbol: sym}
			}
			cur.Score += r.Score * r.Confidence
			cur.Confidence = (cur.Confidence + r.Confidence) / 2
			if r.Thesis != "" {
				cur.Drivers = append(cur.Drivers, r.Headline)
			}
			scores[sym] = cur
		}
	}
	for _, sc := range scores {
		switch {
		case sc.Score > 0.15:
			sc.Label = "bullish"
		case sc.Score < -0.15:
			sc.Label = "bearish"
		default:
			sc.Label = "neutral"
		}
		if sc.Confidence < 0.35 {
			sc.Confidence = 0.35
		}
		if sc.Confidence > 0.92 {
			sc.Confidence = 0.92
		}
	}

	reports = indstocks.OverlayPulse(ctx, scores, reports)

	s.mu.Lock()
	s.news = items
	s.reports = reports
	s.scores = scores
	s.lastRefresh = s.clock()
	s.mode = mode
	s.workers = 0
	s.mu.Unlock()
}

type ranked struct {
	key string
	ns  []article
	mat float64
}

func (s *Service) runQueue(ctx context.Context, jobs []ranked, mode string) []*commonv1.InvestigationReport {
	if s.desk == nil {
		s.desk = research.NewDesk("")
	}
	s.desk.Now = s.clock
	day := llm.ISTDay(s.clock())
	reports := make([]*commonv1.InvestigationReport, 0, len(jobs))
	for i, job := range jobs {
		headline := ""
		sym := ""
		mapped := false
		if len(job.ns) > 0 {
			headline = job.ns[0].Title
			if len(job.ns[0].Symbols) > 0 {
				sym = job.ns[0].Symbols[0]
				mapped = true
			}
		}
		key := research.CacheKey(sym, day, headline)
		s.log.Info("investigation queued", "i", i, "key", key, "mapped", mapped)
		if rec, ok := s.desk.Lookup(key); ok {
			s.log.Info("investigation cache hit", "id", rec.ID, "key", key)
		}
		useLLM := mapped && s.desk.AllowLLM()
		id := research.IDForKey(key)
		reports = append(reports, investigate(ctx, s.llm, s.desk, job.ns, mode, id, useLLM))
	}
	return reports
}

func (s *Service) Loop(ctx context.Context) {
	t := time.NewTicker(s.cfg.RefreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Refresh(ctx, true)
		}
	}
}
