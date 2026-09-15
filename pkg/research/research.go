package research

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aperture/pkg/fundamentals"
	"aperture/pkg/indstocks"
	"aperture/pkg/ta"
)

type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, system, user string) (string, error)
}

type Note struct {
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	StrategyID  string  `json:"strategyId,omitempty"`
	Score       float64 `json:"score"`
	Direction   int     `json:"direction"`
	Headline    string  `json:"headline,omitempty"`
	Fundamental string  `json:"fundamental"`
	Technical   string  `json:"technical"`
	Conclusion  string  `json:"conclusion"`
	Risks       string  `json:"risks"`
	Stance      string  `json:"stance"`
	Horizon     string  `json:"horizon"`
	StandAside  bool    `json:"standAside"`
	Mode        string  `json:"mode"`
	AtUnixMs    int64   `json:"atUnixMs"`
}

type Input struct {
	Symbol     string
	Headline   string
	News       string
	StrategyID string
	Score      float64
	Direction  int
	Now        time.Time
}

const systemPrompt = `You are the research desk for Aperture, an India NSE paper-trading lab. Not financial advice, not a live broker.
You receive (1) a qualitative fundamental card that is a reference briefing, NOT live filings, (2) computed technicals on the desk tape (INDstocks when a token is set, otherwise synthetic), (3) optional news.
Reason step by step: what the business is, what the tape is doing, whether news is material or mis-tagged, then a conclusion for a positional paper book vs Nifty (days to weeks, not a scalp).
Reply with JSON only:
{"stance":"bullish|bearish|mixed","horizon":"swing|position","stand_aside":false,"fundamental":"2-3 sentences","technical":"2-3 sentences","conclusion":"2-3 sentences on what the paper book should do and why","risks":"main ways this call is wrong"}`

var (
	gate     = make(chan struct{}, 1)
	cacheMu  sync.Mutex
	cache    = map[string]Note{}
	cacheTTL = 15 * time.Minute
)

func Snapshot(in Input) (ta.Snapshot, fundamentals.Card) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	bars, _ := indstocks.BarsOrMock(ctx, in.Symbol, "1d", 40, now)
	tech := ta.FromBars(in.Symbol, bars)
	return tech, fundamentals.Lookup(in.Symbol)
}

func Heuristic(in Input) Note {
	tech, card := Snapshot(in)
	stance := "mixed"
	switch {
	case in.Direction > 0 && tech.Trend == "uptrend":
		stance = "bullish"
	case in.Direction < 0 || tech.Trend == "downtrend":
		stance = "bearish"
	}
	horizon := "position"
	if tech.Setup == "breakout / range high" {
		horizon = "swing"
	}
	fund := card.Summary()
	technical := tech.Summary()
	if in.Symbol == "" {
		fund = "No mapped NSE name — keyword tagging missed this headline."
		technical = "Skip technicals until a universe symbol is attached."
	}
	n := Note{
		Symbol:      in.Symbol,
		Name:        card.Name,
		StrategyID:  in.StrategyID,
		Score:       in.Score,
		Direction:   in.Direction,
		Headline:    in.Headline,
		Fundamental: fund,
		Technical:   technical,
		Conclusion:  fmt.Sprintf("Heuristic: %s on %s (%s). %s.", stance, in.Symbol, tech.Trend, tech.Setup),
		Risks:       card.Risks + " Tape is INDstocks when a token is set, otherwise a desk generator; news keywords mis-tag often.",
		Stance:      stance,
		Horizon:     horizon,
		Mode:        "heuristic",
		AtUnixMs:    nowMs(in.Now),
	}
	return n
}

func Conclude(ctx context.Context, llm Completer, in Input) Note {
	base := Heuristic(in)
	if llm == nil || !llm.Enabled() {
		return base
	}
	cacheMu.Lock()
	if prev, ok := cache[in.Symbol]; ok && time.Since(time.UnixMilli(prev.AtUnixMs)) < cacheTTL && prev.Mode == "llm" {
		cacheMu.Unlock()
		prev.StrategyID = in.StrategyID
		prev.Score = in.Score
		prev.Direction = in.Direction
		return prev
	}
	cacheMu.Unlock()

	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return base
	}

	tech, card := Snapshot(in)
	user := fmt.Sprintf("SYMBOL %s (%s)\nFUNDAMENTAL CARD: %s\nTECHNICALS: %s\nSIGNAL strategy=%s score=%.2f direction=%d\nNEWS: %s\nHEADLINE: %s",
		in.Symbol, card.Sector, card.Summary(), tech.Summary(), in.StrategyID, in.Score, in.Direction, clip(in.News, 1200), in.Headline)
	raw, err := llm.Complete(ctx, systemPrompt, user)
	if err != nil || strings.TrimSpace(raw) == "" {
		base.Risks = base.Risks + " LLM: " + errString(err)
		return base
	}
	n := parse(raw, base)
	n.Mode = "llm"
	n.AtUnixMs = nowMs(time.Now())
	cacheMu.Lock()
	cache[in.Symbol] = n
	cacheMu.Unlock()
	return n
}

func parse(raw string, fallback Note) Note {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var p struct {
		Stance      string `json:"stance"`
		Horizon     string `json:"horizon"`
		StandAside  bool   `json:"stand_aside"`
		Fundamental string `json:"fundamental"`
		Technical   string `json:"technical"`
		Conclusion  string `json:"conclusion"`
		Risks       string `json:"risks"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		fallback.Conclusion = clip(raw, 800)
		fallback.Mode = "llm-text"
		return fallback
	}
	if p.Stance != "" {
		fallback.Stance = p.Stance
	}
	if p.Horizon != "" {
		fallback.Horizon = p.Horizon
	}
	fallback.StandAside = p.StandAside
	if p.Fundamental != "" {
		fallback.Fundamental = p.Fundamental
	}
	if p.Technical != "" {
		fallback.Technical = p.Technical
	}
	if p.Conclusion != "" {
		fallback.Conclusion = p.Conclusion
	}
	if p.Risks != "" {
		fallback.Risks = p.Risks
	}
	return fallback
}

func Save(path string, notes []Note) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func Load(path string) []Note {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var notes []Note
	if err := json.Unmarshal(b, &notes); err != nil {
		return nil
	}
	return notes
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func errString(err error) string {
	if err == nil {
		return "empty reply"
	}
	return err.Error()
}

func nowMs(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UnixMilli()
}
