package llm

import (
	"context"
	"errors"
	"testing"
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
	c := &Chain{providers: []named{
		{name: "a", Completer: a},
		{name: "b", Completer: b},
	}}
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
