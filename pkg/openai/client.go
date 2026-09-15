package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"aperture/pkg/httpx"
)

var ErrDisabled = errors.New("openai: no api key")

type Client struct {
	APIKey string
	Model  string
	HTTP   *http.Client
}

func FromEnv() *Client {
	return &Client{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o-mini",
		HTTP:   httpx.Client(12 * time.Second),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.APIKey != ""
}

func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	model := c.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	raw, err := httpx.Do(c.HTTP, req)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("openai: empty completion")
	}
	return parsed.Choices[0].Message.Content, nil
}
