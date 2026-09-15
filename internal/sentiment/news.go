package sentiment

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	commonv1 "aperture/gen/common/v1"
	"aperture/pkg/universe"
)

type article struct {
	ID, Provider, Title, Summary, URL string
	Published                         time.Time
	Symbols                           []string
}

func toNewsItem(n article) *commonv1.NewsItem {
	return &commonv1.NewsItem{
		Id: n.ID, Provider: n.Provider, Title: n.Title, Summary: n.Summary, Url: n.URL,
		PublishedAtUnixMs: n.Published.UnixMilli(), Symbols: n.Symbols,
	}
}

func mockNews(now time.Time) []article {
	if now.IsZero() {
		now = time.Now()
	}
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
	out := make([]article, 0, len(raw)*2)
	for i, r := range raw {
		pub := now.Add(-time.Duration(20+i*7) * time.Minute)
		n := article{
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

func clusterNews(items []article) map[string][]article {
	out := map[string][]article{}
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

func materiality(ns []article) float64 {
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
