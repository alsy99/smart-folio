package indstocks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/httpx"
	"aperture/pkg/prices"
	"aperture/pkg/universe"
)

const (
	defaultBase = "https://api.indstocks.com"
	quoteTTL    = 4 * time.Second
	scripTTL    = 12 * time.Hour
)

type Client struct {
	http    *http.Client
	baseURL string
	token   string
	apiKey  string
	mpin    string
	totpSec string
	cache   string

	mu       sync.Mutex
	scrips   map[string]Scrip
	scripAt  time.Time
	quotes   map[string]Quote
	quoteAt  time.Time
	bars     map[string]barSet
	pulse    Pulse
	pulseAt  time.Time
	tokenAt  time.Time
	lastReq  time.Time
}

type Quote struct {
	LivePrice           float64 `json:"live_price"`
	DayChangePct        float64 `json:"day_change_percentage"`
	DayChange           float64 `json:"day_change"`
	DayOpen             float64 `json:"day_open"`
	DayHigh             float64 `json:"day_high"`
	DayLow              float64 `json:"day_low"`
	PrevClose           float64 `json:"prev_close"`
	Volume              float64 `json:"volume"`
}

type Status struct {
	Configured bool   `json:"configured"`
	Mode       string `json:"mode"`
	ProfileOK  bool   `json:"profileOk"`
	Scrips     int    `json:"scrips"`
	Orders     string `json:"orders"`
	Error      string `json:"error,omitempty"`
}

type envelope struct {
	Status  string          `json:"status"`
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Error   string          `json:"error_code"`
	Data    json.RawMessage `json:"data"`
}

func (e envelope) ok() bool {
	if e.Success != nil {
		return *e.Success
	}
	if e.Status == "" {
		return true
	}
	return strings.EqualFold(e.Status, "success")
}

func (e envelope) err() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Error != "" {
		return e.Error
	}
	return "request failed"
}

func decodeEnvelope(b []byte) (envelope, json.RawMessage, error) {
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return env, nil, err
	}
	return env, env.Data, nil
}

var (
	sharedOnce sync.Once
	shared     *Client
	statusMu   sync.Mutex
	statusAt   time.Time
	statusLast Status
)

func Shared() *Client {
	sharedOnce.Do(func() {
		c, err := FromEnv()
		if err != nil {
			slog.Warn("indstocks", "err", err)
			return
		}
		shared = c
	})
	return shared
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func FromEnv() (*Client, error) {
	token := envFirst("INDSTOCKS_ACCESS_TOKEN", "INDMONEY_API_TOKEN", "INDMONEY_ACCESS_TOKEN")
	apiKey := envFirst("INDSTOCKS_API_KEY", "INDMONEY_CLIENT_ID")
	mpin := envFirst("INDSTOCKS_MPIN", "INDMONEY_MPIN")
	totpSec := envFirst("INDSTOCKS_TOTP_SECRET", "INDMONEY_TOTP_SECRET")
	if token == "" && (apiKey == "" || mpin == "" || totpSec == "") {
		return nil, nil
	}
	c := &Client{
		http:    httpx.Client(12 * time.Second),
		baseURL: config.String("INDSTOCKS_BASE_URL", defaultBase),
		token:   token,
		apiKey:  apiKey,
		mpin:    mpin,
		totpSec: totpSec,
		cache:   config.String("INDSTOCKS_CACHE_DIR", "data"),
		quotes:  map[string]Quote{},
		bars:    map[string]barSet{},
	}
	return c, nil
}

func (c *Client) Enabled() bool { return c != nil }

func (c *Client) base() string {
	return strings.TrimRight(c.baseURL, "/")
}

func (c *Client) ScripCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.scrips)
}

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := 250*time.Millisecond - time.Since(c.lastReq)
	c.mu.Unlock()
	if wait <= 0 {
		c.mu.Lock()
		c.lastReq = time.Now()
		c.mu.Unlock()
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		c.mu.Lock()
		c.lastReq = time.Now()
		c.mu.Unlock()
		return nil
	}
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	if err := c.ensureToken(req.Context()); err != nil {
		return nil, err
	}
	if err := c.throttle(req.Context()); err != nil {
		return nil, err
	}
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != "" && req.Header.Get("Authorization") == "" && req.Header.Get("x-api-key") == "" {
		req.Header.Set("Authorization", tok)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Aperture/1.0")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	b, err := httpx.Do(c.http, req)
	if err != nil && strings.Contains(err.Error(), "http 429") {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(350 * time.Millisecond):
		}
		b, err = httpx.Do(c.http, req)
	}
	return b, err
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	tok := c.token
	at := c.tokenAt
	need := tok == "" || (c.totpSec != "" && !at.IsZero() && time.Since(at) > 20*time.Hour)
	c.mu.Unlock()
	if !need {
		return nil
	}
	if c.apiKey == "" || c.mpin == "" || c.totpSec == "" {
		if tok == "" {
			return fmt.Errorf("indstocks: no access token")
		}
		return nil
	}
	code, err := totpCode(c.totpSec, time.Now())
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"mpin": c.mpin, "totp": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/generate/token", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	b, err := httpx.Do(c.http, req)
	if err != nil {
		return err
	}
	var resp struct {
		Status string `json:"status"`
		Token  string `json:"token"`
		Data   struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return err
	}
	next := strings.TrimSpace(resp.Token)
	if next == "" {
		next = strings.TrimSpace(resp.Data.Token)
	}
	if next == "" {
		return fmt.Errorf("indstocks: token response empty")
	}
	c.mu.Lock()
	c.token = next
	c.tokenAt = time.Now()
	c.mu.Unlock()
	slog.Info("indstocks token", "source", "totp")
	return nil
}

