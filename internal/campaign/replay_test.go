package campaign

import (
	"os"
	"testing"

	pub "aperture/pkg/campaign"
)

func TestReplayIsDeterministic(t *testing.T) {
	a, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	if a.Manifest.SettingsHash != b.Manifest.SettingsHash {
		t.Fatalf("settings hash drifted between replays")
	}
	if err := pub.MatchEquity(a.Days, b.Days); err != nil {
		t.Fatal(err)
	}
	if len(a.Days) != pub.Days {
		t.Fatalf("published %d days, want %d", len(a.Days), pub.Days)
	}
}

// TestLedgerSHAIsTheCommitThatWroteIt: the published manifest must name the
// commit whose code produced it. Concretely — the tree was clean when the
// ledger was written, the working-tree ledger is the committed one, and the
// commit that last wrote the file is either the recorded SHA or a commit
// that changed nothing except the ledger (the ledger committed alone on top
// of the code it records). Any other pairing is a provenance lie and fails.
func TestLedgerSHAIsTheCommitThatWroteIt(t *testing.T) {
	led, err := pub.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no public ledger yet")
		}
		t.Fatal(err)
	}
	if led.Manifest.Dirty {
		t.Fatalf("ledger sha=%s was written from a dirty tree; regenerate on a clean commit", led.Manifest.GitSHA)
	}
	if led.Manifest.GitSHA == "" || led.Manifest.GitSHA == "unknown" {
		t.Fatal("ledger has no git sha")
	}
	writer, err := pub.LastWriter("")
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if writer == "" {
		t.Fatalf("ledger sha=%s is not committed; `git add campaign/public-30d/ledger.json` and commit it alone", led.Manifest.GitSHA)
	}
	if mod, err := pub.LedgerModified(""); err == nil && mod {
		t.Fatalf("working-tree ledger differs from the one committed in %s; commit or revert it", writer[:12])
	}
	same, err := pub.SameCode(led.Manifest.GitSHA, writer)
	if err != nil {
		t.Skipf("cannot diff %s..%s (shallow clone? CI needs fetch-depth 0): %v", led.Manifest.GitSHA[:12], writer[:12], err)
	}
	if !same {
		t.Fatalf("ledger claims sha %s but was last written by %s, and the code differs between them", led.Manifest.GitSHA[:12], writer[:12])
	}
}

func TestReplayMatchesPublished(t *testing.T) {
	want, err := pub.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("run `go run ./cmd/campaign -run` from the repo root to freeze the public ledger")
		}
		t.Fatal(err)
	}
	got, err := Replay()
	if err != nil {
		t.Fatal(err)
	}
	if want.Manifest.SettingsHash != got.Manifest.SettingsHash {
		t.Fatalf("settings hash %s vs published %s — freeze is broken", got.Manifest.SettingsHash, want.Manifest.SettingsHash)
	}
	if err := pub.MatchEquity(want.Days, got.Days); err != nil {
		t.Fatal(err)
	}
}
