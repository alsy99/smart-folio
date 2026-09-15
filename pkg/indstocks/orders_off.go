//go:build !liveorders

package indstocks

import (
	"context"
	"encoding/json"

	"aperture/pkg/live"
)

// PlaceOrder is a no-op in the default binary. AUTOPILOT_LIVE_IND cannot arm it.
func (c *Client) PlaceOrder(_ context.Context, _ Order) (json.RawMessage, error) {
	_, err := refuseLive(live.ErrCompileOff)
	return nil, err
}
