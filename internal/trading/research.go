package trading

import (
	"context"
	"log/slog"
	"os"
	"sort"

	"aperture/pkg/llm"
	"aperture/pkg/research"
	"aperture/pkg/strategies"
)

const picksFile = "data/picks.json"

func (s *Service) analyzePicks(picked map[string]strategies.Signal, news map[string]string) {
	if os.Getenv("CAMPAIGN_REPLAY") == "1" {
		return
	}
	s.mu.Lock()
	if s.analyzing {
		s.mu.Unlock()
		return
	}
	s.analyzing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.analyzing = false
		s.mu.Unlock()
	}()
	type row struct {
		sym string
		sig strategies.Signal
	}
	var rows []row
	for sym, sig := range picked {
		if sig.Direction == 0 || sig.Score < 0.25 {
			continue
		}
		rows = append(rows, row{sym, sig})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].sig.Score > rows[j].sig.Score })
	desk := s.inv
	if desk == nil {
		desk = research.NewDesk("")
	}
	day := llm.ISTDay(s.now())
	var notes []research.Note
	for _, r := range rows {
		in := research.Input{
			Symbol:     r.sym,
			Headline:   r.sig.Reason,
			News:       news[r.sym],
			StrategyID: r.sig.StrategyID,
			Score:      r.sig.Score,
			Direction:  r.sig.Direction,
			Now:        s.now(),
		}
		note := research.Heuristic(in)
		key := research.CacheKey(r.sym, day, firstHeadline(news[r.sym], r.sig.Reason))
		if rec, ok := desk.Lookup(key); ok && rec.Thesis != "" {
			note.Conclusion = rec.Thesis
			note.Mode = rec.Mode
		}
		s.log.Info("pick research",
			"symbol", note.Symbol,
			"mode", note.Mode,
			"stance", note.Stance,
			"strategy", note.StrategyID,
			"conclusion", note.Conclusion,
		)
		notes = append(notes, note)
	}
	if err := research.Save(picksFile, notes); err != nil {
		slog.Warn("pick research save", "err", err)
	}
}

func firstHeadline(news, fallback string) string {
	if news != "" {
		return news
	}
	return fallback
}

func (s *Service) invDir() string {
	if s.inv != nil && s.inv.Dir != "" {
		return s.inv.Dir
	}
	return llm.Dir()
}

func (s *Service) mintInvestigation(sym string, sig strategies.Signal) string {
	desk := s.inv
	if desk == nil {
		desk = research.NewDesk(s.invDir())
		s.inv = desk
	}
	in := research.Input{
		Symbol: sym, Headline: sig.Reason, StrategyID: sig.StrategyID,
		Score: sig.Score, Direction: sig.Direction, Now: s.now(),
	}
	_, rec := desk.Run(context.Background(), nil, in, false)
	return rec.ID
}
