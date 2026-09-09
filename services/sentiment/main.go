package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	commonv1 "aperture/go/gen/common/v1"
	sentimentv1 "aperture/go/gen/sentiment/v1"
	"aperture/pkg/strategies"
	"aperture/pkg/universe"

	"google.golang.org/grpc"
)

type news struct {
	ID, Provider, Title, Summary, URL string
	Published                         time.Time
	Symbols                           []string
}

type report struct {
	*commonv1.InvestigationReport
}

type server struct {
	sentimentv1.UnimplementedSentimentServiceServer
	mu          sync.RWMutex
	news        []news
	reports     []*commonv1.InvestigationReport
	scores      map[string]*commonv1.SentimentScore
	lastRefresh time.Time
	mode        string
	workers     int
	maxWorkers  int
}

func getenvInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (s *server) ScoreSymbols(ctx context.Context, req *sentimentv1.ScoreSymbolsRequest) (*sentimentv1.ScoreSymbolsResponse, error) {
	s.maybeRefresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	syms := req.Symbols
	if len(syms) == 0 {
		syms = universe.EquitySymbols()
	}
	var out []*commonv1.SentimentScore
	for _, sym := range syms {
		if sc, ok := s.scores[sym]; ok {
			out = append(out, sc)
		} else {
			out = append(out, &commonv1.SentimentScore{Symbol: sym, Label: "neutral", Confidence: 0.4})
		}
	}
	return &sentimentv1.ScoreSymbolsResponse{Scores: out}, nil
}

func (s *server) ListNews(ctx context.Context, req *sentimentv1.ListNewsRequest) (*sentimentv1.ListNewsResponse, error) {
	s.maybeRefresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	lim := int(req.Limit)
	if lim <= 0 || lim > len(s.news) {
		lim = len(s.news)
	}
	var items []*commonv1.NewsItem
	for i := 0; i < lim; i++ {
		n := s.news[i]
		items = append(items, toNewsItem(n))
	}
	return &sentimentv1.ListNewsResponse{Items: items, Mode: s.mode, LastRefreshUnixMs: s.lastRefresh.UnixMilli()}, nil
}

func (s *server) GetInvestigations(ctx context.Context, _ *sentimentv1.GetInvestigationsRequest) (*sentimentv1.GetInvestigationsResponse, error) {
	s.maybeRefresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &sentimentv1.GetInvestigationsResponse{
		Reports:           s.reports,
		WorkersActive:     int32(s.workers),
		WorkersMax:        int32(s.maxWorkers),
		LastRefreshUnixMs: s.lastRefresh.UnixMilli(),
		Mode:              s.mode,
	}, nil
}

func toNewsItem(n news) *commonv1.NewsItem {
	return &commonv1.NewsItem{
		Id: n.ID, Provider: n.Provider, Title: n.Title, Summary: n.Summary, Url: n.URL,
		PublishedAtUnixMs: n.Published.UnixMilli(), Symbols: n.Symbols,
	}
}

func (s *server) maybeRefresh() {
	ttl := time.Duration(getenvInt("NEWS_REFRESH_SECONDS", 900)) * time.Second
	s.mu.RLock()
	fresh := time.Since(s.lastRefresh) < ttl && len(s.reports) > 0
	s.mu.RUnlock()
	if fresh {
		return
	}
	s.refresh()
}

