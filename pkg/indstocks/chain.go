package indstocks

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/universe"
)

const pulseTTL = 2 * time.Minute

// Pulse is Nifty option-chain positioning. INDstocks has no news-sentiment
// endpoint; PCR and ATM IV are the live tape read.
type Pulse struct {
	Expiry   string
	LTP      float64
	CallOI   float64
	PutOI    float64
	PCR      float64
	ATMIV    float64
	Tilt     float64
	Headline string
}

type chainLeg struct {
	OI     float64 `json:"oi"`
	PrevOI float64 `json:"previous_oi"`
	Volume float64 `json:"volume"`
	IV     float64 `json:"iv"`
}

type chainStrike struct {
	CE chainLeg `json:"ce"`
	PE chainLeg `json:"pe"`
}

func (c *Client) Expiries(ctx context.Context, underlying string) ([]string, error) {
	q := url.Values{"underlying": {underlying}, "segment": {"DERIVATIVE"}}
	b, err := c.get(ctx, "/market/instruments/expiries", q)
	if err != nil {
		return nil, err
	}
	env, data, err := decodeEnvelope(b)
	if err != nil {
		return nil, err
	}
	if !env.ok() {
		return nil, fmt.Errorf("indstocks: %s", env.err())
	}
	var dates []string
	if err := json.Unmarshal(data, &dates); err != nil {
		return nil, err
	}
	return dates, nil
}

