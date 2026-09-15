package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/httpx"
)

var ErrDisabled = errors.New("gemini: no api key")

type Client struct {
	APIKey string
	Model  string
	HTTP   *http.Client
}

func FromEnv() *Client {
	key := os.Getenv("GOOGLE_API_KEY")
	if key == "" {
		key = os.Getenv("GEMINI_API_KEY")
	}
	model := config.String("GEMINI_MODEL", "gemini-flash-latest")
	return &Client{
		APIKey: key,
		Model:  model,
		HTTP:   httpx.Client(20 * time.Second),
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
		model = "gemini-flash-latest"
	}
	payload, err := json.Marshal(map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": system}},
		},
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]string{{"text": user}}},
		},
	})
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)
	raw, err := httpx.Do(c.HTTP, req)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error.Message != "" {
		return "", fmt.Errorf("gemini: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini: empty completion")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}
