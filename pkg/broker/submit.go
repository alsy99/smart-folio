package broker

import (
	"context"
	"fmt"
)

// Router sends a cleared intent to an exchange adapter. Paper does not use it.
type Router func(ctx context.Context, in Intent) error

// Submit is the live-adapter entry: Check, then route. A future INDstocks
// adapter must call this — never PlaceOrder on an unchecked ticket.
func Submit(ctx context.Context, snap Snapshot, in Intent, route Router) error {
	d := Check(in, snap)
	if !d.Allow {
		return fmt.Errorf("broker: %s", d.Reason)
	}
	if route == nil {
		return fmt.Errorf("broker: no router")
	}
	return route(ctx, in)
}
