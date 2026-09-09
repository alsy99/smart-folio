package strategies

import "math"

const (
	SMACross15m     = "sma_cross_15m"
	Momentum5m      = "momentum_5m"
	MeanReversion15 = "mean_reversion_15m"
	Breakout1h      = "breakout_1h"
	SwingDaily      = "swing_daily"
	SentimentTilt   = "sentiment_tilt"
	OpeningRange    = "opening_range"
)

func IDs() []string {
	return []string{
		SMACross15m, Momentum5m, MeanReversion15, Breakout1h, SwingDaily, SentimentTilt, OpeningRange,
	}
}

type Signal struct {
	StrategyID string
	Symbol     string
	Direction  int     // -1 short/sell, 0 flat, +1 buy
	Score      float64 // 0..1
	Reason     string
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

func SMACross(symbol string, closes []float64) Signal {
	fast, slow := sma(closes, 5), sma(closes, 20)
	s := Signal{StrategyID: SMACross15m, Symbol: symbol}
	if fast == 0 || slow == 0 {
		return s
	}
	diff := (fast - slow) / slow
	s.Score = math.Min(1, math.Abs(diff)*40)
	if diff > 0.001 {
		s.Direction = 1
		s.Reason = "15m fast SMA crossed above slow"
	} else if diff < -0.001 {
		s.Direction = -1
		s.Reason = "15m fast SMA crossed below slow"
	}
	return s
}

func Momentum(symbol string, closes []float64) Signal {
	s := Signal{StrategyID: Momentum5m, Symbol: symbol}
	if len(closes) < 4 {
		return s
	}
	ret := closes[len(closes)-1]/closes[len(closes)-4] - 1
	s.Score = math.Min(1, math.Abs(ret)*25)
	if ret > 0.002 {
		s.Direction = 1
		s.Reason = "5m thrust higher"
	} else if ret < -0.002 {
		s.Direction = -1
		s.Reason = "5m thrust lower"
	}
	return s
}

func MeanRevert(symbol string, closes []float64) Signal {
	s := Signal{StrategyID: MeanReversion15, Symbol: symbol}
	m := sma(closes, 20)
	if m == 0 || len(closes) == 0 {
		return s
	}
	dev := (closes[len(closes)-1] - m) / m
	s.Score = math.Min(1, math.Abs(dev)*20)
	if dev > 0.008 {
		s.Direction = -1
		s.Reason = "extended above 15m VWAP/SMA"
	} else if dev < -0.008 {
		s.Direction = 1
		s.Reason = "extended below 15m SMA"
	}
	return s
}

func Breakout(symbol string, highs, closes []float64) Signal {
	s := Signal{StrategyID: Breakout1h, Symbol: symbol}
	if len(highs) < 21 || len(closes) == 0 {
		return s
	}
	mx := highs[0]
	for i := len(highs) - 21; i < len(highs)-1; i++ {
		if highs[i] > mx {
			mx = highs[i]
		}
	}
	last := closes[len(closes)-1]
	if last > mx {
		s.Direction = 1
		s.Score = math.Min(1, (last/mx-1)*40)
		s.Reason = "1h range high break"
	}
	return s
}

func Swing(symbol string, closes []float64) Signal {
	s := Signal{StrategyID: SwingDaily, Symbol: symbol}
	fast, slow := sma(closes, 10), sma(closes, 30)
	if slow == 0 {
		return s
	}
	diff := (fast - slow) / slow
	s.Score = math.Min(1, math.Abs(diff)*15)
	if diff > 0 {
		s.Direction = 1
		s.Reason = "daily trend up"
	} else {
		s.Direction = -1
		s.Reason = "daily trend down"
	}
	return s
}

func OpeningRangeBreak(symbol string, highs, lows, closes []float64) Signal {
	s := Signal{StrategyID: OpeningRange, Symbol: symbol}
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

func Sentiment(symbol string, score float64) Signal {
	s := Signal{StrategyID: SentimentTilt, Symbol: symbol, Score: math.Min(1, math.Abs(score))}
	if score > 0.15 {
		s.Direction = 1
		s.Reason = "news investigation bullish"
	} else if score < -0.15 {
		s.Direction = -1
		s.Reason = "news investigation bearish"
	} else {
		s.Reason = "news investigation mixed"
	}
	return s
}
