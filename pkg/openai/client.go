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
	text, _, _, _, _, err := c.CompleteWithUsage(ctx, system, user)
	return text, err
}

func (c *Client) CompleteWithUsage(ctx context.Context, system, user string) (text, model string, prompt, completion, total int, err error) {
	if !c.Enabled() {
		return "", "", 0, 0, 0, ErrDisabled
	}
	model = c.Model
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
		return "", model, 0, 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", model, 0, 0, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	raw, err := httpx.Do(c.HTTP, req)
	if err != nil {
		return "", model, 0, 0, 0, err
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", model, 0, 0, 0, err
	}
	if len(parsed.Choices) == 0 {
		return "", model, 0, 0, 0, errors.New("openai: empty completion")
	}
	total = parsed.Usage.TotalTokens
	if total == 0 {
		total = parsed.Usage.PromptTokens + parsed.Usage.CompletionTokens
	}
	return parsed.Choices[0].Message.Content, model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, total, nil
}