func (s *server) refresh() {
	items, mode := gatherNews()
	clusters := clusterNews(items)
	maxW := getenvInt("MAX_INVESTIGATION_WORKERS", 16)
	minMat := 0.35
	if v := os.Getenv("INVESTIGATION_MATERIALITY_MIN"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			minMat = f
		}
	}
	type ranked struct {
		key string
		ns  []news
		mat float64
	}
	var jobs []ranked
	for k, ns := range clusters {
		mat := materiality(ns)
		if mat >= minMat {
			jobs = append(jobs, ranked{k, ns, mat})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].mat > jobs[j].mat })
	if len(jobs) > maxW {
		jobs = jobs[:maxW]
	}

	s.mu.Lock()
	s.workers = len(jobs)
	s.maxWorkers = maxW
	s.mu.Unlock()

	reports := make([]*commonv1.InvestigationReport, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxW)
	for i, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, job ranked) {
			defer wg.Done()
			defer func() { <-sem }()
			reports[i] = investigate(job.ns, mode)
		}(i, job)
	}
	wg.Wait()

	scores := map[string]*commonv1.SentimentScore{}
	for _, r := range reports {
		for _, sym := range r.Symbols {
			cur := scores[sym]
			if cur == nil {
				cur = &commonv1.SentimentScore{Symbol: sym}
			}
			cur.Score += r.Score * r.Confidence
			cur.Confidence = (cur.Confidence + r.Confidence) / 2
			if r.Thesis != "" {
				cur.Drivers = append(cur.Drivers, r.Headline)
			}
			scores[sym] = cur
		}
	}
	for sym, sc := range scores {
		switch {
		case sc.Score > 0.15:
			sc.Label = "bullish"
		case sc.Score < -0.15:
			sc.Label = "bearish"
		default:
			sc.Label = "neutral"
		}
		if sc.Confidence < 0.35 {
			sc.Confidence = 0.35
		}
		if sc.Confidence > 0.92 {
			sc.Confidence = 0.92
		}
		_ = sym
	}

	s.mu.Lock()
	s.news = items
	s.reports = reports
	s.scores = scores
	s.lastRefresh = time.Now()
	s.mode = mode
	s.workers = 0
	s.mu.Unlock()
}

func gatherNews() ([]news, string) {
	var mu sync.Mutex
	var all []news
	mode := "mock"
	var wg sync.WaitGroup
	fetchers := []func() []news{
		func() []news { return fetchNewsAPI() },
		func() []news { return fetchCurrents() },
		func() []news { return fetchFinnhub() },
	}
	for _, f := range fetchers {
		wg.Add(1)
		go func(fn func() []news) {
			defer wg.Done()
			got := fn()
			if len(got) == 0 {
				return
			}
			mu.Lock()
			all = append(all, got...)
			mode = "live"
			mu.Unlock()
		}(f)
	}
	wg.Wait()
	if len(all) == 0 {
		return mockNews(), "mock"
	}
	return all, mode
}

func mockNews() []news {
	now := time.Now()
	raw := []struct {
		title, sum, sym, evt string
	}{
		{"Reliance Jio expansion plan lifts energy complex", "Capex guidance raised for digital and retail arms.", "RELIANCE", "guidance"},
		{"TCS wins multi-year European banking mandate", "Deal size seen above Street estimates; hiring freeze eased.", "TCS", "product"},
		{"HDFC Bank deposit growth cools in fortnight data", "CASA mix weaker; NIM commentary cautious.", "HDFCBANK", "earnings"},
		{"SEBI reviews promoter pledge rules for large-caps", "Draft paper could affect leverage at holding companies.", "RELIANCE", "regulation"},
		{"Infosys guidance range unchanged after deal wins", "Management sticks to FY band; large deal TCV up.", "INFY", "guidance"},
		{"ICICI Bank asset quality print beats peers", "Slippages lower; treasury gains aid NII.", "ICICIBANK", "earnings"},
		{"Airtel ARPU climb continues in metro circles", "5G traffic mix improving; tariff hikes holding.", "BHARTIARTL", "earnings"},
		{"Rumor: ITC hotel demerger timeline in play again", "Unconfirmed report; street waits for board note.", "ITC", "rumor"},
		{"L&T infrastructure order inflow surprises", "Domestic + Middle East mix; margins stable.", "LT", "product"},
		{"RBI holds rates; banks digest liquidity stance", "Nifty bank futures muted into close.", "HDFCBANK", "macro"},
		{"Kotak wholesale book growth re-accelerates", "Management flags competitive pricing.", "KOTAKBANK", "earnings"},
		{"HUL volume recovery in rural still uneven", "Premium mix offsets commodity deflation.", "HINDUNILVR", "earnings"},
		{"Block deal chatter in Axis Bank promoter residual", "Flows unconfirmed; volumes spiked at open.", "AXISBANK", "flow"},
		{"SBI credit growth stays ahead of system", "PSU bank bid into rate-cut hopes.", "SBIN", "earnings"},
	}
	out := make([]news, 0, len(raw)*2)
	for i, r := range raw {
		pub := now.Add(-time.Duration(20+i*7) * time.Minute)
		n := news{
			ID: fmt.Sprintf("mock-%d", i), Provider: []string{"newsapi", "currents", "finnhub"}[i%3],
			Title: r.title, Summary: r.sum, URL: "https://example.com/news/" + strconv.Itoa(i),
			Published: pub, Symbols: []string{r.sym},
		}
		out = append(out, n)
		if i%3 == 0 {
			n2 := n
			n2.ID += "-b"
			n2.Provider = "currents"
			n2.Title = r.title + " — follow-up"
			out = append(out, n2)
		}
	}
	return out
}

