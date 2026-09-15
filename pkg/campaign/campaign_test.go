package campaign

import (
	"context"
	"errors"
	"testing"
	"time"

	marketdatav1 "aperture/gen/marketdata/v1"
	"aperture/pkg/backtest"
	"aperture/pkg/ips"
	"aperture/pkg/marketclock"
)

func TestSettingsHashStable(t *testing.T) {
	a := SettingsHash([]string{"x"}, []string{"y"})
	b := SettingsHash([]string{"x"}, []string{"y"})
	if a == "" || a != b {
		t.Fatalf("settings hash %q vs %q", a, b)
	}
	// Swapping which methods may trade is a settings change.
	if SettingsHash([]string{"x", "y"}, nil) == a {
		t.Fatal("roster must be part of the settings hash")
	}
	in := Inputs{Roster: backtest.RosterSnapshot{Date: "2026-08-14", Tape: "indstocks-1d", Roster: []string{"x"}, Failing: []string{"y"}}, BarsSHA256: "abc"}
	m := NewManifest("deadbeef", false, in)
	if !m.Frozen || m.Days != Days || m.Tape != Tape || m.BarsSHA256 != "abc" || m.RosterAsOf != "2026-08-14" || len(m.Failing) != 1 {
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

// TestBarTapeIsAsOf: a session's close becomes visible only at 15:30 IST.
// 09:15 sees yesterday; 15:30 fills at today's close (MOC).
func TestBarTapeIsAsOf(t *testing.T) {
	bars := &Bars{Series: map[string][]DailyBar{
		"RELIANCE": {
			{Date: "2026-08-14", Close: 100, Volume: 1},
			{Date: "2026-08-17", Close: 110, Volume: 1},
			{Date: "2026-08-18", Close: 120, Volume: 1},
		},
	}}
	loc := marketclock.Location()
	at := time.Date(2026, 8, 17, 9, 15, 0, 0, loc)
	tp := BarTape{Bars: bars, Now: func() time.Time { return at }}
	ctx := context.Background()
	q, err := tp.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: []string{"RELIANCE", "MF_LARGECAP"}})
	if err != nil || len(q.Quotes) != 1 || q.Quotes[0].Last != 100 {
		t.Fatalf("09:15 must quote the prior close, got %+v %v", q, err)
	}
	at = time.Date(2026, 8, 17, 15, 30, 0, 0, loc)
	q, _ = tp.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: []string{"RELIANCE"}})
	if q.Quotes[0].Last != 110 || q.Quotes[0].ChangePct < 9.99 || q.Quotes[0].ChangePct > 10.01 {
		t.Fatalf("15:30 must quote today's close: %+v", q.Quotes[0])
	}
	b, _ := tp.GetBars(ctx, &marketdatav1.GetBarsRequest{Symbol: "RELIANCE", Interval: "1w", Count: 40})
	if len(b.Bars) != 2 || b.Bars[1].Close != 110 {
		t.Fatalf("bars at 15:30 Aug 17 must end at Aug 17: %+v", b.Bars)
	}
	at = time.Date(2026, 8, 22, 12, 0, 0, 0, loc) // Saturday
	q, _ = tp.GetQuotes(ctx, &marketdatav1.GetQuotesRequest{Symbols: []string{"RELIANCE"}})
	if q.Quotes[0].Last != 120 {
		t.Fatalf("weekend quotes the last close: %+v", q.Quotes[0])
	}
	if got := RosterAsOf().Format("2006-01-02"); got != "2026-08-14" {
		t.Fatalf("roster as-of must be the last session before the window, got %s", got)
	}
}

func TestManifestCarriesIPSHash(t *testing.T) {
	p := ips.Default("c-1", 1_000_000)
	in := Inputs{Roster: backtest.RosterSnapshot{Date: "2026-08-14", Tape: "indstocks-1d", Roster: []string{}, Failing: []string{"y"}}, BarsSHA256: "abc"}
	legacy := NewManifest("deadbeef", false, in)
	in.IPS = &p
	m := NewManifest("deadbeef", false, in)
	if m.IPSHash != p.Hash() || m.IPSID != "c-1" || m.Policy != PolicyV1 {
		t.Fatalf("manifest must bind the IPS: %+v", m)
	}
	if legacy.IPSHash != "" || legacy.SettingsHash != SettingsHash(legacy.Roster, legacy.Failing) {
		t.Fatal("legacy campaign must hash exactly as before")
	}
	if m.SettingsHash == legacy.SettingsHash {
		t.Fatal("an IPS is a settings change")
	}
}

// One ledger file, one book: a frozen ledger refuses a different IPS.
func TestTwoBooksCannotShareOneLedger(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMPAIGN_DIR", dir)
	a := ips.Default("c-1", 1_000_000)
	b := ips.Default("c-1", 1_000_000)
	b.MaxDD = 0.10
	in := Inputs{Roster: backtest.RosterSnapshot{Roster: []string{}}, IPS: &a}
	if err := Save("", &Ledger{Manifest: NewManifest("sha", false, in)}); err != nil {
		t.Fatal(err)
	}
	// Same IPS again is fine (a re-run).
	if err := Save("", &Ledger{Manifest: NewManifest("sha2", false, in)}); err != nil {
		t.Fatal(err)
	}
	in.IPS = &b
	if err := Save("", &Ledger{Manifest: NewManifest("sha", false, in)}); !errors.Is(err, ErrLedgerBound) {
		t.Fatalf("want ErrLedgerBound, got %v", err)
	}
	in.IPS = nil
	if err := Save("", &Ledger{Manifest: NewManifest("sha", false, in)}); !errors.Is(err, ErrLedgerBound) {
		t.Fatalf("a no-IPS book must not overwrite a policy ledger, got %v", err)
	}
}

func TestCampaignDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMPAIGN_DIR", dir)
	if got := Dir(""); got != dir {
		t.Fatalf("CAMPAIGN_DIR %s vs %s", got, dir)
	}
}
