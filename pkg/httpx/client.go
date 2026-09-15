package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func Client(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func Get(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return Do(client, req)
}

func Do(client *http.Client, req *http.Request) ([]byte, error) {
	if client == nil {
		client = Client(0)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 240 {
			msg = msg[:240]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: http %d: %s", req.URL.Host, resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("%s: http %d", req.URL.Host, resp.StatusCode)
	}
	return b, nil
}