func mapSymbols(text string) []string {
	t := strings.ToLower(text)
	var hits []string
	for _, e := range universe.Equities() {
		for _, kw := range universe.Keywords(e.Symbol) {
			if kw != "" && strings.Contains(t, strings.ToLower(kw)) {
				hits = append(hits, e.Symbol)
				break
			}
		}
	}
	return unique(hits)
}

func unique(xs []string) []string {
	m := map[string]struct{}{}
	var out []string
	for _, x := range xs {
		if _, ok := m[x]; ok {
			continue
		}
		m[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

func clusterNews(items []news) map[string][]news {
	out := map[string][]news{}
	for _, n := range items {
		if len(n.Symbols) == 0 {
			n.Symbols = mapSymbols(n.Title + " " + n.Summary)
		}
		key := strings.Join(n.Symbols, ",")
		if key == "" {
			key = normTitle(n.Title)
		}
		out[key] = append(out[key], n)
	}
	return out
}

func normTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r == ' ' {
			return r
		}
		return -1
	}, s)
	words := strings.Fields(s)
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Join(words, " ")
}

func materiality(ns []news) float64 {
	if len(ns) == 0 {
		return 0
	}
	score := 0.4
	if len(ns) > 1 {
		score += 0.2
	}
	txt := strings.ToLower(ns[0].Title + " " + ns[0].Summary)
	for _, kw := range []string{"sebi", "earnings", "guidance", "merger", "promoter", "rbi", "block deal", "order inflow"} {
		if strings.Contains(txt, kw) {
			score += 0.15
		}
	}
	if time.Since(ns[0].Published) < 6*time.Hour {
		score += 0.15
	}
	if score > 1 {
		score = 1
	}
	return score
}

func investigate(ns []news, mode string) *commonv1.InvestigationReport {
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
	standAside := event == "rumor" && mathAbs(score) < 0.35
	tilts := mapTilts(event, score, standAside)
	thesis := fmt.Sprintf("%s tape on %s (%s). Stance %s with corroboration %d.",
		event, strings.Join(symbols, ", "), horizon, stance, len(providers))
	risks := "Headline models misread sarcasm and rumours; do not treat as guaranteed alpha vs Nifty."
	if os.Getenv("OPENAI_API_KEY") != "" {
		if t := llmThesis(ns); t != "" {
			thesis = t
		}
	}
	id := fmt.Sprintf("inv-%d", time.Now().UnixNano())
	status := "completed"
	return &commonv1.InvestigationReport{
		Id: id, Headline: ns[0].Title, Symbols: symbols, EventType: event, Stance: stance,
		Score: score, Confidence: conf, Corroboration: int32(len(providers)), Horizon: horizon,
		StrategyImplications: tilts, StandAside: standAside, Sources: sources,
		AnalyzedAtUnixMs: time.Now().UnixMilli(), Mode: mode, Thesis: thesis, Risks: risks, Status: status,
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
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

func llmThesis(ns []news) string {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return ""
	}
	body := map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{
			"role":    "system",
			"content": "You summarise Indian equity news for a paper-trading desk in two sentences. Not financial advice.",
		}, {
			"role":    "user",
			"content": ns[0].Title + " — " + ns[0].Summary,
		}},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(string(b)))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &parsed) != nil || len(parsed.Choices) == 0 {
		return ""
	}
	return parsed.Choices[0].Message.Content
}

