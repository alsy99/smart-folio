package costs

const (
	StartCash     = 1_000_000.0
	CommissionBps = 5.0
	SlippageBps   = 4.0
)

func BuyFill(px float64) float64 {
	return px * (1 + SlippageBps/1e4)
}

func SellFill(px float64) float64 {
	return px * (1 - SlippageBps/1e4)
}

func Commission(notional float64) float64 {
	return notional * CommissionBps / 1e4
}
