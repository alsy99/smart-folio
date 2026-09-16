# Aperture strategy spec — core monthly rebalance, empty satellite

This is the **paper** book the product actually runs. It is not an exchange-filed algo specification, not a live-routing enablement, and not a promise of return. Live constants in `pkg/live` stay false; `pkg/indstocks.PlaceOrder` is compile-time off in the default binary. `AUTOPILOT_LIVE_IND` is ignored.

The published evidence is `campaign/public-30d-core/` (IPS A: 100% core). `campaign/public-30d/` is the historical cash month from before the IPS existed. Do not treat either as a live track record.

## Universe

Twelve NSE cash names, equal weight, the same set the public campaign already carries:

RELIANCE, TCS, HDFCBANK, INFY, ICICIBANK, SBIN, BHARTIARTL, ITC, LT, KOTAKBANK, AXISBANK, HINDUNILVR.

There is no Nifty ETF on the tape. This mix is a large-cap proxy with real tracking error against Nifty 50. Do not invent tickers. Weights are trimmed to the rails below; leftover stays cash (a 100% core is about 81% invested).

## Calendar

- Fills only 09:15–15:30 IST on weekdays that printed a NIFTY50 close on the tape.
- One mark per session at 15:30 IST (market-on-close). A 09:15 quote is the prior close.
- **Rebalance session** = last NSE session of the civil month (monthly IPS) or of the quarter (quarterly). Dates come from NIFTY50 candle dates, not a weekend calendar.
- Initial build is the first open session after the IPS is bound; it may take several sessions because new-buy notional is 10% of equity per IST day.
- Core calendar tickets skip only the cash-slice rule. Name, sector, gross, turnover, and halt still apply. Min-hold (3 sessions) is satellite-only; the core may trade on a calendar session.

## Drift band

A name trades on a rebalance session only if its weight is more than **100 bps** off target, or cash is more than **2%** of equity off target. Inside the band: hold.

## Cost model (`delivery-v1`)

Every ticket, core or satellite:

- Delivery STT 0.1% buy and 0.1% sell
- Exchange, SEBI, stamp (buy), GST on brokerage
- ₹10 brokerage per executed order
- Slippage from 20-day ADV (floor 2 bps, cap 40 bps)

There is no “index-like” path that skips costs.

## Rails (broker intent)

| Rail | Value |
|------|--------|
| Name | 8% of equity |
| Sector | 25% of equity |
| Gross | 90% of equity |
| Cash buffer | 10% of equity |
| New-buy notional / IST session | 10% of equity |
| Min hold (satellite) | 3 full cash sessions |

## Halt

Halt threshold = `min(0.15, ips.MaxDD)` on **total-equity** peak-to-trough.

When halted: **no new buys**. The core holds; it does not liquidate to cash. The log line is `MAX_DRAWDOWN` once for the book and `CORE_HALT` once for the sleeve. Halt lifts only if equity recovers above the cap (it did not, on the synthetic halt fixture).

Published halt path: `campaign/public-30d-halt/` (documented −20% shock from 1 Sep 2026 on every series; bars.json itself is the real tape).

## Satellite gate

Empty is valid.

A method trades only if it is on the dated roster snapshot (`data/roster/` or the campaign’s `roster.json`) with weight > 0. Failing defaults are published at weight 0, regime `failing-gate`. A headline cannot readmit them. Satellite notional ≤ `SatellitePct × equity` (v1 cap 20%). Sentiment tilt applies to that slice only, ±25%, as-of the bar, and is a no-op when the roster is empty.

The as-of 14 Aug 2026 roster admitted **none** of the six shipped defaults. The customer book is therefore core plus empty satellite. Fold-in (`FoldSatellite=true`, default) treats the unused satellite slice as core at rebalance; `campaign/public-30d-fold/` matches core 100% to the rupee.

Do not search `pkg/strategies` until incumbents clear the walk-forward gate (OOS excess vs Nifty **and** absolute return > 0 after costs, ≥30 OOS trades, max DD ≤ 15%, at most 2 admissions per weekly snapshot). Promotion on the mock tape does not count.

## Learning

Period table after each rebalance window and at campaign end: IPS id, sleeve, method, P&L after costs, excess vs the IPS benchmark and vs Nifty, max DD, fills. Weight updates **only** for satellite tags with n ≥ 20 periods or n ≥ 20 satellite fills and negative expectancy vs the IPS benchmark after costs. The core mix does not learn. Nothing auto-raises `SatellitePct`. A single losing fill does not move a weight.

## Disclosure

**+10 percentage points vs Nifty is a target, not a promise.** It is not a constraint in the allocator. Book A tracking Nifty minus costs is a pass. Do not curve-fit core weights until the ledger “wins.”

## Kill switch

A human can stop the book without a deploy:

1. Desk **Stop** (autopilot off).
2. `touch data/KILL` — `pkg/live.Killed()` trips; live adapters refuse. Clear the file to resume.

Paper fills also refuse when the market clock is closed, unless `MARKET_CLOCK_OVERRIDE=open` (demo only).

## What this file is not

- Not an NSE / SEBI algo filing.
- Not a live-order enablement. Do not set `pkg/live` constants true from this document.
- Not a hidden `LIVE=1` switch. There is none.
- Not authority to call INDstocks order endpoints. Those stay compile-off (`-tags liveorders` is unused by `cmd/`).
