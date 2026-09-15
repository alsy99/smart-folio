package main

import (
	"flag"
	"fmt"
	"os"

	runcamp "aperture/internal/campaign"
	"aperture/pkg/campaign"
)

func main() {
	run := flag.Bool("run", false, "write campaign/public-30d/ledger.json (refused once frozen unless -force)")
	verify := flag.Bool("verify", false, "replay the frozen tape and match the published equity line")
	force := flag.Bool("force", false, "allow -run to overwrite a frozen ledger")
	dir := flag.String("dir", campaign.DefaultDir, "ledger directory")
	flag.Parse()

	if !*run && !*verify {
		*verify = true
	}

	got, err := runcamp.Replay()
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}

	if *run {
		if campaign.IsFrozen() && !*force {
			fmt.Fprintln(os.Stderr, "public 30-day campaign is frozen. Clone the SHA and use -verify. Maintainers: -run -force.")
			os.Exit(2)
		}
		if err := campaign.Save(*dir, got); err != nil {
			fmt.Fprintf(os.Stderr, "save: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s sha=%s dirty=%v days=%d equity=%.2f\n",
			campaign.Path(*dir), got.Manifest.GitSHA, got.Manifest.Dirty, len(got.Days), lastEquity(got))
	}

	if *verify {
		want, err := campaign.Load(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		if err := campaign.MatchEquity(want.Days, got.Days); err != nil {
			fmt.Fprintf(os.Stderr, "equity line: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ok sha=%s settings=%s days=%d last_equity=%.2f halted=%v\n",
			want.Manifest.GitSHA, want.Manifest.SettingsHash, len(want.Days), lastEquity(want), lastHalt(want))
	}
}

func lastEquity(led *campaign.Ledger) float64 {
	if led == nil || len(led.Days) == 0 {
		return 0
	}
	return led.Days[len(led.Days)-1].Equity
}

func lastHalt(led *campaign.Ledger) bool {
	if led == nil || len(led.Days) == 0 {
		return false
	}
	return led.Days[len(led.Days)-1].Halted
}
