package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aperture/pkg/backtest"
	"aperture/pkg/broker"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
	"aperture/pkg/universe"
)

const (
	Name = "public-30d"
	Days = 30
	// Tape is INDstocks daily history, checked in as bars.json. Fills are
	// market-on-close at the 15:30 IST tick; a 09:15 quote is the prior close.
	Tape      = "indstocks-1d"
	FillRule  = "MOC: one tick per session at 15:30 IST, filled at that session's close"
	CostModel = "delivery-v1"
	// Weights: equal across the roster that cleared the gate as-of the last
	// session before the window; failing defaults at 0. No weekly moves.
	Weights    = "equal-over-roster, failing=0"
	DefaultDir = "campaign/public-30d"
	LedgerFile = "ledger.json"
	// EquityTolINR is "within rounding" for a clone-and-replay check.
	EquityTolINR = 1.0
)

// Window is the frozen 30×24h paper campaign (positional cash, daily tape).
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
	FillRule        string   `json:"fillRule"`
	// Roster provenance: the gate verdict the book traded on, frozen as-of
	// the last session before the window. Failing defaults carried weight 0.
	Roster      []string `json:"roster"`
	Failing     []string `json:"failing"`
	RosterAsOf  string   `json:"rosterAsOf"`
	RosterTape  string   `json:"rosterTape"`
	RosterDays  int      `json:"rosterDays"`
	RosterFile  string   `json:"rosterFile"`
	BarsFile    string   `json:"barsFile"`
	BarsSHA256  string   `json:"barsSha256"`
	BarsFetched string   `json:"barsFetchedAt"`
	Reproduce   string   `json:"reproduce"`
	// Policy provenance. Empty on the legacy satellite-only campaign; a
	// policy campaign binds one IPS (by id and hash) to one ledger file.
	IPSID   string `json:"ipsId,omitempty"`
	IPSHash string `json:"ipsHash,omitempty"`
	Policy  string `json:"policy,omitempty"`
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

// Inputs are the checked-in evidence a replay runs on.
type Inputs struct {
	Roster     backtest.RosterSnapshot
	Bars       *Bars
	BarsSHA256 string
	// IPS is nil on the legacy satellite-only campaign.
	IPS *ips.IPS
}

// LoadInputs reads bars.json and roster.json from the campaign directory.
func LoadInputs(dir string) (Inputs, error) {
	bars, err := LoadBars(dir)
	if err != nil {
		return Inputs{}, fmt.Errorf("bars: %w (maintainers: go run ./cmd/campaign -fetch)", err)
	}
	roster, err := LoadRoster(dir)
	if err != nil {
		return Inputs{}, fmt.Errorf("roster: %w (maintainers: go run ./cmd/campaign -roster)", err)
	}
	sum, err := BarsSHA256(dir)
	if err != nil {
		return Inputs{}, err
	}
	return Inputs{Roster: roster, Bars: bars, BarsSHA256: sum}, nil
}

func NewManifest(gitSHA string, dirty bool, in Inputs) Manifest {
	start, end := Window()
	roster := append([]string{}, in.Roster.Roster...)
	failing := append([]string{}, in.Roster.Failing...)
	sort.Strings(roster)
	sort.Strings(failing)
	fetched := ""
	if in.Bars != nil {
		fetched = in.Bars.FetchedAt
	}
	ipsID, ipsHash, policy := "", "", ""
	if in.IPS != nil {
		ipsID, ipsHash, policy = in.IPS.ID, in.IPS.Hash(), PolicyV1
	}
	return Manifest{
		IPSID:           ipsID,
		IPSHash:         ipsHash,
		Policy:          policy,
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
		SettingsHash:    SettingsHashIPS(roster, failing, ipsHash),
		Universe:        universe.EquitySymbols(),
		Benchmarks:      Benchmarks,
		NameCap:         costs.NameCap,
		GrossCap:        broker.GrossCap,
		CashBuffer:      broker.CashBuffer,
		SectorCap:       broker.SectorCap,
		DrawdownHalt:    costs.DrawdownHalt,
		MinHoldSessions: costs.MinHoldSessions,
		TurnoverCapDay:  costs.TurnoverCapDay,
		StartCash:       costs.StartCash,
		FillRule:        FillRule,
		Roster:          roster,
		Failing:         failing,
		RosterAsOf:      in.Roster.Date,
		RosterTape:      in.Roster.Tape,
		RosterDays:      in.Roster.Days,
		RosterFile:      RosterFile,
		BarsFile:        BarsFile,
		BarsSHA256:      in.BarsSHA256,
		BarsFetched:     fetched,
		Reproduce:       "go run ./cmd/campaign -verify",
	}
}

