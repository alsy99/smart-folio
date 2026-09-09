package prices

import (
	"hash/fnv"
	"math"
	"time"

	"aperture/pkg/universe"
)

func Hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func Last(symbol string, now time.Time) float64 {
	inst, ok := universe.Lookup(symbol)
	if !ok {
		inst = universe.Instrument{Symbol: symbol, BasePrice: 100}
	}
	seed := float64(Hash(symbol)%1000) / 1000.0
	t := float64(now.Unix()) / 60.0
	drift := 0.0
	switch symbol {
	case "NIFTY50":
		drift = 0.00002
	case "NIFTY500":
		drift = 0.000018
	case "SENSEX":
		drift = 0.000019
	case "MF_LARGECAP":
		drift = 0.000028
	case "MF_FLEXI":
		drift = 0.000031
	}
	wave := math.Sin(t/17.0+seed*6.28)*0.012 + math.Sin(t/41.0+seed)*0.006
	tickNoise := math.Sin(t*1.7+seed*3)*0.004
	day := float64(now.Unix()/60) * drift
	px := inst.BasePrice * (1 + wave + tickNoise + day)
	if px < 1 {
		px = 1
	}
	return math.Round(px*100) / 100
}

func ChangePct(symbol string, now time.Time) float64 {
	cur := Last(symbol, now)
	prev := Last(symbol, now.Add(-24*time.Hour))
	if prev == 0 {
		return 0
	}
	return (cur/prev - 1) * 100
}

func Bars(symbol, interval string, count int, now time.Time) []struct {
	Ts     time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
} {
	if count <= 0 {
		count = 40
	}
	step := 15 * time.Minute
	switch interval {
	case "5m":
		step = 5 * time.Minute
	case "15m":
		step = 15 * time.Minute
	case "1h":
		step = time.Hour
	case "1d":
		step = 24 * time.Hour
	}
	out := make([]struct {
		Ts     time.Time
		Open   float64
		High   float64
		Low    float64
		Close  float64
		Volume float64
	}, count)
	for i := 0; i < count; i++ {
		ts := now.Add(-time.Duration(count-1-i) * step)
		c := Last(symbol, ts)
		o := Last(symbol, ts.Add(-step/2))
		hi := math.Max(o, c) * (1 + 0.003)
		lo := math.Min(o, c) * (1 - 0.003)
		out[i] = struct {
			Ts     time.Time
			Open   float64
			High   float64
			Low    float64
			Close  float64
			Volume float64
		}{Ts: ts, Open: o, High: hi, Low: lo, Close: c, Volume: 1e6 + float64(Hash(symbol+ts.String())%500000)}
	}
	return out
}
