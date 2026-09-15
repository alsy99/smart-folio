package trading

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/research"
	"aperture/pkg/strategies"
)

const picksFile = "data/picks.json"

func (s *Service) analyzePicks(picked map[string]strategies.Signal, news map[string]string) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
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
	llmLeft := config.Int("INVESTIGATION_LLM_MAX", 5)
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
		if llmLeft > 0 && s.llm != nil && s.llm.Enabled() {
			note = research.Conclude(ctx, s.llm, in)
			llmLeft--
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
