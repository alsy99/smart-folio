package learning

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	commonv1 "aperture/gen/common/v1"
	learningv1 "aperture/gen/learning/v1"
	"aperture/pkg/backtest"
	"aperture/pkg/config"
	"aperture/pkg/learn"
	"aperture/pkg/strategies"
	"aperture/pkg/tape"
)

type Config struct {
	Bind string
	Dir  string
}

func LoadConfig() Config {
	return Config{
		Bind: config.String("LEARNING_BIND", ":9083"),
		Dir:  config.String("LEARNING_DIR", "data/learning"),
	}
}

func rosterDir() string {
	return config.String("ROSTER_DIR", "data/roster")
}

type Service struct {
	learningv1.UnimplementedLearningServiceServer
	log     *slog.Logger
	dir     string
	now     func() time.Time
	mu      sync.Mutex
	journal []*commonv1.JournalEntry
	roster  []string
	// failing are shipped defaults that missed the gate on a real tape.
	// They are published with weight 0 so the trading book skips them.
	failing []string
	// gated is true once a real-tape snapshot has judged the book. Only an
	// ungated desk (no file) falls back to the shipped daily specs.
	gated bool
	// rosterSrc is where the roster came from: a dated snapshot or RosterDefault.
	rosterSrc string
	snap      learn.Snapshot
	report    *learningv1.BacktestReport
}

func barsDir() string {
	return config.String("BARS_DIR", tape.DefaultDir)
}

func New(log *slog.Logger) *Service {
	return NewDir(log, LoadConfig().Dir)
}

func NewDir(log *slog.Logger, dir string) *Service {
	if log == nil {
		log = slog.Default()
	}
	if dir == "" {
		dir = "data/learning"
	}
	s := &Service{log: log, dir: dir, now: time.Now, roster: strategies.IDs()}
	if snap, path, err := backtest.LoadLatestRoster(rosterDir()); err == nil {
		s.adoptLocked(snap, path)
		log.Info("roster from snapshot", "file", path, "date", snap.Date, "tape", snap.Tape, "roster", len(snap.Roster), "added", len(snap.Added), "failing", len(snap.Failing))
		if len(snap.Roster) == 0 {
			log.Warn("roster is empty — every shipped default failed the gate; the paper book holds cash", "failing", snap.Failing)
		}
	} else {
		s.rosterSrc = RosterDefault
		log.Warn("roster: no snapshot file — default daily specs", "dir", rosterDir(), "names", len(s.roster), "err", err)
	}
	s.bootWeights()
	s.bootJournal()
	return s
}

// RosterDefault is the hero line when data/roster/ has no snapshot: the book
// runs the shipped daily specs and says so, rather than implying a lab vetted it.
const RosterDefault = "default daily specs (no data/roster snapshot)"

// RegimeFailing tags a weight row for a default that missed the gate.
const RegimeFailing = "failing-gate"

// rosterSource is the one-line provenance shown on the hero.
func rosterSource(snap backtest.RosterSnapshot, path string) string {
	tape := snap.Tape
	if tape == "" {
		tape = "unknown tape"
	}
	return fmt.Sprintf("%s · %s · %d closes · %d trading · %d admitted · %d failing (weight 0)",
		filepath.ToSlash(path), tape, snap.Days, len(snap.Roster), len(snap.Added), len(snap.Failing))
}

// adoptLocked installs a real-tape snapshot: roster trades, failing sit at
// weight 0, and the desk stops falling back to the shipped defaults.
func (s *Service) adoptLocked(snap backtest.RosterSnapshot, path string) {
	var kept []string
	for _, id := range snap.Roster {
		if strategies.IDHoldsUnderSession(id) && !config.Bool("SCALP_MODE") {
			continue
		}
		kept = append(kept, id)
	}
	s.roster = kept
	s.failing = append([]string(nil), snap.Failing...)
	s.gated = true
	s.rosterSrc = rosterSource(snap, path)
}

