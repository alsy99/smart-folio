package llm

import (
	"context"
	"errors"
	"log/slog"
)

type named struct {
	name string
	Completer
}

// Chain tries each enabled provider in order. Rate limits and other errors fall through.
type Chain struct {
	providers []named
}

func (c *Chain) Enabled() bool {
	if c == nil {
		return false
	}
	for _, p := range c.providers {
		if p.Enabled() {
			return true
		}
	}
	return false
}

func (c *Chain) Complete(ctx context.Context, system, user string) (string, error) {
	if c == nil {
		return "", errors.New("llm: no providers")
	}
	var last error
	for _, p := range c.providers {
		if !p.Enabled() {
			continue
		}
		reply, err := p.Complete(ctx, system, user)
		if err == nil && reply != "" {
			slog.Info("llm used", "provider", p.name)
			return reply, nil
		}
		last = err
		if err != nil {
			slog.Warn("llm fallback", "provider", p.name, "err", err)
		}
	}
	if last == nil {
		last = errors.New("llm: no providers enabled")
	}
	return "", last
}
