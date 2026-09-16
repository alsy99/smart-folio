package campaign

import (
	"path/filepath"
	"testing"

	"aperture/pkg/ips"
)

func TestLoadSpecMissingIsLegacy(t *testing.T) {
	s, err := LoadSpec(t.TempDir())
	if err != nil || s != nil {
		t.Fatalf("missing campaign.json must be the legacy book: %+v %v", s, err)
	}
}

func TestShockAppliesFromDateAndLeavesSourceUnchanged(t *testing.T) {
	in := &Bars{Series: map[string][]DailyBar{
		"TCS": {
			{Date: "2026-08-31", Open: 100, High: 110, Low: 90, Close: 105},
			{Date: "2026-09-01", Open: 100, High: 110, Low: 90, Close: 105},
		},
	}}
	out := (&Shock{From: "2026-09-01", Factor: 0.8}).Apply(in)
	if in.Series["TCS"][1].Close != 105 {
		t.Fatal("bars.json must stay the real tape")
	}
	if out.Series["TCS"][0].Close != 105 || out.Series["TCS"][1].Close != 84 {
		t.Fatalf("shock should mark from From inclusive: %+v", out.Series["TCS"])
	}
	if (&Shock{From: "2026-09-01", Factor: 0.8, Note: "fixture"}).Describe() == "" {
		t.Fatal("synthetic books must say so")
	}
}

func TestSaveLoadSpecValidatesIPS(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMPAIGN_DIR", dir)
	p := ips.Default("ips-a-core100", 1_000_000)
	if err := SaveSpec("", Spec{Name: "public-30d-core", IPS: &p, Note: "A"}); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpec("")
	if err != nil || s.Name != "public-30d-core" || s.IPS == nil || s.IPS.Hash() != p.Hash() {
		t.Fatalf("%+v %v", s, err)
	}
	bad := p
	bad.CorePct, bad.SatellitePct = 0.5, 0.5
	if err := SaveSpec("", Spec{Name: "x", IPS: &bad}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSpec(""); err == nil {
		t.Fatal("an invalid IPS must not load")
	}
}

func TestReproduceCmdNamesTheBook(t *testing.T) {
	if ReproduceCmd(Name) != "go run ./cmd/campaign -verify" {
		t.Fatal(ReproduceCmd(Name))
	}
	if got := ReproduceCmd("public-30d-core"); got != "go run ./cmd/campaign -verify -name public-30d-core" {
		t.Fatal(got)
	}
}

func TestCheckedInPolicySpecs(t *testing.T) {
	root := moduleRoot()
	for _, name := range []string{"public-30d-core", "public-30d-fold", "public-30d-halt"} {
		s, err := LoadSpec(filepath.Join(root, "campaign", name))
		if err != nil || s == nil || s.IPS == nil {
			t.Fatalf("%s: %+v %v", name, s, err)
		}
		if s.Name != name {
			t.Fatalf("%s name %q", name, s.Name)
		}
	}
	core, _ := LoadSpec(filepath.Join(root, "campaign", "public-30d-core"))
	fold, _ := LoadSpec(filepath.Join(root, "campaign", "public-30d-fold"))
	halt, _ := LoadSpec(filepath.Join(root, "campaign", "public-30d-halt"))
	if core.IPS.CorePct != 1 || core.IPS.SatellitePct != 0 || core.Shock != nil {
		t.Fatalf("A: %+v", core.IPS)
	}
	if fold.IPS.CorePct != 0.8 || !fold.IPS.FoldSatellite || fold.Shock != nil {
		t.Fatalf("B: %+v", fold.IPS)
	}
	if halt.Shock == nil || halt.Shock.Factor != 0.8 || halt.IPS.CorePct != 1 {
		t.Fatalf("C: %+v", halt)
	}
}
