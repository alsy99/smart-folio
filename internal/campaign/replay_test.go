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
