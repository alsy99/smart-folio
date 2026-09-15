package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthNamesEachService(t *testing.T) {
	t.Setenv("INDSTOCKS_ACCESS_TOKEN", "")
	t.Setenv("INDMONEY_API_TOKEN", "")
	t.Setenv("INDMONEY_ACCESS_TOKEN", "")
	api := New(nil, Clients{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil clients should be degraded, got %d", rec.Code)
	}
	var body struct {
		Status   string               `json:"status"`
		Tape     string               `json:"tape"`
		Services map[string]svcStatus `json:"services"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Tape != "mock" {
		t.Fatalf("missing token must be mock tape, got %s", body.Tape)
	}
	for _, name := range []string{"marketdata", "trading", "learning", "sentiment", "advisor"} {
		st, ok := body.Services[name]
		if !ok {
			t.Fatalf("health missing %s", name)
		}
		if st.OK {
			t.Fatalf("%s should be down without a client", name)
		}
	}
}
