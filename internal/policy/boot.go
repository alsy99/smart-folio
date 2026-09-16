package policy

import (
	"aperture/pkg/ips"
)

// Boot loads the bound statement for the live book. Prefer the named id
// (IPS_ID), then the persisted BOUND pointer, then the sole file on disk,
// then IPS A — created and stored so Autostart never buys into a blank book.
func Boot(store Store, preferID string, startCash float64) (ips.IPS, error) {
	if store == nil {
		store = NewMemStore()
	}
	if preferID != "" {
		if p, err := store.Get(preferID); err == nil {
			_ = store.SetBound(p.ID)
			return p, nil
		}
	}
	if p, err := store.Bound(); err == nil {
		return p, nil
	}
	all, err := store.List()
	if err != nil {
		return ips.IPS{}, err
	}
	if len(all) == 1 {
		_ = store.SetBound(all[0].ID)
		return all[0], nil
	}
	a := ips.A(startCash)
	if p, err := store.Get(a.ID); err == nil {
		_ = store.SetBound(p.ID)
		return p, nil
	}
	if err := store.Put(a); err != nil {
		return ips.IPS{}, err
	}
	if err := store.SetBound(a.ID); err != nil {
		return ips.IPS{}, err
	}
	return a, nil
}