func (c *Client) OptionChain(ctx context.Context, segment, scrip, expiry string, strikes int) (ltp float64, ladder map[string]chainStrike, err error) {
	if strikes <= 0 {
		strikes = 8
	}
	q := url.Values{
		"exchange":         {"NSE"},
		"segment":          {segment},
		"underlying-scrip": {scrip},
		"expiry":           {expiry},
		"strike_count":     {strconv.Itoa(strikes)},
	}
	b, err := c.get(ctx, "/market/option-chain", q)
	if err != nil {
		return 0, nil, err
	}
	env, data, err := decodeEnvelope(b)
	if err != nil {
		return 0, nil, err
	}
	if !env.ok() {
		return 0, nil, fmt.Errorf("indstocks: %s", env.err())
	}
	var wrap struct {
		UnderlyingLTP float64                `json:"underlying_ltp"`
		Strikes       map[string]chainStrike `json:"strikes"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return 0, nil, err
	}
	return wrap.UnderlyingLTP, wrap.Strikes, nil
}

func nearestExpiry(dates []string, now time.Time) string {
	today := now.In(ist()).Format("2006-01-02")
	for _, d := range dates {
		if d >= today {
			return d
		}
	}
	if len(dates) == 0 {
		return ""
	}
	return dates[len(dates)-1]
}

func ist() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func (c *Client) Pulse(ctx context.Context) (Pulse, error) {
	c.mu.Lock()
	if time.Since(c.pulseAt) < pulseTTL && c.pulse.PCR > 0 {
		p := c.pulse
		c.mu.Unlock()
		return p, nil
	}
	c.mu.Unlock()

	if err := c.EnsureScrips(ctx); err != nil {
		return Pulse{}, err
	}
	dates, err := c.Expiries(ctx, "NIFTY")
	if err != nil {
		return Pulse{}, err
	}
	expiry := nearestExpiry(dates, time.Now())
	if expiry == "" {
		return Pulse{}, fmt.Errorf("indstocks: no nifty expiry")
	}
	scrip := "40000001"
	if s, ok := c.scrip("NIFTY50"); ok && s.Token != "" {
		scrip = s.Token
	}
	ltp, ladder, err := c.OptionChain(ctx, "INDEX", scrip, expiry, 8)
	if err != nil {
		return Pulse{}, err
	}
	p := scorePulse(expiry, ltp, ladder)
	c.mu.Lock()
	c.pulse, c.pulseAt = p, time.Now()
	c.mu.Unlock()
	return p, nil
}

func scorePulse(expiry string, ltp float64, ladder map[string]chainStrike) Pulse {
	p := Pulse{Expiry: expiry, LTP: ltp}
	bestAbs := math.MaxFloat64
	var atmIV float64
	for k, st := range ladder {
		strike, err := strconv.ParseFloat(k, 64)
		p.CallOI += st.CE.OI
		p.PutOI += st.PE.OI
		if err != nil || ltp == 0 {
			continue
		}
		d := math.Abs(strike - ltp)
		if d < bestAbs {
			bestAbs = d
			n, sum := 0.0, 0.0
			if st.CE.IV > 0 {
				sum += st.CE.IV
				n++
			}
			if st.PE.IV > 0 {
				sum += st.PE.IV
				n++
			}
			if n > 0 {
				atmIV = sum / n
			}
		}
	}
	p.ATMIV = atmIV
	if p.CallOI > 0 {
		p.PCR = p.PutOI / p.CallOI
	}
	// High PCR = crowded puts (defensive). Low PCR = crowded calls (risk-on).
	p.Tilt = clamp((1-p.PCR)/0.8, -0.45, 0.45)
	side := "balanced"
	switch {
	case p.PCR >= 1.2:
		side = "put-heavy (defensive)"
	case p.PCR > 0 && p.PCR <= 0.8:
		side = "call-heavy (risk-on)"
	}
	p.Headline = fmt.Sprintf("Nifty F&O %s: PCR %.2f, ATM IV %.1f%%, LTP %.0f — %s", expiry, p.PCR, p.ATMIV, p.LTP, side)
	return p
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (p Pulse) Report() *commonv1.InvestigationReport {
	stance, score := "mixed", p.Tilt
	switch {
	case p.Tilt > 0.12:
		stance = "bullish"
	case p.Tilt < -0.12:
		stance = "bearish"
	}
	syms := universe.EquitySymbols()
	return &commonv1.InvestigationReport{
		Id: "tape-nifty-pcr", Headline: p.Headline, Symbols: syms,
		EventType: "flow", Stance: stance, Score: score, Confidence: 0.62,
		Corroboration: 1, Horizon: "position", StandAside: p.ATMIV > 28,
		AnalyzedAtUnixMs: time.Now().UnixMilli(), Mode: "live",
		Thesis: fmt.Sprintf("INDstocks option chain PCR %.2f (put OI %.0f / call OI %.0f). Tilt applied to the paper book; not a live F&O ticket.", p.PCR, p.PutOI, p.CallOI),
		Risks:  "PCR is positioning, not a forecast. Weekly expiry pin and hedging flows can flip the read.",
		Status: "done",
		StrategyImplications: []*commonv1.StrategyTilt{
			{StrategyId: "sentiment_tilt", Tilt: p.Tilt},
			{StrategyId: "momentum_1d", Tilt: p.Tilt * 0.5},
			{StrategyId: "mean_reversion_1d", Tilt: -p.Tilt * 0.4},
		},
		Sources: []*commonv1.NewsItem{{
			Id: "indstocks-option-chain", Provider: "indstocks",
			Title: p.Headline, Summary: "Nifty option-chain PCR and ATM IV",
			PublishedAtUnixMs: time.Now().UnixMilli(), Symbols: []string{"NIFTY50"},
		}},
	}
}

func OverlayPulse(ctx context.Context, scores map[string]*commonv1.SentimentScore, reports []*commonv1.InvestigationReport) []*commonv1.InvestigationReport {
	c := Shared()
	if c == nil {
		return reports
	}
	p, err := c.Pulse(ctx)
	if err != nil || p.PCR == 0 {
		return reports
	}
	rep := p.Report()
	out := make([]*commonv1.InvestigationReport, 0, len(reports)+1)
	out = append(out, rep)
	for _, r := range reports {
		if r != nil {
			out = append(out, r)
		}
	}
	if scores == nil {
		return out
	}
	for _, sym := range universe.EquitySymbols() {
		sc := scores[sym]
		if sc == nil {
			sc = &commonv1.SentimentScore{Symbol: sym, Confidence: 0.5}
			scores[sym] = sc
		}
		sc.Score += p.Tilt
		if !strings.Contains(strings.Join(sc.Drivers, " "), "PCR") {
			sc.Drivers = append(sc.Drivers, p.Headline)
		}
		switch {
		case sc.Score > 0.15:
			sc.Label = "bullish"
		case sc.Score < -0.15:
			sc.Label = "bearish"
		default:
			sc.Label = "neutral"
		}
	}
	return out
}
