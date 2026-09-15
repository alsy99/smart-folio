// Command lab runs the five-year walk-forward on real daily bars and, only
// when the tape is real, writes data/roster/YYYY-MM-DD.json.
//
//	go run ./cmd/lab                 # INDstocks 1d history if a token is set, else mock (no roster)
//	go run ./cmd/lab -write          # also write the dated roster snapshot
//	go run ./cmd/lab -tape mock      # plumbing check; never writes
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"aperture/pkg/backtest"
	"aperture/pkg/indstocks"
	"aperture/pkg/tape"
)

func main() {
	years := flag.Int("years", 5, "history depth")
	which := flag.String("tape", "auto", "auto | indstocks | mock")
	barsDir := flag.String("bars", tape.DefaultDir, "daily-bar cache directory")
	rosterDir := flag.String("roster", "data/roster", "roster snapshot directory")
	write := flag.Bool("write", false, "write the roster snapshot (refused on the mock tape)")
	refresh := flag.Bool("refresh", false, "refetch bars even if cached")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var tp backtest.Tape
	switch *which {
	case "mock":
		tp = backtest.MockTape{}
	case "indstocks":
		c := indstocks.Shared()
		if c == nil {
			fmt.Fprintln(os.Stderr, "INDstocks token not set; use -tape mock for a plumbing check")
			os.Exit(2)
		}
		tp = &tape.INDstocks{Client: c, Dir: *barsDir, Ctx: ctx, Refresh: *refresh}
	default:
		tp = tape.Pick(ctx, *barsDir)
		if t, ok := tp.(*tape.INDstocks); ok {
			t.Refresh = *refresh
		}
	}

	now := time.Now()
	rep := backtest.RunOn(tp, *years, now)
	if rep.Status != "complete" {
		fmt.Fprintf(os.Stderr, "lab: %s — %s\n", rep.Status, rep.Note)
		os.Exit(1)
	}
	fmt.Printf("tape=%s days=%d folds=%d variants=%d promoted=%d nifty=%+.2f%%\n",
		rep.Tape, rep.TapeDays, rep.Folds, rep.VariantsTested, rep.VariantsPromoted, rep.NiftyReturnPct)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEXCESS%\tRET%\tTRADES\tWIN\tMAXDD%\tROSTER")
	for _, v := range rep.Variants {
		tag := ""
		if v.Promoted {
			tag = "live"
		}
		fmt.Fprintf(w, "%s\t%+.2f\t%+.2f\t%d\t%.0f%%\t%.1f\t%s\n",
			v.Spec.ID, v.ExcessPct, v.ReturnPct, v.Trades, v.WinRate*100, v.MaxDDPct, tag)
	}
	w.Flush()
	fmt.Println(rep.Note)

	if !*write {
		return
	}
	if !rep.Promotable() {
		fmt.Fprintln(os.Stderr, "refusing to write a roster from a mock tape")
		os.Exit(3)
	}
	snap := rep.Snapshot
	snap.GitSHA = gitSHA()
	path, err := backtest.SaveRoster(*rosterDir, snap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "roster: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s roster=%d added=%d sha=%s\n", path, len(snap.Roster), len(snap.Added), snap.GitSHA)
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	sha := strings.TrimSpace(string(out))
	// The roster file is this run's output; rewriting it must not mark the run dirty.
	st, err := exec.Command("git", "status", "--porcelain", "--", ".", ":(exclude)data/roster").Output()
	if err == nil && strings.TrimSpace(string(st)) != "" {
		sha += "-dirty"
	}
	return sha
}
