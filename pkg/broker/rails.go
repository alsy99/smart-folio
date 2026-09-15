package broker

import "aperture/pkg/costs"

// Rails for a 10-name NSE cash book. Name cap and the 15% halt are the
// published costs constants so backtest, paper, and a live adapter cannot drift.
const (
	GrossCap   = 0.90          // long notional ≤ 90% of equity
	CashBuffer = 0.10          // cash ≥ 10% of equity
	SectorCap  = 0.25          // one GICS-like sector ≤ 25% of equity
	CashSlice  = costs.NameCap // ≤ 8% of cash on one ticket — not 25%
)

const (
	ReasonMaxDrawdown = "MAX_DRAWDOWN"
	ReasonNameCap     = "NAME_CAP"
	ReasonGrossCap    = "GROSS_CAP"
	ReasonCashBuffer  = "CASH_BUFFER"
	ReasonSectorCap   = "SECTOR_CAP"
	ReasonMinNotional = "MIN_NOTIONAL"
	ReasonTurnover    = "TURNOVER_CAP"
	ReasonMinHold     = "MIN_HOLD"
	ReasonEmpty       = "EMPTY"
)
