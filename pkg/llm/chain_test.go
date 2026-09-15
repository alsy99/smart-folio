package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stub struct {
	on    bool
	reply string
	err   error
	calls int
}

func (s *stub) Enabled() bool { return s.on }
func (s *stub) Complete(context.Context, string, string) (string, error) {
	s.calls++
	return s.reply, s.err
}

func TestChainFallsThroughOnError(t *testing.T) {
	a := &stub{on: true, err: errors.New("429")}
	b := &stub{on: true, reply: "from-b"}
	c := &Chain{
		providers: []named{{name: "a", Completer: a}, {name: "b", Completer: b}},
		Dir:       t.TempDir(),
		Now:       func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) },
	}
	got, err := c.Complete(context.Background(), "sys", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-b" {
		t.Fatalf("got %q", got)
	}
	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("calls a=%d b=%d", a.calls, b.calls)
	}
}

func TestPinnedProviderDoesNotSilentlyFallBack(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	a := &stub{on: true, reply: "from-a"}
	b := &stub{on: true, reply: "from-b"}
	c := &Chain{
		providers: []named{{name: "a", Completer: a}, {name: "b", Completer: b}},
		Dir:       dir,
		Now:       func() time.Time { return now },
	}
	if _, err := c.Complete(context.Background(), "sys", "hi"); err != nil {
		t.Fatal(err)
	}
	a.err = errors.New("down")
	a.reply = ""
	_, err := c.Complete(context.Background(), "sys", "hi")
	if err == nil {
		t.Fatal("pinned provider must fail closed")
	}
	if !strings.Contains(err.Error(), "no mid-campaign fallback") {
		t.Fatalf("err %v", err)
	}
	if b.calls != 0 {
		t.Fatalf("silent fallback to b: %d", b.calls)
	}
}

func TestDayTokenCapIsConfigBounded(t *testing.T) {
	t.Setenv("INVESTIGATION_LLM", "true")
	t.Setenv("INVESTIGATION_LLM_MAX", "10")
	t.Setenv("INVESTIGATION_TOKENS_DAY", "50")
	dir := t.TempDir()
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	st := LoadDay(dir, ISTDay(now))
	if st.MaxTokens != 50 {
		t.Fatalf("max tokens %d", st.MaxTokens)
	}
	if !Allow(st) {
		t.Fatal("fresh day should allow")
	}
	st = Charge(dir, now, 40)
	if !Allow(st) {
		t.Fatal("40 of 50 still under cap")
	}
	st = Charge(dir, now, 20)
	if Allow(st) {
		t.Fatalf("token bill must stop at the day cap, state=%+v", st)
	}
}