// RosterNote prefixes a report note with the roster provenance line the
// desk shows on the hero. Line 1 is always "Roster: …".
func RosterNote(src, rest string) string {
	if src == "" {
		src = RosterDefault
	}
	if rest == "" {
		return "Roster: " + src
	}
	return "Roster: " + src + "\n" + rest
}

func (s *Service) bootWeights() {
	if loaded, err := learn.LoadSnapshot(s.dir); err == nil && len(loaded.Weights) > 0 {
		s.snap = loaded
		return
	}
	s.snap = learn.EqualSnapshot(s.ids(), s.now().UTC())
	if err := learn.SaveSnapshot(s.dir, s.snap); err != nil {
		s.log.Warn("equal weight snapshot", "err", err)
	}
}

func (s *Service) bootJournal() {
	closes, err := learn.LoadCloses(s.dir)
	if err != nil {
		s.log.Warn("closes", "err", err)
		return
	}
	for i := len(closes) - 1; i >= 0 && len(s.journal) < 200; i-- {
		s.journal = append(s.journal, journalFromClose(closes[i]))
	}
	if err := learn.BackupCloses(s.dir, s.now().UTC()); err != nil {
		s.log.Warn("journal backup", "err", err)
	}
}

// ids is the trading set. Once a real tape has judged the book there is no
// fallback: an empty roster means the book holds cash.
func (s *Service) ids() []string {
	if s.gated || len(s.roster) > 0 {
		return s.roster
	}
	return strategies.IDs()
}

func (s *Service) Seed(ctx context.Context) {
	if snap, path, err := backtest.LoadLatestRoster(rosterDir()); err == nil && !backtest.RosterStale(snap, time.Now(), 7) {
		s.mu.Lock()
		s.adoptLocked(snap, path)
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

func (s *Service) Weekly(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.maybeWeekly()
		}
	}
}

func (s *Service) maybeWeekly() {
	s.mu.Lock()
	due := learn.Due(s.snap, s.now().UTC())
	s.mu.Unlock()
	if !due {
		return
	}
	if _, err := s.RunWeekly(false); err != nil {
		s.log.Error("weekly review", "err", err)
	}
}

// RunWeekly is the only path that may rewrite weights.json.
func (s *Service) RunWeekly(force bool) (learn.Snapshot, error) {
	closes, err := learn.LoadCloses(s.dir)
	if err != nil {
		return learn.Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	next := learn.ApplyWeekly(s.ids(), s.snap, closes, now, force)
	if err := learn.SaveSnapshot(s.dir, next); err != nil {
		return learn.Snapshot{}, err
	}
	s.snap = next
	s.log.Info("weekly review", "moved", next.Moved, "note", next.Note, "closes", len(closes))
	return next, nil
}

func (s *Service) RecordTrade(_ context.Context, req *learningv1.RecordTradeRequest) (*learningv1.RecordTradeResponse, error) {
	t := req.GetTrade()
	if t == nil {
		return &learningv1.RecordTradeResponse{}, nil
	}
	lesson, tags := Attribute(t)
	if t.Lesson != "" {
		lesson = t.Lesson + "; " + lesson
	}
	t.AttributionTags = tags
	t.Lesson = lesson
	c := closeFromPaper(t, lesson)
	entry := &commonv1.JournalEntry{
		Id: "j-" + t.Id, Trade: t, Lesson: lesson, Tags: tags, TsUnixMs: s.now().UnixMilli(),
	}
	s.mu.Lock()
	if err := learn.AppendClose(s.dir, c); err != nil {
		s.log.Error("append close", "err", err)
	}
	s.journal = append([]*commonv1.JournalEntry{entry}, s.journal...)
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
	ids := s.ids()
	byID := map[string]learn.Weight{}
	for _, w := range s.snap.Weights {
		byID[w.StrategyID] = w
	}
	eq := learn.EqualWeights(ids)
	var weights []*commonv1.StrategyWeight
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			row = learn.Weight{StrategyID: id, Weight: eq[id], Regime: "pending"}
		}
		weights = append(weights, &commonv1.StrategyWeight{
			StrategyId: id, Weight: row.Weight, Expectancy: row.Expectancy,
			WinRate: row.WinRate, Regime: row.Regime,
		})
	}
	// Failing defaults are published at weight 0 so the book — and the
	// dashboard — see them for what they are, not silently dropped.
	for _, id := range s.failing {
		weights = append(weights, &commonv1.StrategyWeight{StrategyId: id, Weight: 0, Regime: RegimeFailing})
	}
	sort.Slice(weights, func(i, j int) bool {
		if weights[i].Weight == weights[j].Weight {
			return weights[i].StrategyId < weights[j].StrategyId
		}
		return weights[i].Weight > weights[j].Weight
	})
	return &learningv1.GetWeightsResponse{Weights: weights}, nil
}

