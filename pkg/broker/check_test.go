package broker

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"aperture/pkg/costs"
)

func cashBook(eq float64) Snapshot {
	return Snapshot{Equity: eq, Peak: eq, Cash: eq, Held: map[string]float64{}, Sector: map[string]float64{}}
}

func TestNameGrossCashSectorCaps(t *testing.T) {
	s := cashBook(1_000_000)
	if got := Room(s, "TCS"); got != 80_000 {
		t.Fatalf("empty book room should be 8%% name = 80k, got %.0f", got)
	}
	// 25% of cash would be 250k — too fat for a 10-name book.
	if Room(s, "TCS") >= s.Cash*0.25 {
		t.Fatal("cash slice must be tighter than 25% of cash")
	}

	s.Held["TCS"] = 80_000
	s.Gross = 80_000
	s.Cash = 920_000
	s.Sector["IT"] = 80_000
	if Room(s, "TCS") != 0 {
		t.Fatal("name cap must refuse an add")
	}

	s = cashBook(1_000_000)
	s.Gross = 900_000
	s.Cash = 100_000
	if Room(s, "RELIANCE") != 0 {
		t.Fatal("gross cap 90% leaves no room")
	}

	s = cashBook(1_000_000)
	s.Cash = 100_000
	s.Gross = 900_000
	if d := Check(Intent{Symbol: "ITC", Side: Buy, Qty: 20, Price: 500}, s); d.Allow || d.Reason != ReasonGrossCap && d.Reason != ReasonCashBuffer {
		t.Fatalf("expected gross/cash deny, got %+v", d)
	}

	s = cashBook(1_000_000)
	s.Sector["Banks"] = 250_000
	s.Gross = 250_000
	s.Cash = 750_000
	s.Held["HDFCBANK"] = 80_000
	d := Check(Intent{Symbol: "SBIN", Side: Buy, Qty: 10, Price: 800}, s)
	if d.Allow || d.Reason != ReasonSectorCap {
		t.Fatalf("sector cap, got %+v", d)
	}
}

func TestTurnoverCapIsABrokerRail(t *testing.T) {
	s := cashBook(1_000_000)
	if got := TurnoverRoom(s); got != 100_000 {
		t.Fatalf("fresh session room should be %.0f%% of equity = 100k, got %.0f", costs.TurnoverCapDay*100, got)
	}
	// Room folds the turnover rail in: a name cap of 80k stays, but once 60k is
	// bought this session only 40k is left for the next ticket.
	s.DayBuys = 60_000
	if got := Room(s, "TCS"); got < 39_999.99 || got > 40_000.01 {
		t.Fatalf("room after 60k of session buys should be 40k, got %.0f", got)
	}
	d := Check(Intent{Symbol: "TCS", Side: Buy, Qty: 50, Price: 1000}, s)
	if d.Allow || d.Reason != ReasonTurnover {
		t.Fatalf("50k ticket past the session cap must deny TURNOVER_CAP, got %+v", d)
	}
	if d := Check(Intent{Symbol: "TCS", Side: Buy, Qty: 30, Price: 1000}, s); !d.Allow {
		t.Fatalf("30k inside the remaining 40k must pass, got %+v", d)
	}
	s.DayBuys = 100_000
	if d := Check(Intent{Symbol: "TCS", Side: Sell, Qty: 50, Price: 1000}, s); !d.Allow {
		t.Fatal("sells never count against the buy turnover cap")
	}
}

func TestMinHoldIsABrokerRail(t *testing.T) {
	min := costs.MinHoldSessions
	if d := MayExit(min-1, 48*time.Hour); d.Allow || d.Reason != ReasonMinHold {
		t.Fatalf("%d sessions must not pass a %d-session min hold: %+v", min-1, min, d)
	}
	if d := MayExit(min, costs.SessionHold-time.Minute); d.Allow {
		t.Fatal("under one cash session never exits on a flip")
	}
	if d := MayExit(min, 72*time.Hour); !d.Allow {
		t.Fatalf("%d sessions clears the rail: %+v", min, d)
	}
	if d := MayExit(0, costs.MaxHold); !d.Allow {
		t.Fatal("MaxHold recycles the name regardless of sessions")
	}
}

func TestNoAddWhenHaltedSellsStillPass(t *testing.T) {
	s := Snapshot{Equity: 840_000, Peak: 1_000_000, Cash: 840_000, Held: map[string]float64{"TCS": 0}}
	buy := Check(Intent{Symbol: "TCS", Side: Buy, Qty: 10, Price: 1000}, s)
	if buy.Allow || buy.Reason != ReasonMaxDrawdown {
		t.Fatalf("halted book must refuse buys: %+v", buy)
	}
	sell := Check(Intent{Symbol: "TCS", Side: Sell, Qty: 10, Price: 1000}, s)
	if !sell.Allow {
		t.Fatal("sells must still pass the halt")
	}
}

func TestPeakToTroughNotStart(t *testing.T) {
	// Book is up from 1L start, then dumps 15% off the peak.
	s := Snapshot{Equity: 1_020_000, Peak: 1_200_000, Cash: 1_020_000}
	if !Halted(s) {
		t.Fatal("15% off peak must halt even if still above start")
	}
	if costs.BookHalted(1_020_000, 1_000_000) {
		t.Fatal("start-based halt would be wrong here")
	}
}

func TestSubmitLiveAdapterRefusesUncheckedBuy(t *testing.T) {
	routed := false
	err := Submit(context.Background(),
		Snapshot{Equity: 840_000, Peak: 1_000_000, Cash: 840_000},
		Intent{Symbol: "TCS", Side: Buy, Qty: 1, Price: 3000},
		func(context.Context, Intent) error { routed = true; return nil },
	)
	if err == nil || routed {
		t.Fatal("live router must not see a halted buy")
	}
	if !strings.Contains(err.Error(), ReasonMaxDrawdown) {
		t.Fatalf("err %v", err)
	}
}

func TestSynthetic16PctPathRefusesBuysLogsMAX_DRAWDOWNOnce(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	p := NewPaper(log)
	snap := Snapshot{Equity: 840_000, Peak: 1_000_000, Cash: 840_000, Held: map[string]float64{}, Sector: map[string]float64{}}
	if Drawdown(snap) < 0.16-1e-9 {
		t.Fatalf("fixture must be a −16%% path, got %.4f", Drawdown(snap))
	}
	in := Intent{Symbol: "TCS", Side: Buy, Qty: 10, Price: 3000}
	d1 := p.Admit(in, snap)
	d2 := p.Admit(in, snap)
	if d1.Allow || d2.Allow {
		t.Fatalf("−16%% must refuse new buys: %+v %+v", d1, d2)
	}
	if d1.Reason != ReasonMaxDrawdown {
		t.Fatalf("reason %s", d1.Reason)
	}
	n := strings.Count(buf.String(), ReasonMaxDrawdown)
	if n != 1 {
		t.Fatalf("MAX_DRAWDOWN must log once, got %d in %q", n, buf.String())
	}
}
