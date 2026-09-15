package indstocks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aperture/pkg/broker"
	"aperture/pkg/live"
)

func TestPlaceOrderIsCompileOff(t *testing.T) {
	t.Setenv("AUTOPILOT_LIVE_IND", "true")
	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "order") {
			hit++
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), baseURL: srv.URL, token: "t"}
	_, err := c.PlaceOrder(context.Background(), Order{TxnType: "BUY", Qty: 1, SecurityID: "2885"})
	if !errors.Is(err, live.ErrCompileOff) && !errors.Is(err, live.ErrChecklist) && !errors.Is(err, live.ErrKilled) {
		t.Fatalf("want compile-off, got %v", err)
	}
	if hit != 0 {
		t.Fatalf("must not POST /order, hits=%d", hit)
	}
}

func TestPlaceGuardedStillNeedsCheckAndStaysOff(t *testing.T) {
	t.Setenv("AUTOPILOT_LIVE_IND", "true")
	c := &Client{http: http.DefaultClient, baseURL: "http://127.0.0.1:1"}
	snap := broker.Snapshot{Equity: 1e6, Peak: 1e6, Cash: 1e6, Held: map[string]float64{}, Sector: map[string]float64{}}
	_, err := c.PlaceGuarded(context.Background(), snap, broker.Intent{Symbol: "TCS", Side: broker.Buy, Qty: 1, Price: 100}, Order{})
	if err == nil {
		t.Fatal("live path must not succeed")
	}
}

func TestKillSwitchBlocksEvenIfSomeoneCompilesLive(t *testing.T) {
	p := t.TempDir() + "/KILL"
	t.Setenv("KILL_FILE", p)
	if err := live.Trip(); err != nil {
		t.Fatal(err)
	}
	c := &Client{}
	_, err := c.PlaceOrder(context.Background(), Order{})
	if !errors.Is(err, live.ErrKilled) && !errors.Is(err, live.ErrCompileOff) {
		t.Fatalf("kill or compile-off, got %v", err)
	}
}