func fetchNewsAPI() []news {
	key := os.Getenv("NEWSAPI_KEY")
	if key == "" {
		return nil
	}
	q := url.QueryEscape("Nifty OR Reliance OR Infosys OR HDFC OR TCS")
	u := "https://newsapi.org/v2/everything?q=" + q + "&language=en&pageSize=20&sortBy=publishedAt&apiKey=" + url.QueryEscape(key)
	return httpNews("newsapi", u, func(raw []byte) []news {
		var p struct {
			Articles []struct {
				Title, Description, URL string
				PublishedAt             string
				Source                  struct{ Name string }
			}
		}
		if json.Unmarshal(raw, &p) != nil {
			return nil
		}
		var out []news
		for i, a := range p.Articles {
			t, _ := time.Parse(time.RFC3339, a.PublishedAt)
			out = append(out, news{
				ID: "newsapi-" + strconv.Itoa(i), Provider: "newsapi", Title: a.Title, Summary: a.Description,
				URL: a.URL, Published: t, Symbols: mapSymbols(a.Title + " " + a.Description),
			})
		}
		return out
	})
}

func fetchCurrents() []news {
	key := os.Getenv("CURRENTS_API_KEY")
	if key == "" {
		return nil
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.currentsapi.services/v1/search?keywords=Nifty%20India%20stock&language=en", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var p struct {
		News []struct {
			ID, Title, Description, URL, Published string
		}
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	var out []news
	for _, a := range p.News {
		t, err := time.Parse(time.RFC3339, a.Published)
		if err != nil {
			t = time.Now()
		}
		out = append(out, news{
			ID: "currents-" + a.ID, Provider: "currents", Title: a.Title, Summary: a.Description,
			URL: a.URL, Published: t, Symbols: mapSymbols(a.Title + " " + a.Description),
		})
	}
	return out
}

func fetchFinnhub() []news {
	key := os.Getenv("FINNHUB_API_KEY")
	if key == "" {
		return nil
	}
	u := "https://finnhub.io/api/v1/news?category=general&token=" + url.QueryEscape(key)
	return httpNews("finnhub", u, func(raw []byte) []news {
		var arr []struct {
			ID          int
			Headline    string
			Summary     string
			URL         string
			Datetime    int64
			Source      string
		}
		if json.Unmarshal(raw, &arr) != nil {
			return nil
		}
		var out []news
		for i, a := range arr {
			if i > 20 {
				break
			}
			out = append(out, news{
				ID: fmt.Sprintf("finnhub-%d", a.ID), Provider: "finnhub", Title: a.Headline, Summary: a.Summary,
				URL: a.URL, Published: time.Unix(a.Datetime, 0), Symbols: mapSymbols(a.Headline + " " + a.Summary),
			})
		}
		return out
	})
}

func httpNews(provider, rawURL string, parse func([]byte) []news) []news {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		log.Printf("%s fetch: %v", provider, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return parse(b)
}

func main() {
	addr := os.Getenv("SENTIMENT_BIND")
	if addr == "" {
		addr = ":9084"
	}
	s := &server{scores: map[string]*commonv1.SentimentScore{}, maxWorkers: getenvInt("MAX_INVESTIGATION_WORKERS", 16)}
	go s.refresh()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer()
	sentimentv1.RegisterSentimentServiceServer(gs, s)
	log.Printf("sentiment listening on %s", addr)
	log.Fatal(gs.Serve(lis))
}
