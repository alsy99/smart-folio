package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"aperture/pkg/httpx"
)

// Compat talks to any OpenAI-compatible chat completions API.
type Compat struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
	Headers map[string]string
	HTTP    *http.Client
}

func (c *Compat) Enabled() bool {
	return c != nil && c.APIKey != "" && c.BaseURL != ""
}

func (c *Compat) Complete(ctx context.Context, system, user string) (string, error) {
	r, err := c.CompleteR(ctx, system, user)
	return r.Text, err
}

func (c *Compat) ModelName() string {
	if c == nil {
		return ""
	}
	return c.Model
}

func (c *Compat) CompleteR(ctx context.Context, system, user string) (Result, error) {
	if !c.Enabled() {
		return Result{}, fmt.Errorf("%s: disabled", c.Name)
	}
	model := c.Model
	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	client := c.HTTP
	if client == nil {
		client = httpx.Client(20 * time.Second)
	}
	raw, err := httpx.Do(client, req)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", c.Name, err)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, fmt.Errorf("%s: decode: %w", c.Name, err)
	}
	if parsed.Error.Message != "" {
		return Result{}, fmt.Errorf("%s: %s", c.Name, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Result{}, errors.New(c.Name + ": empty completion")
	}
	p, co, tot := ParseUsage(raw)
	r := Result{
		Text: parsed.Choices[0].Message.Content, Provider: c.Name, Model: model,
		PromptTokens: p, CompletionTokens: co, TotalTokens: tot,
	}
	if r.TotalTokens == 0 {
		r.TotalTokens = EstimateTokens(system, user, r.Text)
	}
	return r, nil
}