func (s *Service) RunBacktest(ctx context.Context, req *learningv1.RunBacktestRequest) (*learningv1.BacktestReport, error) {
	years := int(req.Years)
	if years <= 0 {
		years = 5
	}
	s.mu.Lock()
	s.report = &learningv1.BacktestReport{Years: int32(years), Status: "running", Note: RosterNote(s.rosterSrc, "")}
	s.mu.Unlock()
	tp := tape.Pick(ctx, barsDir())
	rep := backtest.RunOn(tp, years, time.Now())
	s.log.Info("walk-forward", "tape", rep.Tape, "days", rep.TapeDays, "status", rep.Status, "promotable", rep.Promotable())
	// Only a real tape may touch data/roster/. A mock run reports, and stops.
	path := ""
	if rep.Promotable() && rep.Snapshot.Date != "" {
		p, err := backtest.SaveRoster(rosterDir(), rep.Snapshot)
		if err != nil {
			s.log.Error("roster snapshot", "err", err)
		} else {
			path = p
			s.log.Info("roster snapshot written", "file", path, "tape", rep.Tape, "roster", len(rep.Snapshot.Roster), "new", len(rep.Snapshot.Added), "failing", len(rep.Snapshot.Failing))
		}
	}
	s.mu.Lock()
	if path != "" {
		s.applyBacktest(rep, path)
	}
	out := toProto(rep)
	out.Note = RosterNote(s.rosterSrc, rep.Note)
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
			Note:   RosterNote(s.rosterSrc, "Run a 5-year backtest on real daily bars to promote new methods and timeframes."),
		}, nil
	}
	return s.report, nil
}

// applyBacktest moves the in-memory roster only on real-tape evidence; a
// mock run leaves whatever the desk booted with. An empty roster is a
// result, not an error: the book holds cash.
func (s *Service) applyBacktest(rep backtest.Report, path string) {
	if !rep.Promotable() {
		return
	}
	s.adoptLocked(rep.Snapshot, path)
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

func closeFromPaper(t *commonv1.PaperTrade, lesson string) learn.Close {
	mae, mfe, regime, hold := learn.ParseFacts(t.AttributionTags)
	if hold == 0 && t.ClosedAtUnixMs > t.OpenedAtUnixMs {
		hold = t.ClosedAtUnixMs - t.OpenedAtUnixMs
	}
	if regime == "" || regime == "chop" {
		if r := learn.Regime(t.NiftyReturn); r != "chop" {
			regime = r
		}
	}
	return learn.NewClose(t.Id, t.StrategyId, t.Symbol, t.Pnl, t.ExcessReturn, hold, mae, mfe, regime, t.ClosedAtUnixMs, lesson)
}

func journalFromClose(c learn.Close) *commonv1.JournalEntry {
	tags := append(learn.FactTags(c.MAE, c.MFE, c.Regime, c.HoldMs), c.Method)
	return &commonv1.JournalEntry{
		Id: "j-" + c.ID,
		Trade: &commonv1.PaperTrade{
			Id: c.ID, Symbol: c.Symbol, StrategyId: c.StrategyID,
			Pnl: c.PnL, ExcessReturn: c.Excess, ClosedAtUnixMs: c.ClosedAtMs,
			Lesson: c.Lesson, AttributionTags: tags,
		},
		Lesson: c.Lesson, Tags: tags, TsUnixMs: c.ClosedAtMs,
	}
}
