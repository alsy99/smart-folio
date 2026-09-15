package llm

import (
	"encoding/json"
	"unicode/utf8"
)

// Result is one completion with the accounting the investigation desk persists.
type Result struct {
	Text             string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (r Result) Tokens() int {
	if r.TotalTokens > 0 {
		return r.TotalTokens
	}
	return r.PromptTokens + r.CompletionTokens
}

// EstimateTokens is a rough bill when the provider omits usage.
func EstimateTokens(system, user, output string) int {
	n := utf8.RuneCountInString(system) + utf8.RuneCountInString(user) + utf8.RuneCountInString(output)
	if n <= 0 {
		return 1
	}
	t := n / 4
	if t < 1 {
		t = 1
	}
	return t
}

type usageJSON struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func ParseUsage(raw []byte) (prompt, completion, total int) {
	var u usageJSON
	if err := json.Unmarshal(raw, &u); err != nil {
		return 0, 0, 0
	}
	prompt = u.Usage.PromptTokens
	completion = u.Usage.CompletionTokens
	total = u.Usage.TotalTokens
	if total == 0 && u.UsageMetadata.TotalTokenCount > 0 {
		prompt = u.UsageMetadata.PromptTokenCount
		completion = u.UsageMetadata.CandidatesTokenCount
		total = u.UsageMetadata.TotalTokenCount
	}
	if total == 0 {
		total = prompt + completion
	}
	return
}

type Modeler interface {
	ModelName() string
}
