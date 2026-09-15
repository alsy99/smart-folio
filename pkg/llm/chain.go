package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type named struct {
	name string
	Completer
}

type richCompleter interface {
	CompleteR(context.Context, string, string) (Result, error)
}

// Chain tries each enabled provider in order until one is pinned for the IST day.
// After a pin, a failure does not silently fall through to another model.
type Chain struct {
	providers []named
	Dir       string
	Now       func() time.Time
}

func (c *Chain) dir() string {
	if c != nil && c.Dir != "" {
		return c.Dir
	}
	return Dir()
}

func (c *Chain) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
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
	r, err := c.CompleteR(ctx, system, user)
	return r.Text, err
}

func (c *Chain) CompleteR(ctx context.Context, system, user string) (Result, error) {
	if c == nil {
		return Result{}, errors.New("llm: no providers")
	}
	day := ISTDay(c.now())
	st := LoadDay(c.dir(), day)
	if st.PinnedProvider != "" {
		p := c.find(st.PinnedProvider)
		if p == nil {
			return Result{}, fmt.Errorf("llm: pinned provider %s is not configured (no silent fallback)", st.PinnedProvider)
		}
		r, err := runOne(*p, ctx, system, user)
		if err != nil {
			return r, fmt.Errorf("llm: pinned %s/%s failed: %w (no mid-campaign fallback)", st.PinnedProvider, st.PinnedModel, err)
		}
		if r.Model == "" {
			r.Model = st.PinnedModel
		}
		return r, nil
	}
	var last error
	for _, p := range c.providers {
		if !p.Enabled() {
			continue
		}
		r, err := runOne(p, ctx, system, user)
		if err == nil && r.Text != "" {
			Pin(c.dir(), c.now(), p.name, r.Model)
			slog.Info("llm pinned", "provider", p.name, "model", r.Model, "day", day)
			return r, nil
		}
		last = err
		if err != nil {
			slog.Warn("llm fallback", "provider", p.name, "err", err)
		}
	}
	if last == nil {
		last = errors.New("llm: no providers enabled")
	}
	return Result{}, last
}

func (c *Chain) find(name string) *named {
	for i := range c.providers {
		if c.providers[i].name == name && c.providers[i].Enabled() {
			return &c.providers[i]
		}
	}
	return nil
}

func runOne(p named, ctx context.Context, system, user string) (Result, error) {
	if rc, ok := p.Completer.(richCompleter); ok {
		r, err := rc.CompleteR(ctx, system, user)
		r.Provider = p.name
		if r.Model == "" {
			r.Model = modelOf(p.Completer)
		}
		if r.Tokens() == 0 && r.Text != "" {
			r.TotalTokens = EstimateTokens(system, user, r.Text)
		}
		return r, err
	}
	reply, err := p.Complete(ctx, system, user)
	r := Result{Text: reply, Provider: p.name, Model: modelOf(p.Completer)}
	if err == nil && reply != "" {
		r.TotalTokens = EstimateTokens(system, user, reply)
	}
	return r, err
}

func modelOf(c Completer) string {
	if m, ok := c.(Modeler); ok {
		return m.ModelName()
	}
	return ""
}
