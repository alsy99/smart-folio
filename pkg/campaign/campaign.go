package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"
)

const (
	Name       = "public-30d"
	Days       = 30
	Tape       = "mock-deterministic"
	CostModel  = "delivery-v1"
	Weights    = "equal"
	DefaultDir = "campaign/public-30d"
	LedgerFile = "ledger.json"
	// EquityTolINR is "within rounding" for a clone-and-replay check.
	EquityTolINR = 1.0
)

// Window is the frozen 30×24h paper campaign (positional cash, mock tape).
func Window() (start, end time.Time) {
	loc := marketclock.Location()
	start = time.Date(2026, 8, 17, 9, 15, 0, 0, loc)
	end = start.Add(time.Duration(Days) * 24 * time.Hour)
	return
}

type Manifest struct {
	Name            string   `json:"name"`
	Frozen          bool     `json:"frozen"`
	GitSHA          string   `json:"gitSha"`
	Dirty           bool     `json:"dirty"`
	Start           string   `json:"start"`
	End             string   `json:"end"`
	StartUnixMs     int64    `json:"startUnixMs"`
	EndUnixMs       int64    `json:"endUnixMs"`
	Days            int      `json:"days"`
	Tape            string   `json:"tape"`
	LLM             bool     `json:"llm"`
	ScalpMode       bool     `json:"scalpMode"`
	Weights         string   `json:"weights"`
	CostModel       string   `json:"costModel"`
	SettingsHash    string   `json:"settingsHash"`
	Universe        []string `json:"universe"`
	Benchmarks      []string `json:"benchmarks"`
	NameCap         float64  `json:"nameCap"`
	GrossCap        float64  `json:"grossCap"`
	CashBuffer      float64  `json:"cashBuffer"`
	SectorCap       float64  `json:"sectorCap"`
	DrawdownHalt    float64  `json:"drawdownHalt"`
	MinHoldSessions int      `json:"minHoldSessions"`
	TurnoverCapDay  float64  `json:"turnoverCapDay"`
	StartCash       float64  `json:"startCash"`
	Reproduce       string   `json:"reproduce"`
}

type Day struct {
	Date              string  `json:"date"`
	Session           string  `json:"session"`
	Equity            float64 `json:"equity"`
	ExcessNifty50Pct  float64 `json:"excessNifty50Pct"`
	ExcessNifty500Pct float64 `json:"excessNifty500Pct"`
	ExcessSensexPct   float64 `json:"excessSensexPct"`
	DrawdownPct       float64 `json:"drawdownPct"`
	TurnoverPct       float64 `json:"turnoverPct"`
	TurnoverINR       float64 `json:"turnoverInr"`
	Fills             int     `json:"fills"`
	Halted            bool    `json:"halted"`
}

type Ledger struct {
	Manifest Manifest `json:"manifest"`
	Days     []Day    `json:"days"`
}

func NewManifest(gitSHA string, dirty bool) Manifest {
	start, end := Window()
	return Manifest{
		Name:            Name,
		Frozen:          true,
		GitSHA:          gitSHA,
		Dirty:           dirty,
		Start:           start.Format(time.RFC3339),
		End:             end.Format(time.RFC3339),
		StartUnixMs:     start.UnixMilli(),
		EndUnixMs:       end.UnixMilli(),
		Days:            Days,
		Tape:            Tape,
		LLM:             false,
		ScalpMode:       false,
		Weights:         Weights,
		CostModel:       CostModel,
		SettingsHash:    SettingsHash(),
		Universe:        universe.EquitySymbols(),
		Benchmarks:      []string{"NIFTY50", "NIFTY500", "SENSEX"},
		NameCap:         costs.NameCap,
		GrossCap:        broker.GrossCap,
		CashBuffer:      broker.CashBuffer,
		SectorCap:       broker.SectorCap,
		DrawdownHalt:    costs.DrawdownHalt,
		MinHoldSessions: costs.MinHoldSessions,
		TurnoverCapDay:  costs.TurnoverCapDay,
		StartCash:       costs.StartCash,
		Reproduce:       "go run ./cmd/campaign -verify",
	}
}