func (c *Client) Profile(ctx context.Context) error {
	b, err := c.get(ctx, "/user/profile", nil)
	if err != nil {
		return err
	}
	env, _, err := decodeEnvelope(b)
	if err != nil {
		return err
	}
	if !env.ok() {
		return fmt.Errorf("indstocks: %s", env.err())
	}
	return nil
}

func (c *Client) Funds(ctx context.Context) (json.RawMessage, error) {
	b, err := c.get(ctx, "/funds", nil)
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
	return data, nil
}

func (c *Client) EnsureScrips(ctx context.Context) error {
	c.mu.Lock()
	if len(c.scrips) > 0 && time.Since(c.scripAt) < scripTTL {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	eq, err := c.instruments(ctx, "equity")
	if err != nil {
		if cached := c.loadCached("equity.csv"); cached != nil {
			eq = cached
		} else {
			return err
		}
	} else {
		c.saveCached("equity.csv", eq)
	}
	idx, err := c.instruments(ctx, "index")
	if err != nil {
		if cached := c.loadCached("index.csv"); cached != nil {
			idx = cached
		} else {
			idx = map[string]Scrip{}
		}
	} else {
		c.saveCached("index.csv", idx)
	}
	mapped := mapUniverse(eq, idx)
	c.mu.Lock()
	c.scrips = mapped
	c.scripAt = time.Now()
	n := len(mapped)
	c.mu.Unlock()
	slog.Info("indstocks scrips", "mapped", n)
	return nil
}

func (c *Client) instruments(ctx context.Context, source string) (map[string]Scrip, error) {
	q := url.Values{"source": {source}}
	b, err := c.get(ctx, "/market/instruments", q)
	if err != nil {
		return nil, err
	}
	if source == "index" {
		return parseIndexCSV(bytes.NewReader(b))
	}
	return parseEquityCSV(bytes.NewReader(b))
}

func (c *Client) saveCached(name string, m map[string]Scrip) {
	_ = os.MkdirAll(c.cache, 0o755)
	var b strings.Builder
	b.WriteString("SYMBOL,CODE\n")
	for sym, s := range m {
		fmt.Fprintf(&b, "%s,%s\n", sym, s.Code)
	}
	_ = os.WriteFile(filepath.Join(c.cache, "indstocks-"+name), []byte(b.String()), 0o644)
}

func (c *Client) loadCached(name string) map[string]Scrip {
	b, err := os.ReadFile(filepath.Join(c.cache, "indstocks-"+name))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	out := map[string]Scrip{}
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		sym, code, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		sym, code = strings.TrimSpace(sym), strings.TrimSpace(code)
		out[sym] = Scrip{Code: code, Symbol: sym}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Client) scrip(symbol string) (Scrip, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.scrips[symbol]
	return s, ok
}

func (c *Client) Quotes(ctx context.Context, symbols []string) (map[string]Quote, error) {
	if err := c.EnsureScrips(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if time.Since(c.quoteAt) < quoteTTL && len(c.quotes) > 0 {
		cp := copyQuotes(c.quotes)
		c.mu.Unlock()
		return filterQuotes(cp, symbols), nil
	}
	c.mu.Unlock()

	var codes []string
	codeToSym := map[string]string{}
	for _, sym := range symbols {
		if !quoteable(sym) {
			continue
		}
		s, ok := c.scrip(sym)
		if !ok {
			continue
		}
		codes = append(codes, s.Code)
		codeToSym[s.Code] = sym
	}
	if len(codes) == 0 {
		return map[string]Quote{}, nil
	}
	q := url.Values{"scrip-codes": {strings.Join(codes, ",")}}
	b, err := c.get(ctx, "/market/quotes/full", q)
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
	var byCode map[string]Quote
	if err := json.Unmarshal(data, &byCode); err != nil {
		return nil, err
	}
	out := map[string]Quote{}
	for code, qt := range byCode {
		sym := codeToSym[code]
		if sym == "" {
			continue
		}
		out[sym] = qt
	}
	c.mu.Lock()
	c.quotes = out
	c.quoteAt = time.Now()
	c.mu.Unlock()
	return filterQuotes(out, symbols), nil
}

func copyQuotes(in map[string]Quote) map[string]Quote {
	out := make(map[string]Quote, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func filterQuotes(in map[string]Quote, symbols []string) map[string]Quote {
	if len(symbols) == 0 {
		return copyQuotes(in)
	}
	out := map[string]Quote{}
	for _, s := range symbols {
		if q, ok := in[s]; ok {
			out[s] = q
		}
	}
	return out
}

type candle struct {
	Ts int64   `json:"ts"`
	O  float64 `json:"o"`
	H  float64 `json:"h"`
	L  float64 `json:"l"`
	C  float64 `json:"c"`
	V  float64 `json:"v"`
}

func mapInterval(desk string) (label string, step time.Duration, maxRange time.Duration) {
	switch desk {
	case "5m":
		return "5minute", 5 * time.Minute, 7 * 24 * time.Hour
	case "1h":
		return "60minute", time.Hour, 15 * 24 * time.Hour
	case "1d":
		return "1day", 24 * time.Hour, 365 * 24 * time.Hour
	case "1w":
		return "1week", 7 * 24 * time.Hour, 365 * 24 * time.Hour
	default:
		return "15minute", 15 * time.Minute, 7 * 24 * time.Hour
	}
}

type barSet struct {
	at   time.Time
	bars []prices.Bar
}

func barTTL(interval string) time.Duration {
	switch interval {
	case "1d", "1w":
		return 15 * time.Minute
	case "1h":
		return 2 * time.Minute
	default:
		return 45 * time.Second
	}
}

func barKey(symbol, interval string) string {
	return symbol + "|" + interval
}

func candlesToBars(cds []candle, count int) []prices.Bar {
	out := make([]prices.Bar, 0, len(cds))
	for _, cd := range cds {
		out = append(out, prices.Bar{
			Ts:     time.Unix(cd.Ts, 0),
			Open:   cd.O,
			High:   cd.H,
			Low:    cd.L,
			Close:  cd.C,
			Volume: cd.V,
		})
	}
	if count > 0 && len(out) > count {
		out = out[len(out)-count:]
	}
	return out
}

func (c *Client) Bars(ctx context.Context, symbol, interval string, count int, now time.Time) ([]prices.Bar, error) {
	if interval == "" || interval == "session" {
		interval = "5m"
	}
	if count <= 0 {
		count = 40
	}
	if err := c.EnsureScrips(ctx); err != nil {
		return nil, err
	}
	if !quoteable(symbol) {
		return nil, fmt.Errorf("indstocks: no equity quotes for %s", symbol)
	}
	c.mu.Lock()
	if c.bars == nil {
		c.bars = map[string]barSet{}
	}
	if hit, ok := c.bars[barKey(symbol, interval)]; ok && time.Since(hit.at) < barTTL(interval) && len(hit.bars) > 0 {
		out := hit.bars
		c.mu.Unlock()
		if len(out) > count {
			out = out[len(out)-count:]
		}
		return out, nil
	}
	c.mu.Unlock()

	var missing []string
	seen := map[string]struct{}{}
	for _, sym := range append([]string{symbol}, universe.EquitySymbols()...) {
		if _, dup := seen[sym]; dup {
			continue
		}
		if !quoteable(sym) {
			continue
		}
		if _, ok := c.scrip(sym); !ok {
			continue
		}
		c.mu.Lock()
		hit, ok := c.bars[barKey(sym, interval)]
		fresh := ok && time.Since(hit.at) < barTTL(interval) && len(hit.bars) > 0
		c.mu.Unlock()
		if fresh {
			continue
		}
		seen[sym] = struct{}{}
		missing = append(missing, sym)
	}
	if err := c.fetchBars(ctx, missing, interval, count, now); err != nil {
		return nil, err
	}
	c.mu.Lock()
	hit := c.bars[barKey(symbol, interval)]
	c.mu.Unlock()
	if len(hit.bars) == 0 {
		return nil, fmt.Errorf("indstocks: no candles for %s", symbol)
	}
	out := hit.bars
	if len(out) > count {
		out = out[len(out)-count:]
	}
	return out, nil
}

func (c *Client) fetchBars(ctx context.Context, symbols []string, interval string, count int, now time.Time) error {
	if len(symbols) == 0 {
		return nil
	}
	label, step, maxRange := mapInterval(interval)
	end := now
	start := end.Add(-time.Duration(count) * step)
	if end.Sub(start) > maxRange {
		start = end.Add(-maxRange)
	}
	for i := 0; i < len(symbols); i += 5 {
		chunk := symbols[i:]
		if len(chunk) > 5 {
			chunk = chunk[:5]
		}
		var codes []string
		codeToSym := map[string]string{}
		for _, sym := range chunk {
			s, ok := c.scrip(sym)
			if !ok {
				continue
			}
			codes = append(codes, s.Code)
			codeToSym[s.Code] = sym
		}
		if len(codes) == 0 {
			continue
		}
		q := url.Values{
			"scrip-codes": {strings.Join(codes, ",")},
			"start_time":  {fmt.Sprintf("%d", start.UnixMilli())},
			"end_time":    {fmt.Sprintf("%d", end.UnixMilli())},
		}
		b, err := c.get(ctx, "/market/historical/"+label, q)
		if err != nil {
			return err
		}
		env, data, err := decodeEnvelope(b)
		if err != nil {
			return err
		}
		if !env.ok() {
			return fmt.Errorf("indstocks: %s", env.err())
		}
		var wrap map[string]struct {
			Candles []candle `json:"candles"`
		}
		if err := json.Unmarshal(data, &wrap); err != nil {
			return err
		}
		nowHit := time.Now()
		c.mu.Lock()
		if c.bars == nil {
			c.bars = map[string]barSet{}
		}
		for code, row := range wrap {
			sym := codeToSym[code]
			if sym == "" {
				continue
			}
			c.bars[barKey(sym, interval)] = barSet{at: nowHit, bars: candlesToBars(row.Candles, count)}
		}
		c.mu.Unlock()
	}
	return nil
}

func Snapshot(ctx context.Context) Status {
	statusMu.Lock()
	if time.Since(statusAt) < 45*time.Second && statusLast.Mode != "" {
		st := statusLast
		statusMu.Unlock()
		return st
	}
	statusMu.Unlock()

	st := Status{Mode: "mock", Orders: "paper"}
	c := Shared()
	if c == nil {
		statusMu.Lock()
		statusLast, statusAt = st, time.Now()
		statusMu.Unlock()
		return st
	}
	st.Configured = true
	if err := c.EnsureScrips(ctx); err != nil {
		st.Error = err.Error()
		statusMu.Lock()
		statusLast, statusAt = st, time.Now()
		statusMu.Unlock()
		return st
	}
	st.Scrips = c.ScripCount()
	if err := c.Profile(ctx); err != nil {
		st.Error = err.Error()
		st.Mode = "degraded"
		statusMu.Lock()
		statusLast, statusAt = st, time.Now()
		statusMu.Unlock()
		return st
	}
	st.ProfileOK = true
	st.Mode = "live"
	statusMu.Lock()
	statusLast, statusAt = st, time.Now()
	statusMu.Unlock()
	return st
}

func QuoteOrMock(ctx context.Context, symbol string, now time.Time) (last, change float64, live bool) {
	c := Shared()
	if c == nil {
		return prices.Last(symbol, now), prices.ChangePct(symbol, now), false
	}
	qs, err := c.Quotes(ctx, []string{symbol})
	if err != nil || qs[symbol].LivePrice == 0 {
		return prices.Last(symbol, now), prices.ChangePct(symbol, now), false
	}
	q := qs[symbol]
	return q.LivePrice, q.DayChangePct, true
}

func BarsOrMock(ctx context.Context, symbol, interval string, count int, now time.Time) ([]prices.Bar, bool) {
	c := Shared()
	if c == nil {
		return prices.Bars(symbol, interval, count, now), false
	}
	bars, err := c.Bars(ctx, symbol, interval, count, now)
	if err != nil || len(bars) == 0 {
		return prices.Bars(symbol, interval, count, now), false
	}
	return bars, true
}
