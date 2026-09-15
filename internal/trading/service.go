package trading

import (
	"log/slog"
	"sync"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/internal/policy"
	"aperture/pkg/broker"
	"aperture/pkg/config"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/live"
	"aperture/pkg/llm"
	"aperture/pkg/research"
)

type Config struct {
	Bind           string
	MarketdataAddr string
	LearningAddr   string
	SentimentAddr  string
	Autostart      bool
	CampaignDays   int
}

func LoadConfig() Config {
	return Config{
		Bind:           config.String("TRADING_BIND", ":9082"),
		MarketdataAddr: config.String("MARKETDATA_ADDR", "127.0.0.1:9081"),
		LearningAddr:   config.String("LEARNING_ADDR", "127.0.0.1:9083"),
		SentimentAddr:  config.String("SENTIMENT_ADDR", "127.0.0.1:9084"),
		Autostart:      config.Bool("AUTOSTART_CAMPAIGN"),
		CampaignDays:   config.Int("CAMPAIGN_DAYS", 30),
	}
}

type Deps struct {
	MarketData marketdatav1.MarketDataServiceClient
	Learning   learningv1.LearningServiceClient
	Sentiment  sentimentv1.SentimentServiceClient
	LLM        llm.Completer
	Log        *slog.Logger
	Now        func() time.Time
	Cfg        Config
	// IPS binds the book to a policy at construction (replays). The live
	// desk binds through BindIPS when a statement is put.
	IPS *ips.IPS
}

type Service struct {
	tradingv1.UnimplementedTradingServiceServer
	md  marketdatav1.MarketDataServiceClient
	ln  learningv1.LearningServiceClient
	sn  sentimentv1.SentimentServiceClient
	llm llm.Completer
	log *slog.Logger
	now func() time.Time
	cfg Config

	mu           sync.Mutex
	cash         float64
	eq0          float64
	pos          map[string]*commonv1.Position
	open         []*commonv1.PaperTrade
	w            []*commonv1.StrategyWeight
	camp         campaign
	benchStart   map[string]float64
	beatWins     map[string]int
	beatN        map[string]int
	seq          int
	plan         bookPlan
	analyzing    bool
	peak         float64
	desk         *broker.Paper
	exc          map[string]*excursion
	inv          *research.Desk
	lastTurnover float64
	lastFills    int
	// Turnover rail: new-buy notional filled in the current IST session.
	dayKey       string
	dayBuys      float64
	dayCapLogged bool
	// Core sleeve under the bound IPS. Policy decides the tickets
	// (internal/policy.Rebalance); this service only executes them.
	ips           *ips.IPS
	coreQty       map[string]float64
	coreLastRebal string
	corePending   bool
	coreLogKey    string
	coreHaltLog   bool
	lastCoreFills int
	lastSatFills  int
	lastCorePlan  policy.Plan
}

type campaign struct {
	active, auto bool
	start, end   time.Time
	days         int
	ticks        int
}

func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	if config.Bool("AUTOPILOT_LIVE_IND") {
		log.Warn("AUTOPILOT_LIVE_IND is ignored; fills stay on the paper book. INDstocks is quotes and history only. Live routing is compile-time off.")
	}
	log.Info("paper book",
		"mode", "positional-cash",
		"scalp", config.Bool("SCALP_MODE"),
		"name_cap", costs.NameCap,
		"gross_cap", broker.GrossCap,
		"cash_buffer", broker.CashBuffer,
		"sector_cap", broker.SectorCap,
		"drawdown_halt", costs.DrawdownHalt,
		"live_orders", live.OrdersMode(),
		"live_ready", live.Ready(),
	)
	s := &Service{
		md: d.MarketData, ln: d.Learning, sn: d.Sentiment, llm: d.LLM,
		log: log, now: now, cfg: d.Cfg,
		cash: costs.StartCash, eq0: costs.StartCash, peak: costs.StartCash,
		pos:        map[string]*commonv1.Position{},
		benchStart: map[string]float64{},
		beatWins:   map[string]int{},
		beatN:      map[string]int{},
		desk:       broker.NewPaper(log),
		inv:        research.NewDesk(""),
		coreQty:    map[string]float64{},
	}
	if d.IPS != nil {
		s.BindIPS(*d.IPS)
	}
	return s
}

func (s *Service) SeedWeights(weights []*commonv1.StrategyWeight) {
	s.mu.Lock()
	s.w = weights
	s.mu.Unlock()
}

// BindIPS puts the book under a statement. The core builds at the next
// open session and rebalances on the statement's calendar. Re-binding a
// different hash restarts the calendar; the same hash is a no-op.
func (s *Service) BindIPS(p ips.IPS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ips != nil && s.ips.Hash() == p.Hash() {
		return
	}
	cp := p
	s.ips = &cp
	s.coreLastRebal = ""
	s.corePending = false
	s.coreHaltLog = false
	s.log.Info("IPS_BOUND", "id", p.ID, "hash", p.Hash(), "line", p.Line(), "halt_at", broker.HaltThreshold(broker.Snapshot{HaltAt: p.MaxDD}))
}

// IPS returns the bound statement, if any.
func (s *Service) IPS() *ips.IPS {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ips == nil {
		return nil
	}
	cp := *s.ips
	return &cp
}
