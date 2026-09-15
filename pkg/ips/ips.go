// Package ips is the Investment Policy Statement: what a client asked for,
// as constraints the book must respect. Enumerated goals, a drawdown cap,
// a benchmark, and a core/satellite split. No free text — "make 10% a
// month" is not a goal the desk accepts.
package ips

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"aperture/pkg/universe"
)

type Goal string

const (
	GoalBeatNifty     Goal = "beat_nifty"
	GoalBeatInflation Goal = "beat_inflation"
	GoalMaxDD         Goal = "max_dd"
)

type Benchmark string

const (
	BenchNifty50  Benchmark = "NIFTY50"
	BenchNifty500 Benchmark = "NIFTY500"
	BenchSensex   Benchmark = "SENSEX"
)

type Rebalance string

const (
	RebalanceMonthly   Rebalance = "monthly"
	RebalanceQuarterly Rebalance = "quarterly"
)

// v1 caps. These are the product's outer walls, not defaults to tune.
const (
	MaxDDCap        = 0.15
	CorePctMin      = 0.80
	CorePctMax      = 1.00
	SatellitePctCap = 0.20
	DefaultMaxDD    = 0.15
	// SplitTol is how far SatellitePct may sit from 1−CorePct before the
	// statement is rejected as internally inconsistent.
	SplitTol = 1e-9
)

var Horizons = []int{1, 3, 5, 10}

type IPS struct {
	ID            string    `json:"id"`
	Goal          Goal      `json:"goal"`
	HorizonYears  int       `json:"horizonYears"`
	MaxDD         float64   `json:"maxDd"`
	Benchmark     Benchmark `json:"benchmark"`
	CorePct       float64   `json:"corePct"`
	SatellitePct  float64   `json:"satellitePct"`
	Rebalance     Rebalance `json:"rebalance"`
	StartCash     float64   `json:"startCash"`
	FoldSatellite bool      `json:"foldSatellite"`
}

// Available reports whether the active tape carries a bar series for a
// benchmark. Callers pass the tape's view; nil means "trust the enum".
type Available func(Benchmark) bool

var (
	ErrGoal       = errors.New("ips: goal must be one of beat_nifty, beat_inflation, max_dd")
	ErrHorizon    = errors.New("ips: horizon must be 1, 3, 5 or 10 years")
	ErrMaxDD      = errors.New("ips: max drawdown must be in (0, 0.15]")
	ErrBenchmark  = errors.New("ips: benchmark must be NIFTY50, NIFTY500 or SENSEX")
	ErrNoSeries   = errors.New("ips: benchmark has no bar series on the active tape")
	ErrCorePct    = errors.New("ips: core must be between 80% and 100% of the book")
	ErrSplit      = errors.New("ips: satellite must equal 1 − core")
	ErrSatellite  = errors.New("ips: satellite may not exceed 20% of the book")
	ErrRebalance  = errors.New("ips: rebalance must be monthly or quarterly")
	ErrStartCash  = errors.New("ips: start cash must be positive")
	ErrID         = errors.New("ips: id required")
	ErrFreeText   = errors.New("ips: free-text goals are not accepted; pick a goal and a pain limit")
	ErrPromissory = errors.New("ips: a return promise is not a goal")
)

// Validate is the whole rule book. It never rounds a value into range.
func (p IPS) Validate() error {
	return p.ValidateOn(nil)
}

// ValidateOn also checks the benchmark exists on the active tape.
func (p IPS) ValidateOn(avail Available) error {
	if strings.TrimSpace(p.ID) == "" {
		return ErrID
	}
	if looksPromissory(string(p.Goal)) {
		return ErrPromissory
	}
	switch p.Goal {
	case GoalBeatNifty, GoalBeatInflation, GoalMaxDD:
	default:
		if strings.TrimSpace(string(p.Goal)) != "" && strings.ContainsAny(string(p.Goal), " %") {
			return ErrFreeText
		}
		return ErrGoal
	}
	if !contains(Horizons, p.HorizonYears) {
		return ErrHorizon
	}
	if !(p.MaxDD > 0 && p.MaxDD <= MaxDDCap+1e-12) {
		return ErrMaxDD
	}
	switch p.Benchmark {
	case BenchNifty50, BenchNifty500, BenchSensex:
	default:
		return ErrBenchmark
	}
	if avail != nil && !avail(p.Benchmark) {
		return ErrNoSeries
	}
	if p.CorePct < CorePctMin-1e-12 || p.CorePct > CorePctMax+1e-12 {
		return ErrCorePct
	}
	if math.Abs(p.SatellitePct-(1-p.CorePct)) > SplitTol {
		return ErrSplit
	}
	if p.SatellitePct > SatellitePctCap+1e-12 {
		return ErrSatellite
	}
	switch p.Rebalance {
	case RebalanceMonthly, RebalanceQuarterly:
	default:
		return ErrRebalance
	}
	if !(p.StartCash > 0) {
		return ErrStartCash
	}
	return nil
}

// Hash is a stable fingerprint of every field that changes what the book
// does. It goes into the campaign manifest; the ID is included so two
// clients with identical settings still have distinct ledgers.
func (p IPS) Hash() string {
	payload := fmt.Sprintf(
		"id=%s|goal=%s|horizon=%d|max_dd=%.6f|bench=%s|core=%.6f|sat=%.6f|rebal=%s|cash=%.2f|fold=%t",
		p.ID, p.Goal, p.HorizonYears, p.MaxDD, p.Benchmark, p.CorePct, p.SatellitePct, p.Rebalance, p.StartCash, p.FoldSatellite,
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:12])
}

// Default is the shipped statement: beat Nifty 50 over five years, 15%
// pain limit, 100% core, monthly, empty satellite folded into core.
func Default(id string, startCash float64) IPS {
	return IPS{
		ID: id, Goal: GoalBeatNifty, HorizonYears: 5, MaxDD: DefaultMaxDD,
		Benchmark: BenchNifty50, CorePct: 1.0, SatellitePct: 0,
		Rebalance: RebalanceMonthly, StartCash: startCash, FoldSatellite: true,
	}
}

// CoreSymbols is the v1 core universe: the twelve campaign names. The
// allocator in pkg/core owns the weights; this only names the set so a
// caller can ask for marks/ADV without importing the allocator.
func (p IPS) CoreSymbols() []string { return universe.EquitySymbols() }

// Line is the one-line desk summary.
func (p IPS) Line() string {
	return fmt.Sprintf("%s · %dy · DD cap %.0f%% · vs %s · core %.0f%% / satellite %.0f%% · %s",
		p.Goal, p.HorizonYears, p.MaxDD*100, p.Benchmark, p.CorePct*100, p.SatellitePct*100, p.Rebalance)
}

// looksPromissory catches return promises typed into the goal field.
func looksPromissory(s string) bool {
	l := strings.ToLower(s)
	for _, w := range []string{"guarantee", "guaranteed", "assured", "risk-free", "risk free", "99.99", "% a month", "% per month", "% monthly", "double"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

func contains(xs []int, x int) bool {
	i := sort.SearchInts(xs, x)
	return i < len(xs) && xs[i] == x
}
