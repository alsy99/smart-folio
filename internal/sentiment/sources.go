package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"aperture/pkg/asof"
	"aperture/pkg/httpx"
)

// Source is the Adapter pattern: each provider speaks a common Fetch contract.
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]article, error)
}

func DefaultSources() []Source {
	client := httpx.Client(8 * time.Second)
	return []Source{
		newsAPI{client: client, key: os.Getenv("NEWSAPI_KEY")},
		currents{client: client, key: os.Getenv("CURRENTS_API_KEY")},
		finnhub{client: client, key: os.Getenv("FINNHUB_API_KEY")},
	}
}

func gather(ctx context.Context, sources []Source, now time.Time) ([]article, string) {
	var mu sync.Mutex
	var all []article
	mode := "mock"
	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(src Source) {
			defer wg.Done()
			got, err := src.Fetch(ctx)
			if err != nil {
				slog.Warn("news ingest", "source", src.Name(), "err", err)
				return
			}
			slog.Info("news ingest", "source", src.Name(), "articles", len(got))
			if len(got) == 0 {
				return
			}
			mu.Lock()
			all = append(all, got...)
			mode = "live"
			mu.Unlock()
		}(src)
	}
	wg.Wait()
	var known []article
	for _, a := range all {
		if asof.KnownAt(a.Published, now) {
			known = append(known, a)
		}
	}
	if len(known) == 0 {
		return mockNews(now), "mock"
	}
	return known, mode
}

type newsAPI struct {
	client *http.Client
	key    string
}

func (n newsAPI) Name() string { return "newsapi" }

func (n newsAPI) Fetch(ctx context.Context) ([]article, error) {
	if n.key == "" {
		return nil, nil
	}
	q := url.QueryEscape("Nifty OR Reliance OR Infosys OR HDFC OR TCS OR Sensex")
	u := "https://newsapi.org/v2/everything?q=" + q + "&language=en&pageSize=20&sortBy=publishedAt&apiKey=" + url.QueryEscape(n.key)
	raw, err := httpx.Get(ctx, n.client, u)
	if err != nil {
		return nil, err
	}
	var p struct {
		Status   string `json:"status"`
		Code     string `json:"code"`
		Message  string `json:"message"`
		Articles []struct {
			Title, Description, URL string
			PublishedAt             string
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Status == "error" {
		return nil, fmt.Errorf("newsapi: %s (%s)", p.Message, p.Code)
	}
	out := make([]article, 0, len(p.Articles))
	for i, a := range p.Articles {
		t, _ := time.Parse(time.RFC3339, a.PublishedAt)
		out = append(out, article{
			ID: "newsapi-" + strconv.Itoa(i), Provider: "newsapi", Title: a.Title, Summary: a.Description,
			URL: a.URL, Published: t, Symbols: mapSymbols(a.Title + " " + a.Description),
		})
	}
	return out, nil
}

type currents struct {
	client *http.Client
	key    string
}

func (c currents) Name() string { return "currents" }

func (c currents) Fetch(ctx context.Context) ([]article, error) {
	if c.key == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.currentsapi.services/v1/search?keywords=Nifty%20India%20stock&language=en", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	raw, err := httpx.Do(c.client, req)
	if err != nil {
		return nil, err
	}
	var p struct {
		News []struct {
			ID, Title, Description, URL, Published string
		}
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	out := make([]article, 0, len(p.News))
	for _, a := range p.News {
		t, err := time.Parse(time.RFC3339, a.Published)
		if err != nil {
			t = time.Now()
		}
		out = append(out, article{
			ID: "currents-" + a.ID, Provider: "currents", Title: a.Title, Summary: a.Description,
			URL: a.URL, Published: t, Symbols: mapSymbols(a.Title + " " + a.Description),
		})
	}
	return out, nil
}

type finnhub struct {
	client *http.Client
	key    string
}

func (f finnhub) Name() string { return "finnhub" }

func (f finnhub) Fetch(ctx context.Context) ([]article, error) {
	if f.key == "" {
		return nil, nil
	}
	u := "https://finnhub.io/api/v1/news?category=general&token=" + url.QueryEscape(f.key)
	raw, err := httpx.Get(ctx, f.client, u)
	if err != nil {
		return nil, err
	}
	var arr []struct {
		ID       int
		Headline string
		Summary  string
		URL      string
		Datetime int64
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	out := make([]article, 0, 21)
	for i, a := range arr {
		if i > 20 {
			break
		}
		out = append(out, article{
			ID: fmt.Sprintf("finnhub-%d", a.ID), Provider: "finnhub", Title: a.Headline, Summary: a.Summary,
			URL: a.URL, Published: time.Unix(a.Datetime, 0), Symbols: mapSymbols(a.Headline + " " + a.Summary),
		})
	}
	return out, nil
}
