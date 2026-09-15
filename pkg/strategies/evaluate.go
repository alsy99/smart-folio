package strategies

import "math"

const (
	SMACross1d      = "sma_cross_1d"
	Momentum1d      = "momentum_1d"
	MeanReversion1d = "mean_reversion_1d"
	Breakout1d      = "breakout_1d"
	SwingDaily      = "swing_daily"
	SentimentTilt   = "sentiment_tilt"
)

func IDs() []string {
	return []string{
		SMACross1d, Momentum1d, MeanReversion1d, Breakout1d, SwingDaily, SentimentTilt,
	}
}

type Signal struct {
	StrategyID string
	Symbol     string
	Direction  int     // -1 short/sell, 0 flat, +1 buy
	Score      float64 // 0..1
	Reason     string
}

type Window struct {
	Close, High, Low []float64
}

// Evaluator is the Strategy pattern: each method is a swappable algorithm.
type Evaluator func(spec Spec, symbol string, w Window, sentiment float64) Signal

var registry = map[string]Evaluator{
	"sma_cross":      evalSMA,
	"momentum":       evalMomentum,
	"mean_reversion": evalMeanRevert,
	"breakout":       evalBreakout,
	"swing":          evalSwing,
	"opening_range":  evalOpeningRange,
	"sentiment":      evalSentiment,
}

func Evaluate(spec Spec, symbol string, w Window, sentiment float64) Signal {
	if ev, ok := registry[spec.Method]; ok {
		return ev(spec, symbol, w, sentiment)
	}
	return Signal{StrategyID: spec.ID, Symbol: symbol}
}

func evalSMA(spec Spec, symbol string, w Window, _ float64) Signal {
	fast, slow := spec.Fast, spec.Slow
	if fast <= 0 {
		fast = 5
	}
	if slow <= 0 {
		slow = 20
	}
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	f, sl := sma(w.Close, fast), sma(w.Close, slow)
	if f == 0 || sl == 0 {
		return s
	}
	diff := (f - sl) / sl
	s.Score = clamp01(math.Abs(diff) * 40)
	if diff > 0.001 {
		s.Direction = 1
		s.Reason = spec.Timeframe + " fast SMA above slow"
	} else if diff < -0.001 {
		s.Direction = -1
		s.Reason = spec.Timeframe + " fast SMA below slow"
	}
	return s
}

func evalMomentum(spec Spec, symbol string, w Window, _ float64) Signal {
	lb := spec.Lookback
	if lb < 2 {
		lb = 3
	}
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	if len(w.Close) <= lb {
		return s
	}
	ret := w.Close[len(w.Close)-1]/w.Close[len(w.Close)-1-lb] - 1
	s.Score = clamp01(math.Abs(ret) * 25)
	if ret > 0.002 {
		s.Direction = 1
		s.Reason = "momentum thrust higher"
	} else if ret < -0.002 {
		s.Direction = -1
		s.Reason = "momentum thrust lower"
	}
	return s
}

func evalMeanRevert(spec Spec, symbol string, w Window, _ float64) Signal {
	n := spec.Slow
	if n <= 0 {
		n = 20
	}
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	m := sma(w.Close, n)
	if m == 0 || len(w.Close) == 0 {
		return s
	}
	dev := (w.Close[len(w.Close)-1] - m) / m
	s.Score = clamp01(math.Abs(dev) * 20)
	if dev > 0.008 {
		s.Direction = -1
		s.Reason = "extended above mean"
	} else if dev < -0.008 {
		s.Direction = 1
		s.Reason = "extended below mean"
	}
	return s
}

func evalBreakout(spec Spec, symbol string, w Window, _ float64) Signal {
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	highs, closes := w.High, w.Close
	if len(highs) < 21 || len(closes) == 0 {
		return s
	}
	mx := highs[len(highs)-21]
	for i := len(highs) - 21; i < len(highs)-1; i++ {
		if highs[i] > mx {
			mx = highs[i]
		}
	}
	last := closes[len(closes)-1]
	if last > mx {
		s.Direction = 1
		s.Score = clamp01((last/mx - 1) * 40)
		s.Reason = "range high break"
	}
	return s
}

func evalSwing(spec Spec, symbol string, w Window, _ float64) Signal {
	fast, slow := spec.Fast, spec.Slow
	if fast <= 0 {
		fast = 10
	}
	if slow <= 0 {
		slow = 30
	}
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	f, sl := sma(w.Close, fast), sma(w.Close, slow)
	if sl == 0 {
		return s
	}
	diff := (f - sl) / sl
	s.Score = clamp01(math.Abs(diff) * 15)
	if diff > 0 {
		s.Direction = 1
		s.Reason = "swing trend up"
	} else {
		s.Direction = -1
		s.Reason = "swing trend down"
	}
	return s
}

func evalOpeningRange(spec Spec, symbol string, w Window, _ float64) Signal {
	s := Signal{StrategyID: spec.ID, Symbol: symbol}
	highs, lows, closes := w.High, w.Low, w.Close
	if len(closes) < 6 || len(highs) < 6 || len(lows) < 6 {
		return s
	}
	orH, orL := highs[len(highs)-6], lows[len(lows)-6]
	for i := len(highs) - 6; i < len(highs)-3; i++ {
		if highs[i] > orH {
			orH = highs[i]
		}
		if lows[i] < orL {
			orL = lows[i]
		}
	}
	last := closes[len(closes)-1]
	if last > orH {
		s.Direction = 1
		s.Score = 0.7
		s.Reason = "opening-range high break"
	} else if last < orL {
		s.Direction = -1
		s.Score = 0.7
		s.Reason = "opening-range low break"
	}
	return s
}

func evalSentiment(spec Spec, symbol string, _ Window, score float64) Signal {
	s := Signal{StrategyID: spec.ID, Symbol: symbol, Score: clamp01(math.Abs(score))}
	if score > 0.15 {
		s.Direction = 1
		s.Reason = "tape + news bullish"
	} else if score < -0.15 {
		s.Direction = -1
		s.Reason = "tape + news bearish"
	} else {
		s.Reason = "tape + news mixed"
	}
	return s
}

func sma(xs []float64, n int) float64 {
	if len(xs) < n || n <= 0 {
		return 0
	}
	s := 0.0
	for i := len(xs) - n; i < len(xs); i++ {
		s += xs[i]
	}
	return s / float64(n)
}

func clamp01(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}
