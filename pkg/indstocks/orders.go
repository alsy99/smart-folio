package indstocks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"aperture/pkg/broker"
	"aperture/pkg/live"
)

// Order is an INDstocks cash/F&O ticket. The default build never posts it.
type Order struct {
	TxnType    string  `json:"txn_type"`
	Exchange   string  `json:"exchange"`
	Segment    string  `json:"segment"`
	SecurityID string  `json:"security_id"`
	Qty        int     `json:"qty"`
	OrderType  string  `json:"order_type"`
	LimitPrice float64 `json:"limit_price,omitempty"`
	Validity   string  `json:"validity"`
	Product    string  `json:"product"`
	IsAMO      bool    `json:"is_amo"`
	AlgoID     string  `json:"algo_id"`
}

func prepareOrder(o Order) Order {
	if o.AlgoID == "" {
		o.AlgoID = "99999"
	}
	if o.Exchange == "" {
		o.Exchange = "NSE"
	}
	if o.Segment == "" {
		o.Segment = "EQUITY"
	}
	if o.Validity == "" {
		o.Validity = "DAY"
	}
	if o.Product == "" {
		o.Product = "CNC"
	}
	return o
}

// PlaceGuarded is the live-adapter path: Check, then PlaceOrder.
// In the default binary PlaceOrder is compile-time off.
func (c *Client) PlaceGuarded(ctx context.Context, snap broker.Snapshot, in broker.Intent, o Order) (json.RawMessage, error) {
	var out json.RawMessage
	err := broker.Submit(ctx, snap, in, func(ctx context.Context, _ broker.Intent) error {
		b, err := c.PlaceOrder(ctx, o)
		out = b
		return err
	})
	return out, err
}

func (c *Client) postJSON(ctx context.Context, path string, body any) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	b, err := c.do(req)
	if err != nil {
		return nil, err
	}
	env, data, err := decodeEnvelope(b)
	if err != nil {
		return nil, err
	}
	if !env.ok() {
		return nil, fmt.Errorf("indstocks: %s", env.err())
	}
	return data, nil
}

func refuseLive(reason error) (json.RawMessage, error) {
	if live.Killed() {
		return nil, live.ErrKilled
	}
	if !live.Ready() {
		return nil, reason
	}
	return nil, reason
}
