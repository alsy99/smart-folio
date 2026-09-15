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
	"aperture/pkg/config"
	"aperture/pkg/costs"
	"aperture/pkg/llm"
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

	mu         sync.Mutex
	cash       float64
	eq0        float64
	pos        map[string]*commonv1.Position
	open       []*commonv1.PaperTrade
	w          []*commonv1.StrategyWeight
	camp       campaign
	benchStart map[string]float64
	beatWins   map[string]int
	beatN      map[string]int
	seq        int
	plan       bookPlan
	analyzing  bool
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
		log.Warn("AUTOPILOT_LIVE_IND is ignored; fills stay on the paper book. INDstocks is quotes and history only.")
	}
	log.Info("paper book",
		"mode", "positional-cash",
		"scalp", config.Bool("SCALP_MODE"),
		"name_cap", costs.NameCap,
		"drawdown_halt", costs.DrawdownHalt,
	)
	return &Service{
		md: d.MarketData, ln: d.Learning, sn: d.Sentiment, llm: d.LLM,
		log: log, now: now, cfg: d.Cfg,
		cash: costs.StartCash, eq0: costs.StartCash,
		pos:        map[string]*commonv1.Position{},
		benchStart: map[string]float64{},
		beatWins:   map[string]int{},
		beatN:      map[string]int{},
	}
}

func (s *Service) SeedWeights(weights []*commonv1.StrategyWeight) {
	s.mu.Lock()
	s.w = weights
	s.mu.Unlock()
}
