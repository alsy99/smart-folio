package costs

import (
	"math"
	"testing"
)

func TestADVSlippageScales(t *testing.T) {
	flat := ADVSlippageBps(0)
	if flat != SlippageBps {
		t.Fatalf("no-history should fall back to flat %.1f, got %.2f", SlippageBps, flat)
	}
	reliance := ADVSlippageBps(7.5e9) // ≈ RELIANCE 20-day ADV in ₹
	if math.Abs(reliance-5.65) > 1.0 {
		t.Fatalf("RELIANCE-scale ADV should be near 4 bps, got %.2f", reliance)
	}
	thin := ADVSlippageBps(1e8)
	if thin <= reliance {
		t.Fatalf("thin ADV %.1e should slip more than RELIANCE, got %.2f vs %.2f", 1e8, thin, reliance)
	}
	if got := ADVSlippageBps(1e4); got != ADVMaxBps {
		t.Fatalf("tiny ADV should cap at %.0f, got %.2f", ADVMaxBps, got)
	}
}

// 1-lot RELIANCE round-trip on 2024-06-21 (NIFTY expiry Friday).
// Contract-note math for a delivery cash trade, INDstocks tariff:
//
//	buy 1 × ₹2,905.00, sell 1 × ₹2,920.00, ADV ≈ ₹7.5e9 → slippage ≈ 4 bps.
func TestRelianceRoundTripContractNote(t *testing.T) {
	qty := 1.0
	buyMark := 2905.00
	sellMark := 2920.00
	adv := 7.5e9

	buyFill := BuyFill(buyMark, qty, adv)
	sellProceeds := func() float64 {
		f, p := SellFill(sellMark, qty, adv)
		_ = f
		return p
	}()

	c := RoundTrip(qty*buyFill, sellProceeds, adv)

	// --- buy leg ---
	// STT 0.1% on 2906.16 ≈ ₹2.9062
	wantBuySTT := 2906.162 * 0.1 / 100
	if math.Abs(c.BuyLines[0].Amt-wantBuySTT) > 0.01 {
		t.Fatalf("buy STT: got ₹%.4f want ≈₹%.4f", c.BuyLines[0].Amt, wantBuySTT)
	}
	// stamp duty 0.015% on buy ≈ ₹0.4359
	wantStamp := 2906.162 * 0.015 / 100
	if math.Abs(c.BuyLines[3].Amt-wantStamp) > 0.001 {
		t.Fatalf("stamp duty: got ₹%.4f want ≈₹%.4f", c.BuyLines[3].Amt, wantStamp)
	}
	// brokerage is flat ₹10 per order
	if c.Brokerage != 20.0 {
		t.Fatalf("brokerage should be 2×₹10 = ₹20, got ₹%.2f", c.Brokerage)
	}
	// GST 18% on (brokerage + exchange + SEBI) — both legs ≈ ₹3.60 total
	wantGST := (10 + 2906.162*0.0297/1e4 + 2906.162*0.0001/1e4 +
		10 + sellProceeds*0.0297/1e4 + sellProceeds*0.0001/1e4) * 0.18
	if math.Abs(c.GST-wantGST) > 0.01 {
		t.Fatalf("GST: got ₹%.4f want ≈₹%.4f", c.GST, wantGST)
	}
	// statutory = STT + stamp + SEBI + exchange
	if c.Statutory < 6.0 || c.Statutory > 6.5 {
		t.Fatalf("statutory out of range: ₹%.4f", c.Statutory)
	}
	// total charges (excl. slippage) ≈ ₹29.9 — within a few rupees of any
	// discount-broker contract note for this trade.
	if c.Total < 29.0 || c.Total > 30.5 {
		t.Fatalf("round-trip charges ₹%.4f outside contract-note band [29, 30.5]", c.Total)
	}
	// net cost including slippage ≈ ₹33.1
	if c.NetCost < 32.0 || c.NetCost > 34.5 {
		t.Fatalf("net cost ₹%.4f outside [32, 34.5]", c.NetCost)
	}
	// journal ledger line must carry every charge name for the desk.
	for _, name := range []string{"STT", "StampDuty", "Exchange", "SEBI", "Brokerage", "GST"} {
		if !containsLine(c.BuyLines, name) && !containsLine(c.SellLines, name) {
			t.Fatalf("missing charge line %s in %+v", name, c)
		}
	}
	if c.Slippage <= 0 {
		t.Fatalf("slippage should be tracked, got ₹%.2f", c.Slippage)
	}
}

func containsLine(ls []Line, name string) bool {
	for _, l := range ls {
		if l.Name == name {
			return true
		}
	}
	return false
}

func TestNameRoomAndBookHalt(t *testing.T) {
	if NameRoom(1e6, 8e4) != 0 {
		t.Fatal("8% cap on 1L equity with 80k held leaves no room")
	}
	if NameRoom(1e6, 7e4) != 1e4 {
		t.Fatalf("room should be 10k, got %.0f", NameRoom(1e6, 7e4))
	}
	if !BookHalted(8.5e5, 1e6) {
		t.Fatal("15% drawdown halt should trigger")
	}
	if BookHalted(8.6e5, 1e6) {
		t.Fatal("14% drawdown should not halt")
	}
}