// SettingsHash pins every parameter a replay depends on, including which
// methods were allowed to trade. The bars are pinned separately by sha256.
func SettingsHash(roster, failing []string) string {
	payload := fmt.Sprintf(
		"tape=%s|fill=%s|cost=%s|weights=%s|roster=%s|failing=%s|scalp=false|llm=false|cash=%.0f|name=%.4f|gross=%.4f|cashbuf=%.4f|sector=%.4f|halt=%.4f|min_hold_sessions=%d|turnover_cap_day=%.4f|max_hold_h=%.0f|stt_buy=%.4f|stt_sell=%.4f|exch=%.6f|sebi=%.6f|stamp=%.4f|gst=%.4f|brokerage=%.2f|adv_base=%.2f|adv_kappa=%.0f|universe=%s",
		Tape, FillRule, CostModel, Weights, strings.Join(roster, ","), strings.Join(failing, ","),
		costs.StartCash, costs.NameCap, broker.GrossCap, broker.CashBuffer, broker.SectorCap, costs.DrawdownHalt,
		costs.MinHoldSessions, costs.TurnoverCapDay, costs.MaxHold.Hours(),
		costs.RefSTTBpsBuy, costs.RefSTTBpsSell, costs.RefExchBps, costs.RefSEBIBps, costs.RefStampBps, costs.RefGSTRate, costs.RefBrokerage,
		costs.ADVBaseBps, costs.ADVKappa, strings.Join(universe.EquitySymbols(), ","),
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:12])
}

// PolicyV1 names the core+gated-satellite policy a campaign ran under.
const PolicyV1 = "core-satellite-v1"

// SettingsHashIPS folds the IPS fingerprint into the settings hash. With no
// IPS it is exactly SettingsHash, so the legacy ledger still verifies.
func SettingsHashIPS(roster, failing []string, ipsHash string) string {
	base := SettingsHash(roster, failing)
	if ipsHash == "" {
		return base
	}
	sum := sha256.Sum256([]byte(base + "|policy=" + PolicyV1 + "|ips=" + ipsHash))
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

// ErrLedgerBound: a ledger file belongs to one book. Writing a different
// IPS (or a no-IPS book) over it is refused; use a new campaign directory.
var ErrLedgerBound = errors.New("campaign: ledger file is bound to a different IPS")

func Save(dir string, led *Ledger) error {
	d := Dir(dir)
	if prev, err := Load(dir); err == nil && prev.Manifest.Frozen {
		if prev.Manifest.IPSHash != led.Manifest.IPSHash || prev.Manifest.IPSID != led.Manifest.IPSID {
			return fmt.Errorf("%w: %s holds ips %s/%s, refusing %s/%s", ErrLedgerBound, filepath.Join(d, LedgerFile),
				prev.Manifest.IPSID, prev.Manifest.IPSHash, led.Manifest.IPSID, led.Manifest.IPSHash)
		}
	}
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

// LedgerDirs lists every campaign directory under campaign/ that holds a
// ledger.json — the set the SHA-provenance test and the IPS freeze cover.
func LedgerDirs() []string {
	root := filepath.Join(moduleRoot(), "campaign")
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rel := filepath.Join("campaign", e.Name())
		if _, err := os.Stat(filepath.Join(root, e.Name(), LedgerFile)); err == nil {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// FrozenIPS reports whether an IPS is bound to any frozen ledger, and the
// hash it was frozen under. The policy service refuses to change it.
func FrozenIPS(id string) (hash string, frozen bool) {
	for _, d := range LedgerDirs() {
		led, err := Load(d)
		if err != nil || !led.Manifest.Frozen || led.Manifest.IPSID != id {
			continue
		}
		return led.Manifest.IPSHash, true
	}
	return "", false
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
