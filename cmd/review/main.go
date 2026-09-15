package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"aperture/pkg/config"
	"aperture/pkg/learn"
	"aperture/pkg/strategies"
)

func main() {
	reweight := flag.Bool("reweight", false, "run the scheduled review if it is due (never on a single fill or period)")
	force := flag.Bool("force", false, "run the review now, still requiring n≥20 to move a weight")
	flag.Parse()
	dir := config.String("LEARNING_DIR", "data/learning")
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}
	closes, err := learn.LoadCloses(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "closes: %v\n", err)
		os.Exit(1)
	}
	periods, err := learn.LoadPeriods(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "periods: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(learn.FormatTable(learn.ByMethod(closes)))
	fmt.Println()
	fmt.Println("periods by IPS and sleeve (core is reported, never reweighted):")
	fmt.Print(learn.FormatPeriodTable(learn.ByIPSSleeve(periods)))
	if !*reweight && !*force {
		return
	}
	ids := strategies.IDs()
	prev, err := learn.LoadSnapshot(dir)
	if err != nil {
		prev = learn.EqualSnapshot(ids, time.Now().UTC())
	}
	snap := learn.ApplyReview(ids, prev, closes, periods, time.Now().UTC(), *force)
	if err := learn.SaveSnapshot(dir, snap); err != nil {
		fmt.Fprintf(os.Stderr, "weights: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s moved=%v shrunk=%v\n", snap.Note, snap.Moved, snap.Shrunk)
}
