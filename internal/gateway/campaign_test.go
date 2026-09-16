package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicCampaignsListsFrozenLedgers(t *testing.T) {
	api := New(nil, Clients{})
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/public-campaigns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /public-campaigns %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Campaigns []struct {
			Manifest struct {
				Name string `json:"name"`
				Tape string `json:"tape"`
			} `json:"manifest"`
		} `json:"campaigns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range body.Campaigns {
		if c.Manifest.Name == "public-30d" {
			found = true
			if c.Manifest.Tape == "mock-deterministic" {
				t.Fatal("cash month must stay on the INDstocks tape")
			}
		}
	}
	if !found {
		t.Fatal("GET /public-campaigns must include the historical cash month")
	}
}
