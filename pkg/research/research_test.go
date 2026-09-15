package research

import "testing"

func TestParseJSONConclusion(t *testing.T) {
	raw := "```json\n{\"stance\":\"bullish\",\"horizon\":\"swing\",\"stand_aside\":false,\"fundamental\":\"IT services cash conversion.\",\"technical\":\"SMA5 above SMA20.\",\"conclusion\":\"Paper book can lean long vs Nifty on dips.\",\"risks\":\"Guidance cut.\"}\n```"
	got := parse(raw, Note{Symbol: "TCS", Conclusion: "fallback"})
	if got.Stance != "bullish" || got.Horizon != "swing" {
		t.Fatalf("%+v", got)
	}
	if got.Conclusion != "Paper book can lean long vs Nifty on dips." {
		t.Fatalf("conclusion %q", got.Conclusion)
	}
}

func TestHeuristicHasTAAndFA(t *testing.T) {
	n := Heuristic(Input{Symbol: "TCS", StrategyID: "momentum_1d", Score: 0.4, Direction: 1})
	if n.Technical == "" || n.Fundamental == "" {
		t.Fatalf("%+v", n)
	}
	if n.Mode != "heuristic" {
		t.Fatalf("mode %s", n.Mode)
	}
}
