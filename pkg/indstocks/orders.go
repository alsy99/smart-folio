package indstocks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Order is an INDstocks cash/F&O ticket. Aperture does not send these unless
// a future path explicitly calls PlaceOrder; the paper book never does.
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

func (c *Client) PlaceOrder(ctx context.Context, o Order) (json.RawMessage, error) {
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
	return c.postJSON(ctx, "/order", o)
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
