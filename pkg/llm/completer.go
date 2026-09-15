package llm

import "context"

type Completer interface {
	Enabled() bool
	Complete(ctx context.Context, system, user string) (string, error)
}
