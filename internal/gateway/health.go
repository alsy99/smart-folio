package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	advisorv1 "aperture/gen/advisor/v1"
	learningv1 "aperture/gen/learning/v1"
	marketdatav1 "aperture/gen/marketdata/v1"
	sentimentv1 "aperture/gen/sentiment/v1"
	tradingv1 "aperture/gen/trading/v1"
	"aperture/pkg/indstocks"
)

type svcStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func ping(ctx context.Context, timeout time.Duration, fn func(context.Context) error) svcStatus {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := fn(c); err != nil {
		return svcStatus{OK: false, Error: err.Error()}
	}
	return svcStatus{OK: true}
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	const d = 2 * time.Second
	services := map[string]svcStatus{
		"marketdata": ping(ctx, d, func(c context.Context) error {
			if a.md == nil {
				return errNoClient
			}
			_, err := a.md.ListUniverse(c, &marketdatav1.ListUniverseRequest{})
			return err
		}),
		"trading": ping(ctx, d, func(c context.Context) error {
			if a.tr == nil {
				return errNoClient
			}
			_, err := a.tr.GetCampaign(c, &tradingv1.GetCampaignRequest{})
			return err
		}),
		"learning": ping(ctx, d, func(c context.Context) error {
			if a.ln == nil {
				return errNoClient
			}
			_, err := a.ln.ListJournal(c, &learningv1.ListJournalRequest{Limit: 1})
			return err
		}),
		"sentiment": ping(ctx, d, func(c context.Context) error {
			if a.sn == nil {
				return errNoClient
			}
			_, err := a.sn.ListNews(c, &sentimentv1.ListNewsRequest{Limit: 1})
			return err
		}),
		"advisor": ping(ctx, d, func(c context.Context) error {
			if a.ad == nil {
				return errNoClient
			}
			_, err := a.ad.Chat(c, &advisorv1.ChatRequest{Message: "__health__"})
			return err
		}),
	}
	ok := true
	for _, st := range services {
		if !st.OK {
			ok = false
			break
		}
	}
	st := indstocks.Snapshot(ctx)
	tape := "mock"
	if st.Mode == "live" && st.Configured {
		tape = "live"
	}
	status := "ok"
	if !ok {
		status = "degraded"
	}
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    status,
		"tape":      tape,
		"services":  services,
		"indstocks": st,
	})
}

type missingClient struct{}

func (missingClient) Error() string { return "client not configured" }

var errNoClient error = missingClient{}
