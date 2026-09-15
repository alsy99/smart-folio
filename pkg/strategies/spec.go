package strategies

import (
	"fmt"
	"strconv"
	"strings"
)

type Spec struct {
	ID        string
	Method    string
	Timeframe string
	Fast      int
	Slow      int
	Lookback  int
}

func DefaultSpecs() []Spec {
	return []Spec{
		{ID: SMACross1d, Method: "sma_cross", Timeframe: "1d", Fast: 10, Slow: 30, Lookback: 30},
		{ID: Momentum1d, Method: "momentum", Timeframe: "1d", Fast: 0, Slow: 0, Lookback: 10},
		{ID: MeanReversion1d, Method: "mean_reversion", Timeframe: "1d", Fast: 0, Slow: 20, Lookback: 20},
		{ID: Breakout1d, Method: "breakout", Timeframe: "1d", Fast: 0, Slow: 0, Lookback: 20},
		{ID: SwingDaily, Method: "swing", Timeframe: "1d", Fast: 10, Slow: 30, Lookback: 30},
		{ID: SentimentTilt, Method: "sentiment", Timeframe: "1d", Fast: 0, Slow: 0, Lookback: 1},
	}
}

func Encode(method, tf string, fast, slow, lookback int) string {
	return fmt.Sprintf("%s_%s_%d_%d_%d", method, tf, fast, slow, lookback)
}

func Parse(id string) Spec {
	for _, d := range DefaultSpecs() {
		if d.ID == id {
			return d
		}
	}
	parts := strings.Split(id, "_")
	s := Spec{ID: id, Method: "momentum", Timeframe: "1d", Lookback: 5}
	if len(parts) >= 5 {
		s.Lookback, _ = strconv.Atoi(parts[len(parts)-1])
		s.Slow, _ = strconv.Atoi(parts[len(parts)-2])
		s.Fast, _ = strconv.Atoi(parts[len(parts)-3])
		s.Timeframe = parts[len(parts)-4]
		s.Method = strings.Join(parts[:len(parts)-4], "_")
	}
	if s.Lookback <= 0 {
		s.Lookback = 10
	}
	return s
}

// HoldsUnderSession is true for tape frames shorter than one NSE cash session.
func HoldsUnderSession(tf string) bool {
	switch strings.ToLower(strings.TrimSpace(tf)) {
	case "1m", "5m", "15m", "30m", "1h", "session":
		return true
	default:
		return false
	}
}

// IDHoldsUnderSession covers both DefaultSpecs and encoded ids like momentum_5m.
func IDHoldsUnderSession(id string) bool {
	spec := Parse(id)
	if HoldsUnderSession(spec.Timeframe) {
		return true
	}
	parts := strings.Split(strings.ToLower(id), "_")
	if len(parts) == 0 {
		return false
	}
	return HoldsUnderSession(parts[len(parts)-1])
}

func SearchGrid() []Spec {
	methods := []string{"sma_cross", "momentum", "mean_reversion", "breakout", "swing"}
	frames := []string{"1d", "1w"}
	pairs := [][3]int{{3, 10, 10}, {5, 20, 20}, {8, 34, 34}}
	out := DefaultSpecs()
	seen := map[string]struct{}{}
	for _, d := range out {
		seen[d.ID] = struct{}{}
	}
	for _, m := range methods {
		for _, tf := range frames {
			for _, p := range pairs {
				id := Encode(m, tf, p[0], p[1], p[2])
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				out = append(out, Spec{ID: id, Method: m, Timeframe: tf, Fast: p[0], Slow: p[1], Lookback: p[2]})
			}
		}
	}
	return out
}
