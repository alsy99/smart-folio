package backtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Promotion is one newly admitted roster entry with its OOS evidence.
type Promotion struct {
	ID           string  `json:"id"`
	OOSExcessPct float64 `json:"oosExcessPct"`
	OOSTrades    int     `json:"oosTrades"`
	MaxDDPct     float64 `json:"maxDdPct"`
}

// RosterSnapshot is the dated, on-disk roster that the live book runs.
// The trading book never uses "whatever won last night": it boots from the
// latest snapshot file, and a backtest run writes at most one file per day.
type RosterSnapshot struct {
	Date string `json:"date"` // YYYY-MM-DD (IST run date)
	// Roster is the set that trades: everything that cleared the gate on
	// this tape. It may be empty — then the paper book holds cash.
	Roster []string    `json:"roster"`
	Added  []Promotion `json:"added"` // new admissions this snapshot (≤ MaxNewPerSnap)
	// Failing lists shipped defaults that missed the gate on this tape.
	// They are off the roster and carry weight 0 on the paper book until a
	// later snapshot readmits them. Listed so the file says it out loud.
	Failing []string `json:"failing"`
	Folds   int      `json:"folds"`
	Tape    string   `json:"tape"`   // "indstocks-1d" | "bhavcopy" — never "mock"
	Days    int      `json:"days"`   // daily closes the folds ran on
	GitSHA  string   `json:"gitSha"` // commit the lab ran at
	Note    string   `json:"note"`
}

// SaveRoster writes data/roster/YYYY-MM-DD.json. One file per day: a re-run
// on the same date overwrites rather than forks the roster. A mock-tape
// snapshot is refused: the live book must not boot from sine-wave evidence.
func SaveRoster(dir string, snap RosterSnapshot) (string, error) {
	if snap.Date == "" {
		return "", fmt.Errorf("roster snapshot has no date")
	}
	if len(snap.Roster) == 0 && len(snap.Failing) == 0 {
		return "", fmt.Errorf("roster snapshot judged nothing")
	}
	if IsMockTape(snap.Tape) {
		return "", fmt.Errorf("roster snapshot from the %q tape is not promotable; run the lab on real daily bars", snap.Tape)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, snap.Date+".json")
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

// LoadLatestRoster returns the newest dated snapshot in dir.
func LoadLatestRoster(dir string) (RosterSnapshot, string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return RosterSnapshot{}, "", err
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return RosterSnapshot{}, "", fmt.Errorf("no roster snapshots in %s", dir)
	}
	sort.Strings(names) // dated names sort chronologically
	path := filepath.Join(dir, names[len(names)-1])
	b, err := os.ReadFile(path)
	if err != nil {
		return RosterSnapshot{}, "", err
	}
	var snap RosterSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return RosterSnapshot{}, "", err
	}
	if len(snap.Roster) == 0 && len(snap.Failing) == 0 {
		return RosterSnapshot{}, "", fmt.Errorf("roster snapshot %s judged nothing", path)
	}
	if IsMockTape(snap.Tape) {
		return RosterSnapshot{}, "", fmt.Errorf("roster snapshot %s is from the %q tape; not admissible", path, snap.Tape)
	}
	return snap, path, nil
}

// RosterStale reports whether the snapshot is older than maxAgeDays.
func RosterStale(snap RosterSnapshot, now time.Time, maxAgeDays int) bool {
	d, err := time.Parse("2006-01-02", snap.Date)
	if err != nil {
		return true
	}
	return now.Sub(d) > time.Duration(maxAgeDays)*24*time.Hour
}
