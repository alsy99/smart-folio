package advisor

import (
	"context"
	"strings"
	"testing"

	advisorv1 "aperture/gen/advisor/v1"
)

type disabledLLM struct{}

func (disabledLLM) Enabled() bool { return false }
func (disabledLLM) Complete(context.Context, string, string) (string, error) {
	return "", nil
}

func TestHeuristicFallback(t *testing.T) {
	svc := New(disabledLLM{})
	resp, err := svc.Chat(context.Background(), &advisorv1.ChatRequest{Message: "how do we beat nifty?"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Mode != "mock" {
		t.Fatalf("mode %s", resp.Mode)
	}
	if !strings.Contains(strings.ToLower(resp.Reply), "excess") {
		t.Fatalf("reply %q", resp.Reply)
	}
}
