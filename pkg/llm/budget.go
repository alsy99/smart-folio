package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/marketclock"
)

const (
	DefaultMaxCalls  = 5
	DefaultMaxTokens = 50_000
)

// DayState is the IST-day ledger: call cap, token bill, and the pinned model.
type DayState struct {
	Day            string `json:"day"`
	Calls          int    `json:"calls"`
	Tokens         int    `json:"tokens"`
	PinnedProvider string `json:"pinnedProvider,omitempty"`
	PinnedModel    string `json:"pinnedModel,omitempty"`
	MaxCalls       int    `json:"maxCalls"`
	MaxTokens      int    `json:"maxTokens"`
}

func ISTDay(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return now.In(marketclock.Location()).Format("2006-01-02")
}

func Dir() string {
	return config.String("INVESTIGATION_DIR", "data/investigations")
}

func MaxCalls() int {
	n := config.Int("INVESTIGATION_LLM_MAX", DefaultMaxCalls)
	if n < 0 {
		return 0
	}
	return n
}

func MaxTokens() int {
	n := config.Int("INVESTIGATION_TOKENS_DAY", DefaultMaxTokens)
	if n < 0 {
		return 0
	}
	return n
}

func statePath(dir, day string) string {
	return filepath.Join(dir, "budget-"+day+".json")
}

var stateMu sync.Mutex

func LoadDay(dir, day string) DayState {
	st := DayState{Day: day, MaxCalls: MaxCalls(), MaxTokens: MaxTokens()}
	b, err := os.ReadFile(statePath(dir, day))
	if err != nil {
		return st
	}
	var loaded DayState
	if json.Unmarshal(b, &loaded) != nil || loaded.Day != day {
		return st
	}
	loaded.MaxCalls = st.MaxCalls
	loaded.MaxTokens = st.MaxTokens
	return loaded
}

func SaveDay(dir string, st DayState) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := statePath(dir, st.Day) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(dir, st.Day))
}

func Allow(st DayState) bool {
	if !config.BoolDefault("INVESTIGATION_LLM", true) {
		return false
	}
	if st.MaxCalls <= 0 || st.Calls >= st.MaxCalls {
		return false
	}
	if st.MaxTokens <= 0 || st.Tokens >= st.MaxTokens {
		return false
	}
	return true
}

func Charge(dir string, now time.Time, tokens int) DayState {
	stateMu.Lock()
	defer stateMu.Unlock()
	day := ISTDay(now)
	st := LoadDay(dir, day)
	st.Calls++
	st.Tokens += tokens
	_ = SaveDay(dir, st)
	return st
}

func Pin(dir string, now time.Time, provider, model string) DayState {
	stateMu.Lock()
	defer stateMu.Unlock()
	day := ISTDay(now)
	st := LoadDay(dir, day)
	st.PinnedProvider = provider
	st.PinnedModel = model
	_ = SaveDay(dir, st)
	return st
}

func WithState(dir, day string, fn func(st *DayState)) DayState {
	stateMu.Lock()
	defer stateMu.Unlock()
	st := LoadDay(dir, day)
	fn(&st)
	_ = SaveDay(dir, st)
	return st
}
