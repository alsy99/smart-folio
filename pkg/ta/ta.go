package ta

import (
	"fmt"
	"math"

	"aperture/pkg/prices"
)

type Snapshot struct {
	Symbol    string
	Last      float64
	ChangePct float64
	SMAFast   float64
	SMASlow   float64
	Momentum  float64
	RSI       float64
	RangePos  float64
	High20    float64
	Low20     float64
	Trend     string
	Setup     string
}

func FromBars(symbol string, bars []prices.Bar) Snapshot {
	s := Snapshot{Symbol: symbol, Trend: "chop", Setup: "no clear setup"}
	if len(bars) == 0 {
		return s
	}
	closes := make([]float64, len(bars))
	highs := make([]float64, len(bars))
	lows := make([]float64, len(bars))
	for i, b := range bars {
		closes[i], highs[i], lows[i] = b.Close, b.High, b.Low
	}
	s.Last = closes[len(closes)-1]
	if len(closes) > 1 && closes[len(closes)-2] != 0 {
		s.ChangePct = (s.Last/closes[len(closes)-2] - 1) * 100
	}
	s.SMAFast = sma(closes, 5)
	s.SMASlow = sma(closes, 20)
	if len(closes) > 5 && closes[len(closes)-6] != 0 {
		s.Momentum = (s.Last/closes[len(closes)-6] - 1) * 100
	}
	s.RSI = rsi(closes, 14)
	s.High20, s.Low20 = maxMin(highs, lows, 20)
	if s.High20 > s.Low20 {
		s.RangePos = (s.Last - s.Low20) / (s.High20 - s.Low20)
	}
	switch {
	case s.SMAFast > 0 && s.SMASlow > 0 && s.SMAFast > s.SMASlow*1.002 && s.Momentum > 0:
		s.Trend = "uptrend"
	case s.SMAFast > 0 && s.SMASlow > 0 && s.SMAFast < s.SMASlow*0.998 && s.Momentum < 0:
		s.Trend = "downtrend"
	}
	switch {
	case s.RangePos > 0.97 && s.Trend == "uptrend":
		s.Setup = "breakout / range high"
	case s.RangePos < 0.15 && s.Trend != "downtrend":
		s.Setup = "oversold bounce candidate"
	case s.RSI >= 70:
		s.Setup = "stretched; fade or wait"
	case s.RSI > 0 && s.RSI <= 30:
		s.Setup = "RSI oversold"
	case s.Trend == "uptrend":
		s.Setup = "trend continuation"
	case s.Trend == "downtrend":
		s.Setup = "trend down; avoid fresh longs"
	}
	return s
}

func (s Snapshot) Summary() string {
	return fmt.Sprintf(
		"%s last %.2f (bar %+0.2f%%) SMA5 %.2f SMA20 %.2f mom5 %+0.2f%% RSI %.0f range-pos %.0f%% high20 %.2f low20 %.2f trend %s setup %s",
		s.Symbol, s.Last, s.ChangePct, s.SMAFast, s.SMASlow, s.Momentum, s.RSI, s.RangePos*100, s.High20, s.Low20, s.Trend, s.Setup,
	)
}

func sma(xs []float64, n int) float64 {
	if len(xs) < n || n <= 0 {
		return 0
	}
	sum := 0.0
	for i := len(xs) - n; i < len(xs); i++ {
		sum += xs[i]
	}
	return math.Round(sum/float64(n)*100) / 100
}

func rsi(closes []float64, n int) float64 {
	if len(closes) <= n {
		return 50
	}
	var gain, loss float64
	for i := len(closes) - n; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		if d > 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	if loss == 0 {
		return 100
	}
	rs := (gain / float64(n)) / (loss / float64(n))
	return math.Round((100-100/(1+rs))*10) / 10
}

func maxMin(highs, lows []float64, n int) (float64, float64) {
	if len(highs) == 0 || len(lows) == 0 {
		return 0, 0
	}
	start := 0
	if len(highs) > n {
		start = len(highs) - n
	}
	mx, mn := highs[start], lows[start]
	for i := start; i < len(highs) && i < len(lows); i++ {
		if highs[i] > mx {
			mx = highs[i]
		}
		if lows[i] < mn {
			mn = lows[i]
		}
	}
	return mx, mn
}
