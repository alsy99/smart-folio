//go:build liveorders

package indstocks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aperture/pkg/live"
)

var orderRate live.Limiter

// PlaceOrder posts to INDstocks only in a liveorders build, and only after
// the NSE checklist, kill switch, Algo-ID, and rate cap all pass.
// cmd/ binaries are not built with this tag.
func (c *Client) PlaceOrder(ctx context.Context, o Order) (json.RawMessage, error) {
	if live.Killed() {
		return nil, live.ErrKilled
	}
	if !live.Ready() {
		return nil, live.ErrChecklist
	}
	o = prepareOrder(o)
	if o.AlgoID == "" || o.AlgoID == "99999" {
		return nil, fmt.Errorf("live orders: registered Algo-ID required, not a placeholder")
	}
	if !orderRate.Allow(time.Now()) {
		return nil, fmt.Errorf("live orders: rate %d/s would exceed the exchange-safe cap", live.MaxOrdersPerSec)
	}
	return c.postJSON(ctx, "/order", o)
}
