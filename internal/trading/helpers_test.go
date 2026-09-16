package trading

import (
	"time"

	"aperture/internal/policy"
	"aperture/pkg/costs"
	"aperture/pkg/ips"
)

// paperSatIPS is a valid statement with satellite room so satellite-fill
// tests can bind an IPS without a 100% core crowding out the sleeve.
func paperSatIPS() ips.IPS {
	p := ips.Default("c-1", costs.StartCash)
	p.CorePct, p.SatellitePct = 0.80, 0.20
	return p
}

// holdCoreCalendar marks today's session as already rebalanced so an
// Execute under an IPS does not also fire CORE_INITIAL.
func holdCoreCalendar(svc *Service, now time.Time) {
	svc.mu.Lock()
	svc.coreLastRebal = policy.SessionDate(now)
	svc.mu.Unlock()
}