func SettingsHash() string {
	payload := fmt.Sprintf(
		"tape=%s|cost=%s|weights=%s|scalp=false|llm=false|cash=%.0f|name=%.4f|gross=%.4f|cashbuf=%.4f|sector=%.4f|halt=%.4f|min_hold_sessions=%d|turnover_cap_day=%.4f|max_hold_h=%.0f|stt_buy=%.4f|stt_sell=%.4f|exch=%.6f|sebi=%.6f|stamp=%.4f|gst=%.4f|brokerage=%.2f|adv_base=%.2f|adv_kappa=%.0f|universe=%s",
		Tape, CostModel, Weights, costs.StartCash, costs.NameCap, broker.GrossCap, broker.CashBuffer, broker.SectorCap, costs.DrawdownHalt,
		costs.MinHoldSessions, costs.TurnoverCapDay, costs.MaxHold.Hours(),
		costs.RefSTTBpsBuy, costs.RefSTTBpsSell, costs.RefExchBps, costs.RefSEBIBps, costs.RefStampBps, costs.RefGSTRate, costs.RefBrokerage,
		costs.ADVBaseBps, costs.ADVKappa, strings.Join(universe.EquitySymbols(), ","),
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:12])
}

func INR(x float64) float64 { return math.Round(x*100) / 100 }

func PP(x float64) float64 { return math.Round(x*10000) / 10000 }

func moduleRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd
}

func Dir(p string) string {
	if env := strings.TrimSpace(os.Getenv("CAMPAIGN_DIR")); env != "" && strings.TrimSpace(p) == "" {
		return env
	}
	root := moduleRoot()
	if strings.TrimSpace(p) != "" {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(root, p)
	}
	return filepath.Join(root, DefaultDir)
}

func Path(dir string) string {
	return filepath.Join(Dir(dir), LedgerFile)
}

func Load(dir string) (*Ledger, error) {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		return nil, err
	}
	var led Ledger
	if err := json.Unmarshal(b, &led); err != nil {
		return nil, err
	}
	return &led, nil
}

func Save(dir string, led *Ledger) error {
	d := Dir(dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(led, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(d, LedgerFile), b, 0o644)
}

func IsFrozen() bool {
	led, err := Load("")
	return err == nil && led.Manifest.Frozen
}

// ledgerPathspec excludes the ledger itself from dirty/diff checks: the file
// is the output of the run, so rewriting it must not make the run "dirty".
func ledgerPathspec() string {
	return ":(exclude)" + filepath.ToSlash(filepath.Join(DefaultDir, LedgerFile))
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = moduleRoot()
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// GitSHA is HEAD plus whether any tracked or untracked file other than the
// ledger differs from it. A ledger written from a dirty tree is not
// reproducible by SHA and the replay test refuses it.
func GitSHA() (sha string, dirty bool) {
	sha, err := git("rev-parse", "HEAD")
	if err != nil {
		return "unknown", true
	}
	st, err := git("status", "--porcelain", "--", ".", ledgerPathspec())
	if err != nil {
		return sha, true
	}
	return sha, st != ""
}

// LastWriter is the commit that last touched the ledger ("" if the file is
// not committed). Needs full history: CI must check out with fetch-depth 0.
func LastWriter(dir string) (string, error) {
	rel, err := filepath.Rel(moduleRoot(), Path(dir))
	if err != nil {
		return "", err
	}
	return git("log", "-1", "--format=%H", "--", filepath.ToSlash(rel))
}

// LedgerModified reports whether the working-tree ledger differs from HEAD's.
func LedgerModified(dir string) (bool, error) {
	rel, err := filepath.Rel(moduleRoot(), Path(dir))
	if err != nil {
		return false, err
	}
	cmd := exec.Command("git", "diff", "--quiet", "HEAD", "--", filepath.ToSlash(rel))
	cmd.Dir = moduleRoot()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// SameCode is true when commits a and b differ in nothing but the ledger —
// i.e. the ledger was committed alone straight on top of the code it records.
func SameCode(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	cmd := exec.Command("git", "diff", "--quiet", a, b, "--", ".", ledgerPathspec())
	cmd.Dir = moduleRoot()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MatchEquity reports whether got reproduces want's equity line within rounding.
func MatchEquity(want, got []Day) error {
	if len(want) != len(got) {
		return fmt.Errorf("day count %d vs %d", len(got), len(want))
	}
	for i := range want {
		if want[i].Date != got[i].Date {
			return fmt.Errorf("day %d date %s vs %s", i, got[i].Date, want[i].Date)
		}
		d := math.Abs(want[i].Equity - got[i].Equity)
		if d > EquityTolINR {
			return fmt.Errorf("%s equity %.2f vs published %.2f (Δ %.2f > %.2f)", want[i].Date, got[i].Equity, want[i].Equity, d, EquityTolINR)
		}
	}
	return nil
}
