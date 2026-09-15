package sentiment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/config"
	"aperture/pkg/research"
	"aperture/pkg/strategies"
)

func investigate(ctx context.Context, llm Completer, ns []article, mode, id string, useLLM bool) *commonv1.InvestigationReport {
	txt := ""
	var sources []*commonv1.NewsItem
	symset := map[string]struct{}{}
	providers := map[string]struct{}{}
	for _, n := range ns {
		txt += " " + n.Title + " " + n.Summary
		sources = append(sources, toNewsItem(n))
		providers[n.Provider] = struct{}{}
		for _, s := range n.Symbols {
			symset[s] = struct{}{}
		}
	}
	low := strings.ToLower(txt)
	event := classifyEvent(low)
	stance, score := classifyStance(low)
	var symbols []string
	for s := range symset {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)
	age := time.Since(ns[0].Published)
	conf := 0.45 + 0.12*float64(len(providers))
	if age > 24*time.Hour {
		conf -= 0.15
		score *= 0.7
	}
	if event == "rumor" {
		conf -= 0.2
	}
	if conf > 0.9 {
		conf = 0.9
	}
	if conf < 0.25 {
		conf = 0.25
	}
	horizon := "swing"
	if strings.Contains(low, "open") || event == "flow" {
		horizon = "intraday"
	}
	if event == "guidance" || event == "regulation" {
		horizon = "position"
	}
	standAside := event == "rumor" && math.Abs(score) < 0.35
	thesis := fmt.Sprintf("%s tape on %s (%s). Stance %s with corroboration %d.",
		event, strings.Join(symbols, ", "), horizon, stance, len(providers))
	risks := "Headline models misread sarcasm and rumours; do not treat as guaranteed alpha vs Nifty."

	sym := ""
	if len(symbols) > 0 {
		sym = symbols[0]
	}
	in := research.Input{
		Symbol: sym, Headline: ns[0].Title, News: txt, Score: score,
	}
	note := research.Heuristic(in)
	if useLLM && llm != nil && llm.Enabled() && config.BoolDefault("INVESTIGATION_LLM", true) {
		note = research.Conclude(ctx, llm, in)
	}
	if note.Conclusion != "" {
		thesis = note.Conclusion
		if note.Technical != "" || note.Fundamental != "" {
			thesis = note.Conclusion + "\nTechnical: " + note.Technical + "\nFundamental: " + note.Fundamental
		}
	}
	if note.Risks != "" {
		risks = note.Risks
	}
	if note.Stance != "" {
		stance = note.Stance
	}
	if note.Horizon != "" {
		horizon = note.Horizon
	}
	if note.StandAside {
		standAside = true
	}
	tilts := mapTilts(event, score, standAside)
	if id == "" {
		id = fmt.Sprintf("inv-%d", time.Now().UnixNano())
	}
	slog.Info("news analysis",
		"id", id,
		"mode", mode,
		"event", event,
		"stance", stance,
		"score", score,
		"confidence", conf,
		"horizon", horizon,
		"symbols", symbols,
		"sources", len(providers),
		"headline", ns[0].Title,
		"research", note.Mode,
		"thesis", clipLog(thesis, 180),
	)
	return &commonv1.InvestigationReport{
		Id: id, Headline: ns[0].Title, Symbols: symbols,
		EventType: event, Stance: stance, Score: score, Confidence: conf,
		Corroboration: int32(len(providers)), Horizon: horizon, StrategyImplications: tilts,
		StandAside: standAside, Sources: sources, AnalyzedAtUnixMs: time.Now().UnixMilli(),
		Mode: mode, Thesis: thesis, Risks: risks, Status: "completed",
	}
}

func classifyEvent(low string) string {
	switch {
	case strings.Contains(low, "rumor") || strings.Contains(low, "unconfirmed"):
		return "rumor"
	case strings.Contains(low, "sebi") || strings.Contains(low, "rbi"):
		return "regulation"
	case strings.Contains(low, "earnings") || strings.Contains(low, "npa") || strings.Contains(low, "nim"):
		return "earnings"
	case strings.Contains(low, "guidance") || strings.Contains(low, "outlook"):
		return "guidance"
	case strings.Contains(low, "block deal") || strings.Contains(low, "flow"):
		return "flow"
	case strings.Contains(low, "order") || strings.Contains(low, "mandate") || strings.Contains(low, "deal"):
		return "product"
	case strings.Contains(low, "legal") || strings.Contains(low, "court"):
		return "legal"
	case strings.Contains(low, "rate") || strings.Contains(low, "inflation"):
		return "macro"
	default:
		return "macro"
	}
}

func classifyStance(low string) (string, float64) {
	bull := []string{"wins", "beats", "lifts", "growth", "raised", "improving", "ahead", "inflow", "climb"}
	bear := []string{"cools", "weaker", "cautious", "uneven", "rumor", "pledge", "spike", "digest"}
	s := 0.0
	for _, w := range bull {
		if strings.Contains(low, w) {
			s += 0.18
		}
	}
	for _, w := range bear {
		if strings.Contains(low, w) {
			s -= 0.18
		}
	}
	if s > 1 {
		s = 1
	}
	if s < -1 {
		s = -1
	}
	label := "mixed"
	if s > 0.15 {
		label = "bullish"
	} else if s < -0.15 {
		label = "bearish"
	} else if s == 0 {
		label = "unclear"
	}
	return label, s
}

func mapTilts(event string, score float64, standAside bool) []*commonv1.StrategyTilt {
	if standAside {
		return []*commonv1.StrategyTilt{{StrategyId: strategies.MeanReversion15, Tilt: 0, Reason: "stand aside on unconfirmed tape"}}
	}
	var out []*commonv1.StrategyTilt
	if score > 0.1 && (event == "earnings" || event == "product" || event == "guidance") {
		out = append(out,
			&commonv1.StrategyTilt{StrategyId: strategies.Momentum5m, Tilt: 0.35, Reason: "confirmed constructive tape"},
			&commonv1.StrategyTilt{StrategyId: strategies.Breakout1h, Tilt: 0.25, Reason: "news can fuel range break"},
			&commonv1.StrategyTilt{StrategyId: strategies.SwingDaily, Tilt: 0.2, Reason: "swing continuation"},
		)
	}
	if event == "rumor" || score < -0.1 {
		out = append(out, &commonv1.StrategyTilt{StrategyId: strategies.MeanReversion15, Tilt: 0.3, Reason: "fade noisy extension"})
	}
	if event == "flow" {
		out = append(out, &commonv1.StrategyTilt{StrategyId: strategies.OpeningRange, Tilt: 0.4, Reason: "open flow shock"})
	}
	out = append(out, &commonv1.StrategyTilt{StrategyId: strategies.SentimentTilt, Tilt: score, Reason: "investigation overlay"})
	return out
}

func clipLog(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
