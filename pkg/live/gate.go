package live

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aperture/pkg/config"
)

var (
	// ErrCompileOff is returned by PlaceOrder in the default (!liveorders) build.
	ErrCompileOff = errors.New("live orders are compile-time off; the paper book is the only fill path")
	// ErrChecklist is returned even in a liveorders build until every NSE gate is met.
	ErrChecklist = errors.New("live orders blocked: strategy spec, principal path, Algo-ID, static IP, rate limit, kill switch, and 10pp-is-a-target disclosure are not all met")
	ErrKilled    = errors.New("kill switch is set; a human must clear data/KILL")
)

// Paper-lab checklist. Flip a constant only when that gate is actually done.
// Ready() is false in main — do not add a LIVE_ORDERS env flag.
const (
	WrittenStrategySpec = false // DefaultSpecs is the paper roster, not an exchange-filed algo spec
	BrokerPrincipalPath = false // cmd/trading never routes a principal live adapter
	AlgoIDTagging       = false // Order.AlgoID 99999 is a placeholder, not a registered NSE algo id
	StaticIP            = false // order endpoints are not pinned to a declared static IP
	OrderRateArmed      = false // limiter exists; it is not armed on the paper path
	HumanKillSwitch     = true  // Stop Autopilot + KILL file
	ExcessIsTarget      = true  // +10pp vs Nifty is a measured target, not a promise
)

func Ready() bool {
	return WrittenStrategySpec && BrokerPrincipalPath && AlgoIDTagging &&
		StaticIP && OrderRateArmed && HumanKillSwitch && ExcessIsTarget
}

func Missing() []string {
	var out []string
	if !WrittenStrategySpec {
		out = append(out, "written strategy spec")
	}
	if !BrokerPrincipalPath {
		out = append(out, "broker principal path")
	}
	if !AlgoIDTagging {
		out = append(out, "Algo-ID tagging")
	}
	if !StaticIP {
		out = append(out, "static IP for order endpoints")
	}
	if !OrderRateArmed {
		out = append(out, "order-rate under exchange threshold")
	}
	if !HumanKillSwitch {
		out = append(out, "human kill switch")
	}
	if !ExcessIsTarget {
		out = append(out, "10pp vs Nifty disclosed as a target not a promise")
	}
	return out
}

func OrdersMode() string {
	return "compile-off"
}

func killPath() string {
	return config.String("KILL_FILE", "data/KILL")
}

var killMu sync.Mutex

func Killed() bool {
	killMu.Lock()
	defer killMu.Unlock()
	_, err := os.Stat(killPath())
	return err == nil
}

// Trip is the human kill switch: Stop Autopilot and `touch data/KILL`.
func Trip() error {
	killMu.Lock()
	defer killMu.Unlock()
	p := killPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil && filepath.Dir(p) != "." {
		return err
	}
	return os.WriteFile(p, []byte("halted "+time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func Clear() error {
	killMu.Lock()
	defer killMu.Unlock()
	err := os.Remove(killPath())
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// MaxOrdersPerSec is below typical NSE algo message caps (often ≥10/s).
const MaxOrdersPerSec = 5

type Limiter struct {
	mu   sync.Mutex
	last []time.Time
}

func (l *Limiter) Allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-time.Second)
	kept := l.last[:0]
	for _, t := range l.last {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.last = kept
	if len(l.last) >= MaxOrdersPerSec {
		return false
	}
	l.last = append(l.last, now)
	return true
}
