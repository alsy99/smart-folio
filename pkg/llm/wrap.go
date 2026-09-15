package llm

import (
	"context"

	"aperture/pkg/gemini"
	"aperture/pkg/openai"
)

type usageClient interface {
	Enabled() bool
	Complete(ctx context.Context, system, user string) (string, error)
	CompleteWithUsage(ctx context.Context, system, user string) (text, model string, prompt, completion, total int, err error)
}

type billed struct {
	name string
	usageClient
}

func (b billed) CompleteR(ctx context.Context, system, user string) (Result, error) {
	text, model, p, co, tot, err := b.CompleteWithUsage(ctx, system, user)
	return Result{
		Text: text, Provider: b.name, Model: model,
		PromptTokens: p, CompletionTokens: co, TotalTokens: tot,
	}, err
}

func (b billed) ModelName() string {
	if o, ok := b.usageClient.(*openai.Client); ok {
		if o.Model != "" {
			return o.Model
		}
		return "gpt-4o-mini"
	}
	if g, ok := b.usageClient.(*gemini.Client); ok {
		return g.Model
	}
	return ""
}

func wrapOpenAI(c *openai.Client) Completer {
	if c == nil {
		return c
	}
	return billed{name: "openai", usageClient: c}
}

func wrapGemini(c *gemini.Client) Completer {
	if c == nil {
		return c
	}
	return billed{name: "gemini", usageClient: c}
}
