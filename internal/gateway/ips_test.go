package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	policyv1 "aperture/gen/policy/v1"
	"aperture/internal/policy"

	"google.golang.org/grpc"
)

// localPolicy adapts the server to the client interface, no socket.
type localPolicy struct{ s *policy.Service }

func (l localPolicy) PutIPS(ctx context.Context, in *policyv1.PutIPSRequest, _ ...grpc.CallOption) (*policyv1.IPS, error) {
	return l.s.PutIPS(ctx, in)
}
func (l localPolicy) GetIPS(ctx context.Context, in *policyv1.GetIPSRequest, _ ...grpc.CallOption) (*policyv1.IPS, error) {
	return l.s.GetIPS(ctx, in)
}
func (l localPolicy) PreviewTargets(ctx context.Context, in *policyv1.PreviewTargetsRequest, _ ...grpc.CallOption) (*policyv1.Targets, error) {
	return l.s.PreviewTargets(ctx, in)
}

const goodIPS = `{"id":"c-1","goal":"beat_nifty","horizonYears":5,"maxDd":0.15,"benchmark":"NIFTY50","corePct":1,"satellitePct":0,"rebalance":"monthly","startCash":1000000,"foldSatellite":true}`

func TestIPSRoutes(t *testing.T) {
	api := New(nil, Clients{Policy: localPolicy{policy.New(policy.Deps{})}})
	h := api.Handler()

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/ips", strings.NewReader(body)))
		return rec
	}
	if rec := put(goodIPS); rec.Code != http.StatusOK {
		t.Fatalf("PUT /ips %d %s", rec.Code, rec.Body)
	}
	// Invalid statements are refused, and free text has nowhere to go.
	for _, body := range []string{
		strings.Replace(goodIPS, `"maxDd":0.15`, `"maxDd":0.20`, 1),
		strings.Replace(goodIPS, `"corePct":1,"satellitePct":0`, `"corePct":0.5,"satellitePct":0.5`, 1),
		strings.Replace(goodIPS, `"benchmark":"NIFTY50"`, `"benchmark":""`, 1),
		strings.Replace(goodIPS, `"goal":"beat_nifty"`, `"goal":"guaranteed 10%"`, 1),
		strings.Replace(goodIPS, `"foldSatellite":true`, `"foldSatellite":true,"note":"make 10% a month"`, 1),
	} {
		if rec := put(body); rec.Code == http.StatusOK {
			t.Fatalf("must refuse %s", body)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ips/c-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ips/c-1 %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Hash string `json:"hash"`
		Line string `json:"line"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got.Hash == "" || !strings.Contains(got.Line, "core 100%") {
		t.Fatalf("GET body %+v", got)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ips", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ips %d %s", rec.Code, rec.Body)
	}
	var bound struct {
		ID   string `json:"id"`
		Line string `json:"line"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&bound)
	if bound.ID != "c-1" || bound.Line != got.Line {
		t.Fatalf("GET /ips must paint the same line as GET /ips/c-1: %+v vs %+v", bound, got)
	}
}
