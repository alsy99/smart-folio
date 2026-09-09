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
		{ID: SMACross15m, Method: "sma_cross", Timeframe: "15m", Fast: 5, Slow: 20, Lookback: 20},
		{ID: Momentum5m, Method: "momentum", Timeframe: "5m", Fast: 0, Slow: 0, Lookback: 3},
		{ID: MeanReversion15, Method: "mean_reversion", Timeframe: "15m", Fast: 0, Slow: 20, Lookback: 20},
		{ID: Breakout1h, Method: "breakout", Timeframe: "1h", Fast: 0, Slow: 0, Lookback: 20},
		{ID: SwingDaily, Method: "swing", Timeframe: "1d", Fast: 10, Slow: 30, Lookback: 30},
		{ID: SentimentTilt, Method: "sentiment", Timeframe: "session", Fast: 0, Slow: 0, Lookback: 1},
		{ID: OpeningRange, Method: "opening_range", Timeframe: "session", Fast: 0, Slow: 0, Lookback: 6},
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

func SearchGrid() []Spec {
	methods := []string{"sma_cross", "momentum", "mean_reversion", "breakout", "swing"}
	frames := []string{"5m", "15m", "1h", "1d", "1w"}
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

type Window struct {
	Close, High, Low []float64
}

func Evaluate(spec Spec, symbol string, w Window, sentiment float64) Signal {
	c, h, l := w.Close, w.High, w.Low
	switch spec.Method {
	case "sma_cross":
		sig := smaCrossParams(symbol, c, spec.Fast, spec.Slow)
		sig.StrategyID = spec.ID
		sig.Reason = spec.Timeframe + " " + sig.Reason
		return sig
	case "momentum":
		lb := spec.Lookback
		if lb < 2 {
			lb = 3
		}
		sig := momentumLookback(symbol, c, lb)
		sig.StrategyID = spec.ID
		return sig
	case "mean_reversion":
		n := spec.Slow
		if n <= 0 {
			n = 20
		}
		sig := meanRevertN(symbol, c, n)
		sig.StrategyID = spec.ID
		return sig
	case "breakout":
		sig := Breakout(symbol, h, c)
		sig.StrategyID = spec.ID
		return sig
	case "swing":
		fast, slow := spec.Fast, spec.Slow
		if fast <= 0 {
			fast = 10
		}
		if slow <= 0 {
			slow = 30
		}
		sig := swingParams(symbol, c, fast, slow)
		sig.StrategyID = spec.ID
		return sig
	case "opening_range":
		sig := OpeningRangeBreak(symbol, h, l, c)
		sig.StrategyID = spec.ID
		return sig
	case "sentiment":
		return Sentiment(symbol, sentiment)
	default:
		return Signal{StrategyID: spec.ID, Symbol: symbol}
	}
}

func smaCrossParams(symbol string, closes []float64, fast, slow int) Signal {
	if fast <= 0 {
		fast = 5
	}
	if slow <= 0 {
		slow = 20
	}
	s := Signal{StrategyID: SMACross15m, Symbol: symbol}
	f, sl := sma(closes, fast), sma(closes, slow)
	if f == 0 || sl == 0 {
		return s
	}
	diff := (f - sl) / sl
	s.Score = min1(abs(diff) * 40)
	if diff > 0.001 {
		s.Direction = 1
		s.Reason = "fast SMA above slow"
	} else if diff < -0.001 {
		s.Direction = -1
		s.Reason = "fast SMA below slow"
	}
	return s
}

func momentumLookback(symbol string, closes []float64, lb int) Signal {
	s := Signal{StrategyID: Momentum5m, Symbol: symbol}
	if len(closes) <= lb {
		return s
	}
	ret := closes[len(closes)-1]/closes[len(closes)-1-lb] - 1
	s.Score = min1(abs(ret) * 25)
	if ret > 0.002 {
		s.Direction = 1
		s.Reason = "momentum thrust higher"
	} else if ret < -0.002 {
		s.Direction = -1
		s.Reason = "momentum thrust lower"
	}
	return s
}

func meanRevertN(symbol string, closes []float64, n int) Signal {
	s := Signal{StrategyID: MeanReversion15, Symbol: symbol}
	m := sma(closes, n)
	if m == 0 || len(closes) == 0 {
		return s
	}
	dev := (closes[len(closes)-1] - m) / m
	s.Score = min1(abs(dev) * 20)
	if dev > 0.008 {
		s.Direction = -1
		s.Reason = "extended above mean"
	} else if dev < -0.008 {
		s.Direction = 1
		s.Reason = "extended below mean"
	}
	return s
}

func swingParams(symbol string, closes []float64, fast, slow int) Signal {
	s := Signal{StrategyID: SwingDaily, Symbol: symbol}
	f, sl := sma(closes, fast), sma(closes, slow)
	if sl == 0 {
		return s
	}
	diff := (f - sl) / sl
	s.Score = min1(abs(diff) * 15)
	if diff > 0 {
		s.Direction = 1
		s.Reason = "swing trend up"
	} else {
		s.Direction = -1
		s.Reason = "swing trend down"
	}
	return s
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func min1(v float64) float64 {
	if v > 1 {
		return 1
	}
	return v
}
