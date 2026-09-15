package learning

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	"aperture/pkg/backtest"
	"aperture/pkg/config"
	"aperture/pkg/strategies"
)

type Config struct {
	Bind string
}

func LoadConfig() Config {
	return Config{Bind: config.String("LEARNING_BIND", ":9083")}
}

type stat struct {
	n, wins int
	pnlEMA  float64
	excEMA  float64
}

type Service struct {
	learningv1.UnimplementedLearningServiceServer
	log     *slog.Logger
	mu      sync.Mutex
	journal []*commonv1.JournalEntry
	stats   map[string]*stat
	roster  []string
	report  *learningv1.BacktestReport
}

func rosterDir() string {
	return config.String("ROSTER_DIR", "data/roster")
}

func New(log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	st := map[string]*stat{}
	roster := strategies.IDs()
	for _, id := range roster {
		st[id] = &stat{}
	}
	s := &Service{log: log, stats: st, roster: roster}
	// Boot from the dated roster snapshot if one exists — never from
	// "whatever won last night" in memory.
	if snap, path, err := backtest.LoadLatestRoster(rosterDir()); err == nil {
		s.roster = snap.Roster
		log.Info("roster from snapshot", "file", path, "date", snap.Date, "names", len(snap.Roster))
	}
	return s
}

func (s *Service) Seed(ctx context.Context) {
	// A fresh weekly snapshot already validated the roster OOS — reuse it
	// instead of re-running (and possibly re-promoting) on every boot.
	if snap, path, err := backtest.LoadLatestRoster(rosterDir()); err == nil && !backtest.RosterStale(snap, time.Now(), 7) {
		s.mu.Lock()
		s.roster = snap.Roster
		s.mu.Unlock()
		s.log.Info("roster snapshot is current", "file", path, "date", snap.Date)
		return
	}
	if _, err := s.RunBacktest(ctx, &learningv1.RunBacktestRequest{Years: 5}); err != nil {
		s.log.Error("seed backtest", "err", err)
		return
	}
	s.log.Info("seeded 5-year walk-forward backtest roster")
}

func (s *Service) RecordTrade(_ context.Context, req *learningv1.RecordTradeRequest) (*learningv1.RecordTradeResponse, error) {
	t := req.GetTrade()
	if t == nil {
		return &learningv1.RecordTradeResponse{}, nil
	}
	tags := t.AttributionTags
	lesson := t.Lesson
	if lesson == "" {
		lesson, tags = Attribute(t)
		t.AttributionTags = tags
		t.Lesson = lesson
	}
	entry := &commonv1.JournalEntry{
		Id: "j-" + t.Id, Trade: t, Lesson: lesson, Tags: tags, TsUnixMs: time.Now().UnixMilli(),
	}
	s.mu.Lock()
	s.journal = append([]*commonv1.JournalEntry{entry}, s.journal...)
	st := s.stats[t.StrategyId]
	if st == nil {
		st = &stat{}
		s.stats[t.StrategyId] = st
	}
	st.n++
	if t.Pnl > 0 {
		st.wins++
	}
	const alpha = 0.2
	st.pnlEMA = (1-alpha)*st.pnlEMA + alpha*t.Pnl
	st.excEMA = (1-alpha)*st.excEMA + alpha*t.ExcessReturn
	s.mu.Unlock()
	return &learningv1.RecordTradeResponse{Entry: entry}, nil
}

func (s *Service) ListJournal(_ context.Context, req *learningv1.ListJournalRequest) (*learningv1.ListJournalResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lim := int(req.Limit)
	if lim <= 0 || lim > len(s.journal) {
		lim = len(s.journal)
	}
	return &learningv1.ListJournalResponse{Entries: s.journal[:lim]}, nil
}

