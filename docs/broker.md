# Broker conversation

This is the packet for a principal / Algo-ID conversation. It is not an order.

**Attach:** [`docs/spec.md`](spec.md) — core monthly rebalance, empty satellite, delivery cost model, halt = no new buys (marks can still go through 15%), kill switch.

**Do not attach or implement:** `pkg/indstocks/orders_on.go`, `-tags liveorders`, `AUTOPILOT_LIVE_IND`, or any hidden enable switch. Live `PlaceOrder` stays compile-off until the checklist in `pkg/live` is actually met, and flipping those constants is a human + broker decision, not a code path from this file.

Ask them:

1. Principal path (who is the client, who is the algo).
2. Algo-ID tagging on every ticket.
3. Static IP for order endpoints.
4. Order-rate cap under the exchange threshold.
5. How they want halt represented: today’s hold (no new buys) vs Phase 8 flatten-on-halt (costs and taxes first, then a *new* ledger — not a rewrite of `public-30d-halt`).

Published paper evidence to point at, all frozen: `campaign/public-30d/` (cash), `public-30d-core/` (A), `public-30d-fold/` (B), `public-30d-halt/` (C, hold). Next book after 16 Oct 2026 is `public-30d-core-2`, same IPS A, new directory.
