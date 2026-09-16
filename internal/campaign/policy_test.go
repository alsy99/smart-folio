package campaign

import (
	"math"
	"os"
	"strings"
	"testing"

	pub "aperture/pkg/campaign"
)

const (
	dirCore = "campaign/public-30d-core"
	dirFold = "campaign/public-30d-fold"
	dirHalt = "campaign/public-30d-halt"
)

func loadOrSkip(t *testing.T, dir string) *pub.Ledger {
	t.Helper()
	led, err := pub.Load(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no frozen ledger in %s yet (go run ./cmd/campaign -run -name %s)", dir, strings.TrimPrefix(dir, "campaign/"))
		}
		t.Fatal(err)
	}
	return led
}

// TestEveryFrozenLedgerReplaysWithinARupee: a stranger's clone replays
// each campaign/*/ledger.json on its checked-in tape and IPS to the
// published equity line, with the same bars sha256, settings hash and IPS
// hash. Covers the legacy cash month and every policy book.
func TestEveryFrozenLedgerReplaysWithinARupee(t *testing.T) {
	dirs := pub.LedgerDirs()
	if len(dirs) == 0 {
		t.Skip("no frozen ledgers")
	}
	for _, dir := range dirs {
		want, err := pub.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReplayDir(dir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		if want.Manifest.BarsSHA256 != got.Manifest.BarsSHA256 {
			t.Fatalf("%s: bars sha %s vs published %s", dir, got.Manifest.BarsSHA256, want.Manifest.BarsSHA256)
		}
		if want.Manifest.SettingsHash != got.Manifest.SettingsHash {
			t.Fatalf("%s: settings hash %s vs published %s", dir, got.Manifest.SettingsHash, want.Manifest.SettingsHash)
		}
		if want.Manifest.IPSHash != got.Manifest.IPSHash || want.Manifest.IPSID != got.Manifest.IPSID {
			t.Fatalf("%s: ips %s/%s vs published %s/%s", dir, got.Manifest.IPSID, got.Manifest.IPSHash, want.Manifest.IPSID, want.Manifest.IPSHash)
		}
		if err := pub.MatchEquity(want.Days, got.Days); err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		if want.Manifest.Name != strings.TrimPrefix(dir, "campaign/") {
			t.Fatalf("%s: manifest name %q", dir, want.Manifest.Name)
		}
		if want.Manifest.CostModel != pub.CostModel || !strings.HasPrefix(want.Manifest.Tape, pub.Tape) {
			t.Fatalf("%s: tape %q cost model %q", dir, want.Manifest.Tape, want.Manifest.CostModel)
		}
		if want.Manifest.RosterAsOf != pub.RosterAsOf().Format("2006-01-02") {
			t.Fatalf("%s: roster as-of %s, want the last session before the window", dir, want.Manifest.RosterAsOf)
		}
	}
}

// TestEveryFrozenLedgerSHAIsTheCommitThatWroteIt extends the provenance
// rule to every campaign directory.
func TestEveryFrozenLedgerSHAIsTheCommitThatWroteIt(t *testing.T) {
	for _, dir := range pub.LedgerDirs() {
		led, err := pub.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if led.Manifest.Dirty || led.Manifest.GitSHA == "" || led.Manifest.GitSHA == "unknown" {
			t.Fatalf("%s: sha=%q dirty=%v", dir, led.Manifest.GitSHA, led.Manifest.Dirty)
		}
		writer, err := pub.LastWriter(dir)
		if err != nil {
			t.Skipf("git unavailable: %v", err)
		}
		if writer == "" {
			t.Skipf("%s: ledger not committed yet; freeze then commit ledger.json alone", dir)
		}
		if mod, err := pub.LedgerModified(dir); err == nil && mod {
			t.Fatalf("%s: working-tree ledger differs from %s", dir, writer[:12])
		}
		same, err := pub.SameCode(led.Manifest.GitSHA, writer)
		if err != nil {
			t.Skipf("cannot diff %s..%s: %v", led.Manifest.GitSHA[:12], writer[:12], err)
		}
		if !same {
			t.Fatalf("%s: ledger claims %s but was written by %s and the code differs", dir, led.Manifest.GitSHA[:12], writer[:12])
		}
	}
}

func lastOpen(led *pub.Ledger) pub.Day {
	var last pub.Day
	for _, d := range led.Days {
		if d.Session == "open" {
			last = d
		}
	}
	return last
}

// TestBookAIsInvestedAndTracksNiftyMinusCosts: the core book is not cash.
// Its excess vs Nifty 50 is small and comes from costs and from holding a
// 12-name equal-weight sample at ~81% (rails) against a cap-weighted
// index: nothing here is tuned to win.
func TestBookAIsInvestedAndTracksNiftyMinusCosts(t *testing.T) {
	led := loadOrSkip(t, dirCore)
	if led.Manifest.Policy != pub.PolicyV1 || led.Manifest.IPSID == "" || led.Manifest.IPSHash == "" {
		t.Fatalf("book A must carry its IPS: %+v", led.Manifest)
	}
	if led.Manifest.Synthetic != "" || led.Manifest.Tape != pub.Tape {
		t.Fatalf("book A is the real tape: %q %q", led.Manifest.Tape, led.Manifest.Synthetic)
	}
	last := lastOpen(led)
	if last.Halted {
		t.Fatal("book A did not halt on this tape")
	}
	if last.CoreINR < 0.7*led.Manifest.StartCash {
		t.Fatalf("book A must be invested, core %.0f of %.0f", last.CoreINR, led.Manifest.StartCash)
	}
	if last.SatelliteINR != 0 || last.SatelliteFills != 0 {
		t.Fatalf("satellite is empty on this roster: %+v", last)
	}
	if math.Abs(last.ExcessNifty50Pct) > 5 {
		t.Fatalf("excess vs Nifty %.2f%% is not 'small'; do not tune, explain", last.ExcessNifty50Pct)
	}
	buys := 0
	for _, d := range led.Days {
		buys += d.CoreFills
	}
	if buys == 0 {
		t.Fatal("the core never built")
	}
}

// TestBookBMatchesAWithEmptySatelliteFoldIn: an 80/20 IPS with fold-in and
// no admitted method is the 100% core book to the rupee.
func TestBookBMatchesAWithEmptySatelliteFoldIn(t *testing.T) {
	a := loadOrSkip(t, dirCore)
	b := loadOrSkip(t, dirFold)
	if a.Manifest.IPSHash == b.Manifest.IPSHash {
		t.Fatal("A and B are different statements")
	}
	if err := pub.MatchEquity(a.Days, b.Days); err != nil {
		t.Fatalf("B should track A: %v", err)
	}
	for i := range a.Days {
		if a.Days[i].CoreINR != b.Days[i].CoreINR || b.Days[i].SatelliteINR != 0 {
			t.Fatalf("%s: core %.0f vs %.0f, satellite %.0f", a.Days[i].Date, a.Days[i].CoreINR, b.Days[i].CoreINR, b.Days[i].SatelliteINR)
		}
	}
}

// TestBookCHaltsAndBuysNothingAfter: on the documented synthetic shock the
// client cap fires once and every later session has zero fills.
func TestBookCHaltsAndBuysNothingAfter(t *testing.T) {
	led := loadOrSkip(t, dirHalt)
	if led.Manifest.Synthetic == "" || !strings.HasSuffix(led.Manifest.Tape, pub.Synthetic) {
		t.Fatalf("book C must declare its synthetic shock: tape %q synthetic %q", led.Manifest.Tape, led.Manifest.Synthetic)
	}
	halted := false
	for _, d := range led.Days {
		if d.Session != "open" {
			continue
		}
		if d.Halted {
			if !halted && d.DrawdownPct < 15 {
				t.Fatalf("%s: halted at %.2f%% drawdown, below the cap", d.Date, d.DrawdownPct)
			}
			halted = true
			if d.Fills != 0 || d.CoreFills != 0 {
				t.Fatalf("%s: %d fills after the halt", d.Date, d.Fills)
			}
			continue
		}
		if halted {
			t.Fatalf("%s: halt released inside the window", d.Date)
		}
	}
	if !halted {
		t.Fatal("the halt never fired")
	}
	if last := lastOpen(led); last.CoreINR == 0 {
		t.Fatal("a halted core holds; it does not liquidate")
	}
}