func (s *Service) GetWeights(_ context.Context, _ *learningv1.GetWeightsRequest) (*learningv1.GetWeightsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.roster
	if len(ids) == 0 {
		ids = strategies.IDs()
	}
	raw := make([]float64, len(ids))
	sum := 0.0
	for i, id := range ids {
		st := s.stats[id]
		if st == nil {
			st = &stat{}
		}
		wr := 0.5
		if st.n > 0 {
			wr = float64(st.wins) / float64(st.n)
		}
		score := math.Exp(3 * (st.excEMA*10 + (wr - 0.5)))
		if score < 0.15 {
			score = 0.15
		}
		raw[i] = score
		sum += score
	}
	if sum == 0 {
		sum = 1
	}
	var weights []*commonv1.StrategyWeight
	for i, id := range ids {
		st := s.stats[id]
		wr := 0.5
		regime := "mixed"
		if st != nil && st.n > 0 {
			wr = float64(st.wins) / float64(st.n)
		}
		if st != nil && st.excEMA != 0 {
			regime = "backtest-fit"
		}
		expect := 0.0
		if st != nil {
			expect = st.excEMA
		}
		weights = append(weights, &commonv1.StrategyWeight{
			StrategyId: id, Weight: raw[i] / sum, Expectancy: expect, WinRate: wr, Regime: regime,
		})
	}
	sort.Slice(weights, func(i, j int) bool { return weights[i].Weight > weights[j].Weight })
	return &learningv1.GetWeightsResponse{Weights: weights}, nil
}

func (s *Service) RunBacktest(_ context.Context, req *learningv1.RunBacktestRequest) (*learningv1.BacktestReport, error) {
	years := int(req.Years)
	if years <= 0 {
		years = 5
	}
	s.mu.Lock()
	s.report = &learningv1.BacktestReport{Years: int32(years), Status: "running"}
	s.mu.Unlock()
	rep := backtest.Run(years, time.Now())
	out := toProto(rep)
	if rep.Snapshot.Date != "" && len(rep.Snapshot.Roster) > 0 {
		if path, err := backtest.SaveRoster(rosterDir(), rep.Snapshot); err != nil {
			s.log.Error("roster snapshot", "err", err)
		} else {
			s.log.Info("roster snapshot written", "file", path, "names", len(rep.Snapshot.Roster), "new", len(rep.Snapshot.Added))
		}
	}
	s.mu.Lock()
	s.applyBacktest(rep)
	s.report = out
	s.mu.Unlock()
	return out, nil
}

func (s *Service) GetBacktest(_ context.Context, _ *learningv1.GetBacktestRequest) (*learningv1.BacktestReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.report == nil {
		return &learningv1.BacktestReport{
			Status: "idle",
			Note:   "Run a 5-year backtest to promote new methods and timeframes.",
		}, nil
	}
	return s.report, nil
}

func (s *Service) applyBacktest(rep backtest.Report) {
	var roster []string
	if len(rep.Snapshot.Roster) > 0 {
		roster = append(roster, rep.Snapshot.Roster...)
	} else {
		for _, v := range rep.Variants {
			if v.Promoted {
				roster = append(roster, v.Spec.ID)
			}
		}
	}
	var kept []string
	for _, id := range roster {
		if strategies.IDHoldsUnderSession(id) && !config.Bool("SCALP_MODE") {
			continue
		}
		kept = append(kept, id)
	}
	for _, v := range rep.Variants {
		if !v.Promoted {
			continue
		}
		s.stats[v.Spec.ID] = &stat{
			n: v.Trades, wins: v.Wins, pnlEMA: v.ReturnPct, excEMA: v.ExcessPct / 100,
		}
	}
	if len(kept) > 0 {
		s.roster = kept
	}
}

func toProto(rep backtest.Report) *learningv1.BacktestReport {
	var shown []backtest.Variant
	for _, v := range rep.Variants {
		if v.Promoted {
			shown = append(shown, v)
		}
	}
	for _, v := range rep.Variants {
		if len(shown) >= 18 {
			break
		}
		if !v.Promoted {
			shown = append(shown, v)
		}
	}
	var vs []*learningv1.BacktestVariant
	for _, v := range shown {
		vs = append(vs, &learningv1.BacktestVariant{
			StrategyId: v.Spec.ID, Method: v.Spec.Method, Timeframe: v.Spec.Timeframe,
			Params:    fmt.Sprintf("fast=%d slow=%d lookback=%d", v.Spec.Fast, v.Spec.Slow, v.Spec.Lookback),
			ReturnPct: v.ReturnPct, ExcessPct: v.ExcessPct, WinRate: v.WinRate,
			Trades: int32(v.Trades), Promoted: v.Promoted, Lesson: v.Lesson,
		})
	}
	return &learningv1.BacktestReport{
		Years: int32(rep.Years), VariantsTested: int32(rep.VariantsTested),
		VariantsPromoted: int32(rep.VariantsPromoted), Status: rep.Status,
		RanAtUnixMs: rep.RanAt.UnixMilli(), NiftyReturnPct: rep.NiftyReturnPct,
		Variants: vs, Note: rep.Note,
	}
}
