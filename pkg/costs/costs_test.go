package costs

import "testing"

func TestFillsAndCommission(t *testing.T) {
	if BuyFill(100) <= 100 {
		t.Fatal("buy fill should include slippage")
	}
	if SellFill(100) >= 100 {
		t.Fatal("sell fill should include slippage")
	}
	if Commission(100_000) != 50 {
		t.Fatalf("5bps of 100k should be 50, got %v", Commission(100_000))
	}
}
