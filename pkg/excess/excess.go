package excess

import "math"

const TargetPP = 0.10

func PortfolioReturn(equity, start float64) float64 {
	if start == 0 {
		return 0
	}
	return equity/start - 1
}

func Excess(portfolioRet, benchRet float64) float64 {
	return portfolioRet - benchRet
}

func Annualized(excess float64, days float64) float64 {
	if days <= 0 {
		return 0
	}
	return math.Pow(1+excess, 365.0/days) - 1
}

func OnTrack(excessAnn float64) bool {
	return excessAnn >= TargetPP
}
