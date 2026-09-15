package live

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReadyIsFalseUntilChecklist(t *testing.T) {
	if Ready() {
		t.Fatal("live orders must stay off until every NSE gate is met")
	}
	miss := Missing()
	if len(miss) == 0 {
		t.Fatal("expected unmet gates")
	}
}

func TestKillSwitchHumanCanHit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "KILL")
	t.Setenv("KILL_FILE", p)
	if Killed() {
		t.Fatal("fresh")
	}
	if err := Trip(); err != nil {
		t.Fatal(err)
	}
	if !Killed() {
		t.Fatal("trip")
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if Killed() {
		t.Fatal("cleared")
	}
}

func TestLimiterStaysUnderExchangeCap(t *testing.T) {
	var l Limiter
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < MaxOrdersPerSec; i++ {
		if !l.Allow(now) {
			t.Fatalf("allowed %d", i)
		}
	}
	if l.Allow(now) {
		t.Fatal("must refuse above MaxOrdersPerSec")
	}
	if !l.Allow(now.Add(time.Second)) {
		t.Fatal("window elapsed")
	}
}

func TestMainsHaveNoLiveOrdersFlag(t *testing.T) {
	_, this, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(this), "../.."))
	var hits []string
	_ = filepath.Walk(filepath.Join(root, "cmd"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "LIVE_ORDERS") {
			hits = append(hits, path)
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("LIVE_ORDERS must not exist in cmd: %v", hits)
	}
}

func TestOrdersModeIsCompileOff(t *testing.T) {
	if OrdersMode() != "compile-off" {
		t.Fatalf("%s", OrdersMode())
	}
}
