package research

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/llm"
)

// Receipt is the COGS record persisted next to a fill.
type Receipt struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	FillID           string `json:"fillId,omitempty"`
	Symbol           string `json:"symbol"`
	Day              string `json:"day"`
	HeadlineHash     string `json:"headlineHash"`
	Headline         string `json:"headline"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	Prompt           string `json:"prompt"`
	System           string `json:"system"`
	Output           string `json:"output,omitempty"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
	Mode             string `json:"mode"`
	Thesis           string `json:"thesis,omitempty"`
	Cached           bool   `json:"cached"`
	AtUnixMs         int64  `json:"atUnixMs"`
}

type Desk struct {
	Dir  string
	Now  func() time.Time
	mu   sync.Mutex
	memo map[string]Receipt
}

func NewDesk(dir string) *Desk {
	if dir == "" {
		dir = llm.Dir()
	}
	return &Desk{Dir: dir, Now: time.Now, memo: map[string]Receipt{}}
}

func (d *Desk) now() time.Time {
	if d != nil && d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Desk) dir() string {
	if d != nil && d.Dir != "" {
		return d.Dir
	}
	return llm.Dir()
}

func (d *Desk) AllowLLM() bool {
	st := llm.LoadDay(d.dir(), llm.ISTDay(d.now()))
	return llm.Allow(st)
}

func (d *Desk) Lookup(key string) (Receipt, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if rec, ok := d.memo[key]; ok {
		rec.Cached = true
		return rec, true
	}
	rec, err := LoadReceipt(d.dir(), IDForKey(key))
	if err != nil {
		return Receipt{}, false
	}
	d.memo[key] = rec
	rec.Cached = true
	return rec, true
}

func (d *Desk) Store(rec Receipt) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if rec.Key != "" {
		d.memo[rec.Key] = rec
	}
	return SaveReceipt(d.dir(), rec)
}

// Run is the only investigation LLM path. Cache hits and a spent daily cap
// return the heuristic note without a new completion.
func (d *Desk) Run(ctx context.Context, completer llm.Completer, in Input, wantLLM bool) (Note, Receipt) {
	now := in.Now
	if now.IsZero() {
		now = d.now()
	}
	in.Now = now
	day := llm.ISTDay(now)
	key := CacheKey(in.Symbol, day, in.Headline)
	id := IDForKey(key)
	if rec, ok := d.Lookup(key); ok {
		n := Heuristic(in)
		if rec.Thesis != "" {
			n.Conclusion = rec.Thesis
			n.Mode = rec.Mode
			if rec.Mode == "" {
				n.Mode = "cache"
			}
		}
		rec.Cached = true
		return n, rec
	}

	base := Heuristic(in)
	sys, user := prompts(in)
	rec := Receipt{
		ID: id, Key: key, Symbol: in.Symbol, Day: day,
		HeadlineHash: HeadlineHash(in.Headline), Headline: in.Headline,
		Prompt: user, System: sys, Mode: base.Mode,
		Thesis: base.Conclusion, AtUnixMs: nowMs(now),
	}

	useLLM := wantLLM && d.AllowLLM() && completer != nil && completer.Enabled() && config.BoolDefault("INVESTIGATION_LLM", true)
	if !useLLM {
		_ = d.Store(rec)
		return base, rec
	}

	raw, result, err := complete(ctx, completer, sys, user)
	if err != nil || raw == "" {
		base.Risks = base.Risks + " LLM: " + errString(err)
		rec.Output = errString(err)
		_ = d.Store(rec)
		return base, rec
	}
	n := parse(StripOrderJSON(raw), base)
	n.Mode = "llm"
	n.AtUnixMs = nowMs(now)
	tok := result.Tokens()
	if tok == 0 {
		tok = llm.EstimateTokens(sys, user, raw)
	}
	llm.Charge(d.dir(), now, tok)
	rec.Provider = result.Provider
	rec.Model = result.Model
	rec.Output = raw
	rec.PromptTokens = result.PromptTokens
	rec.CompletionTokens = result.CompletionTokens
	rec.TotalTokens = tok
	rec.Mode = "llm"
	rec.Thesis = n.Conclusion
	_ = d.Store(rec)
	slog.Info("investigation llm", "id", rec.ID, "model", rec.Model, "tokens", tok, "key", key)
	return n, rec
}

func complete(ctx context.Context, c llm.Completer, sys, user string) (string, llm.Result, error) {
	if rc, ok := c.(interface {
		CompleteR(context.Context, string, string) (llm.Result, error)
	}); ok {
		r, err := rc.CompleteR(ctx, sys, user)
		return r.Text, r, err
	}
	text, err := c.Complete(ctx, sys, user)
	r := llm.Result{Text: text, TotalTokens: llm.EstimateTokens(sys, user, text)}
	return text, r, err
}

func prompts(in Input) (string, string) {
	tech, card := Snapshot(in)
	user := "SYMBOL " + in.Symbol + " (" + card.Sector + ")\nFUNDAMENTAL CARD: " + card.Summary() +
		"\nTECHNICALS: " + tech.Summary() +
		"\nSIGNAL strategy=" + in.StrategyID +
		"\nNEWS: " + clip(in.News, 1200) +
		"\nHEADLINE: " + in.Headline
	return systemPrompt, user
}

func SaveReceipt(dir string, rec Receipt) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, rec.ID+".json"), b, 0o644)
}

func LoadReceipt(dir, id string) (Receipt, error) {
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return Receipt{}, err
	}
	var rec Receipt
	if err := json.Unmarshal(b, &rec); err != nil {
		return Receipt{}, err
	}
	return rec, nil
}

// LinkFill copies the investigation receipt next to the fill row.
func LinkFill(dir, fillID, invID string) error {
	rec, err := LoadReceipt(dir, invID)
	if err != nil {
		rec = Receipt{ID: invID, Mode: "missing"}
	}
	rec.FillID = fillID
	if err := os.MkdirAll(filepath.Join(dir, "fills"), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fills", fillID+".json"), b, 0o644)
}
