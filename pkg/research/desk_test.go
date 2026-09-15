package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aperture/pkg/llm"
)

type richStub struct {
	calls int
	reply string
	toks  int
}

func (s *richStub) Enabled() bool { return true }
func (s *richStub) Complete(ctx context.Context, system, user string) (string, error) {
	r, err := s.CompleteR(ctx, system, user)
	return r.Text, err
}
func (s *richStub) CompleteR(context.Context, string, string) (llm.Result, error) {
	s.calls++
	return llm.Result{Text: s.reply, Provider: "stub", Model: "stub-model", TotalTokens: s.toks}, nil
}

func TestParseDropsOrderFields(t *testing.T) {
	raw := `{"stance":"bullish","qty":500,"side":"BUY","order":{"symbol":"TCS","qty":500},"conclusion":"lean long vs Nifty","risks":"guidance"}`
	got := parse(raw, Note{Symbol: "TCS"})
	if got.Conclusion != "lean long vs Nifty" {
		t.Fatalf("conclusion %q", got.Conclusion)
	}
	stripped := StripOrderJSON(raw)
	if strings.Contains(stripped, `"qty"`) || strings.Contains(stripped, `"side"`) || strings.Contains(stripped, `"order"`) {
		t.Fatalf("order fields survived: %s", stripped)
	}
}

func TestCacheBySymbolDayHeadline(t *testing.T) {
	t.Setenv("INVESTIGATION_LLM", "true")
	t.Setenv("INVESTIGATION_LLM_MAX", "5")
	t.Setenv("INVESTIGATION_TOKENS_DAY", "50000")
	dir := t.TempDir()
	desk := NewDesk(dir)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	desk.Now = func() time.Time { return now }
	st := &richStub{reply: `{"stance":"bullish","horizon":"position","conclusion":"cached thesis","risks":"x"}`, toks: 10}
	in := Input{Symbol: "TCS", Headline: "TCS wins a large mandate", Now: now}
	if _, rec := desk.Run(context.Background(), st, in, true); rec.ID == "" || st.calls != 1 {
		t.Fatalf("first run calls=%d rec=%+v", st.calls, rec)
	}
	if _, rec := desk.Run(context.Background(), st, in, true); !rec.Cached || st.calls != 1 {
		t.Fatalf("cache miss calls=%d cached=%v", st.calls, rec.Cached)
	}
	in.Headline = "TCS different headline entirely"
	if _, rec := desk.Run(context.Background(), st, in, true); rec.Cached || st.calls != 2 {
		t.Fatalf("new headline should miss cache calls=%d cached=%v", st.calls, rec.Cached)
	}
}

func TestDayTokenBillBoundsLLM(t *testing.T) {
	t.Setenv("INVESTIGATION_LLM", "true")
	t.Setenv("INVESTIGATION_LLM_MAX", "9")
	t.Setenv("INVESTIGATION_TOKENS_DAY", "20")
	dir := t.TempDir()
	desk := NewDesk(dir)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	desk.Now = func() time.Time { return now }
	st := &richStub{reply: `{"stance":"mixed","conclusion":"ok","risks":"x"}`, toks: 20}
	a := Input{Symbol: "TCS", Headline: "one", Now: now}
	b := Input{Symbol: "INFY", Headline: "two", Now: now}
	_, rec1 := desk.Run(context.Background(), st, a, true)
	if rec1.Mode != "llm" {
		t.Fatalf("first should be llm %+v", rec1)
	}
	_, rec2 := desk.Run(context.Background(), st, b, true)
	if rec2.Mode == "llm" {
		t.Fatalf("second must be blocked by the day token cap: %+v", rec2)
	}
	if st.calls != 1 {
		t.Fatalf("calls %d", st.calls)
	}
}

func TestSessionCallCap(t *testing.T) {
	t.Setenv("INVESTIGATION_LLM", "true")
	t.Setenv("INVESTIGATION_LLM_MAX", "1")
	t.Setenv("INVESTIGATION_TOKENS_DAY", "50000")
	dir := t.TempDir()
	desk := NewDesk(dir)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	desk.Now = func() time.Time { return now }
	st := &richStub{reply: `{"stance":"mixed","conclusion":"ok","risks":"x"}`, toks: 5}
	desk.Run(context.Background(), st, Input{Symbol: "TCS", Headline: "a", Now: now}, true)
	_, rec := desk.Run(context.Background(), st, Input{Symbol: "INFY", Headline: "b", Now: now}, true)
	if rec.Mode == "llm" || st.calls != 1 {
		t.Fatalf("INVESTIGATION_LLM_MAX=1 calls=%d mode=%s", st.calls, rec.Mode)
	}
}

func TestLinkFillPersistsPromptModelTokens(t *testing.T) {
	dir := t.TempDir()
	rec := Receipt{
		ID: "inv-abc", Key: "TCS|2026-09-15|deadbeef", Symbol: "TCS",
		Prompt: "hello", System: "sys", Output: "out", Model: "gpt-4o-mini",
		Provider: "openai", TotalTokens: 12, Mode: "llm",
	}
	if err := SaveReceipt(dir, rec); err != nil {
		t.Fatal(err)
	}
	if err := LinkFill(dir, "t-1", rec.ID); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "fills", "t-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"hello", "gpt-4o-mini", `"totalTokens": 12`, "out", "inv-abc"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fill receipt missing %s in %s", want, got)
		}
	}
}
