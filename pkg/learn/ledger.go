package learn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Weight struct {
	StrategyID string  `json:"strategyId"`
	Weight     float64 `json:"weight"`
	Expectancy float64 `json:"expectancy"`
	WinRate    float64 `json:"winRate"`
	Regime     string  `json:"regime"`
	N          int     `json:"n"`
}

// Snapshot is the last weekly weight file. GetWeights reads this; RecordTrade
// never writes it.
type Snapshot struct {
	AsOf    string   `json:"asOf"`    // RFC3339
	NextDue string   `json:"nextDue"` // RFC3339
	Weights []Weight `json:"weights"`
	Moved   bool     `json:"moved"`
	Note    string   `json:"note"`
	Shrunk  []string `json:"shrunk,omitempty"`
}

func ClosesPath(dir string) string {
	return filepath.Join(dir, ClosesFile)
}

func WeightsPath(dir string) string {
	return filepath.Join(dir, WeightsFile)
}

func AppendClose(dir string, c Close) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(ClosesPath(dir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err != nil {
		return err
	}
	return BackupCloses(dir, time.Now().UTC())
}

// BackupCloses copies closes.jsonl to backup/closes-YYYY-MM-DD.jsonl.
func BackupCloses(dir string, now time.Time) error {
	src := ClosesPath(dir)
	in, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dstDir := filepath.Join(dir, BackupDir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dst := filepath.Join(dstDir, "closes-"+now.Format("2006-01-02")+".jsonl")
	return os.WriteFile(dst, in, 0o644)
}

func LoadCloses(dir string) ([]Close, error) {
	f, err := os.Open(ClosesPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Close
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c Close
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("closes.jsonl: %w", err)
		}
		if c.Method == "" {
			c.Method = MethodOf(c.StrategyID)
		}
		if c.Regime == "" {
			c.Regime = "chop"
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

func SaveSnapshot(dir string, snap Snapshot) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(WeightsPath(dir), b, 0o644)
}

func LoadSnapshot(dir string) (Snapshot, error) {
	b, err := os.ReadFile(WeightsPath(dir))
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func ParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func EqualSnapshot(ids []string, now time.Time) Snapshot {
	w := EqualWeights(ids)
	rows := make([]Weight, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, Weight{
			StrategyID: id, Weight: w[id], Regime: "pending",
		})
	}
	return Snapshot{
		AsOf:    now.UTC().Format(time.RFC3339),
		NextDue: now.UTC().Add(Period).Format(time.RFC3339),
		Weights: rows,
		Moved:   false,
		Note:    "equal weights; weekly job has not run",
	}
}
