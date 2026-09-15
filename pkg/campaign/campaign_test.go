package campaign

import "testing"

func TestSettingsHashStable(t *testing.T) {
	a := SettingsHash()
	b := SettingsHash()
	if a == "" || a != b {
		t.Fatalf("settings hash %q vs %q", a, b)
	}
	m := NewManifest("deadbeef", false)
	if !m.Frozen || m.Days != Days || m.Tape != Tape {
		t.Fatalf("manifest %+v", m)
	}
	start, end := Window()
	if !end.Equal(start.AddDate(0, 0, Days)) && end.Sub(start).Hours() != float64(Days)*24 {
		t.Fatalf("window %s → %s", start, end)
	}
}

func TestMatchEquityTolerance(t *testing.T) {
	want := []Day{{Date: "2026-08-17", Equity: 1_000_000}}
	got := []Day{{Date: "2026-08-17", Equity: 1_000_000.49}}
	if err := MatchEquity(want, got); err != nil {
		t.Fatal(err)
	}
	got[0].Equity = 1_000_002
	if err := MatchEquity(want, got); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestCampaignDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMPAIGN_DIR", dir)
	if got := Dir(""); got != dir {
		t.Fatalf("CAMPAIGN_DIR %s vs %s", got, dir)
	}
}
