package learn

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func EqualWeights(ids []string) map[string]float64 {
	out := map[string]float64{}
	if len(ids) == 0 {
		return out
	}
	eq := 1.0 / float64(len(ids))
	for _, id := range ids {
		out[id] = eq
	}
	return out
}

func copyWeights(src map[string]float64, ids []string) map[string]float64 {
	out := EqualWeights(ids)
	if len(src) == 0 {
		return out
	}
	for _, id := range ids {
		// An explicit 0 is a decision (a method shrunk to nothing) and is
		// kept; only a missing or corrupt value falls back to equal weight.
		if w, ok := src[id]; ok && w >= 0 {
			out[id] = w
		}
	}
	return renormalize(out)
}

func SnapshotMap(snap Snapshot) map[string]float64 {
	out := map[string]float64{}
	for _, w := range snap.Weights {
		out[w.StrategyID] = w.Weight
	}
	return out
}

func Due(snap Snapshot, now time.Time) bool {
	if snap.NextDue == "" {
		return true
	}
	due := ParseTime(snap.NextDue)
	if due.IsZero() {
		return true
	}
	return !now.Before(due)
}

// Reweight shrinks strategy weights whose (strategy, regime) tag has
// negative expectancy and n ≥ MinN. A single fill never qualifies.
func Reweight(ids []string, prev map[string]float64, closes []Close) (map[string]float64, []string) {
	next := copyWeights(prev, ids)
	var shrunk []string
	for _, row := range ByTag(closes) {
		if row.N < MinN || row.Expectancy() >= 0 {
			continue
		}
		if _, ok := next[row.StrategyID]; !ok {
			continue
		}
		next[row.StrategyID] *= Shrink
		if next[row.StrategyID] < MinWeight {
			next[row.StrategyID] = MinWeight
		}
		shrunk = append(shrunk, row.Key)
	}
	sort.Strings(shrunk)
	return renormalize(next), shrunk
}

func renormalize(w map[string]float64) map[string]float64 {
	sum := 0.0
	for _, v := range w {
		sum += v
	}
	if sum <= 0 {
		return w
	}
	for k, v := range w {
		w[k] = v / sum
	}
	return w
}

// ApplyWeekly is the fill-tag review alone; ApplyReview adds the period
// table. Both are the scheduled job and the only writers of weights.
func ApplyWeekly(ids []string, prev Snapshot, closes []Close, now time.Time, force bool) Snapshot {
	return ApplyReview(ids, prev, closes, nil, now, force)
}

// ApplyReview runs the scheduled weight review: fill tags (n ≥ MinN
// closes per strategy|regime with negative expectancy) and satellite
// periods (n ≥ MinN periods or fills with negative excess vs the IPS
// benchmark). Core periods are reported, never weighted.
func ApplyReview(ids []string, prev Snapshot, closes []Close, periods []Period, now time.Time, force bool) Snapshot {
	if !force && !Due(prev, now) {
		prev.Moved = false
		prev.Note = "weights hold until " + prev.NextDue
		return prev
	}
	cur := SnapshotMap(prev)
	next, shrunk := Reweight(ids, cur, closes)
	next, shrunkP := ReweightPeriods(ids, next, periods, closes)
	shrunk = append(shrunk, shrunkP...)
	moved := len(shrunk) > 0
	byID := map[string]Row{}
	for _, row := range ByStrategy(closes) {
		byID[row.StrategyID] = row
	}
	rows := make([]Weight, 0, len(ids))
	for _, id := range ids {
		st := byID[id]
		regime := "mixed"
		if st.N == 0 {
			regime = "pending"
		}
		wr := 0.5
		if st.N > 0 {
			wr = float64(st.Wins) / float64(st.N)
		}
		rows = append(rows, Weight{
			StrategyID: id, Weight: next[id],
			Expectancy: st.Expectancy(), WinRate: wr, Regime: regime, N: st.N,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Weight == rows[j].Weight {
			return rows[i].StrategyID < rows[j].StrategyID
		}
		return rows[i].Weight > rows[j].Weight
	})
	note := "weekly review; no tag met n≥20 with negative expectancy"
	if moved {
		note = fmt.Sprintf("weekly review; shrunk %s", strings.Join(shrunk, ", "))
	}
	return Snapshot{
		AsOf:    now.UTC().Format(time.RFC3339),
		NextDue: now.UTC().Add(ReviewEvery).Format(time.RFC3339),
		Weights: rows,
		Moved:   moved,
		Note:    note,
		Shrunk:  shrunk,
	}
}
