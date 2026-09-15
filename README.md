# Aperture

**Aperture** is an India-only positional cash paper book for NSE equities. This git repository is named `smart-folio`; the product, Go module (`aperture`), and desk title are Aperture.

A Next.js desk (`apps/web`) talks HTTP/JSON to a Go **gateway**. Five backend services speak **gRPC**. Licensed under **Apache-2.0**.

This is educational software. It is **not** financial advice. It does **not** guarantee beating Nifty, Sensex, or mutual funds by 10 percentage points — that number is a **measured target** on the dashboard.

## What it does

- 30-day paper campaign: **fills only in NSE hours** (09:15–15:30 IST, weekdays). News, investigations, and strategy plans run **24×7**.
- **One shipped roster**: daily-bar methods (SMA, momentum, mean-reversion, breakout, swing, sentiment tilt). Holds are **days to weeks** — never under one cash session.
- **Risk rails at the broker-intent gate** (paper and any live adapter): 8% name cap, 90% gross, 10% cash buffer, 25% sector, **15% peak-to-trough halt** (`MAX_DRAWDOWN`, logged once). No add when halted. Ticket size is 8% of cash — not a 25% cash slice.
- Parallel **news investigations** with LLM reasoning (fundamental card + technicals). Headlines, cards, and corporate actions are **as-of the bar** — `published_at > bar_time` is rejected. Investigations are **queued**, cached by `(symbol, day, headline-hash)`, and hard-capped by `INVESTIGATION_LLM_MAX` and `INVESTIGATION_TOKENS_DAY`. The model is pinned for the IST day (fallback chain only before the first success). An LLM cannot emit an order; every fill row points at exactly one investigation id with prompt/model/tokens/output persisted beside it.
- When `INDSTOCKS_ACCESS_TOKEN` is empty the desk shows a **red MOCK TAPE** banner. Mock PnL is never presented as live NSE.
- Learning journal after every closed fill (helped vs hurt Nifty). Closes store strategy, regime, PnL after costs, excess vs Nifty, hold, MAE/MFE. **Weights move only on the weekly job** (shrink tags with negative expectancy and n ≥ 20). `make review` prints expectancy by method.
- **5-year walk-forward backtest lab** on daily/weekly frames. Promotion requires **out-of-sample** excess vs Nifty net of delivery costs, ≥30 OOS round trips, and max drawdown under the 15% book cap. At most **2 new admissions per weekly snapshot**.
- The live roster is a **dated snapshot file** (`data/roster/YYYY-MM-DD.json`) — services boot from the latest file, never from an in-memory winner.
- Benchmark table: Nifty 50, Nifty 500, Sensex, large-cap / flexi-cap MF **peer proxies**

Quotes and bars use the [INDstocks API](https://api-docs.indstocks.com/api-overview/) when `INDSTOCKS_ACCESS_TOKEN` is set. Fills stay on the **paper book**. Live order routing is **compile-time off** (`-tags liveorders` is not used by `cmd/`). `AUTOPILOT_LIVE_IND` is ignored. There is no environment flag that enables live routing.

## Public 30-day paper campaign

Frozen ledger: `campaign/public-30d/ledger.json`. Same git SHA, settings, universe, and delivery cost model. Window **17 Aug 2026 09:15 IST → 16 Sep 2026 09:15 IST**. Daily prints: equity, excess vs Nifty 50 / Nifty 500 / Sensex, drawdown, turnover, fills, halted-or-not. Restart and weight edits are refused while the ledger is frozen.

```bash
go run ./cmd/campaign -verify
# or
go test ./internal/campaign -count=1
```

A stranger who clones that SHA must match the published equity line within ₹1. The tape is the deterministic mock (`pkg/prices`); LLM off; equal frozen weights. Do not edit parameters mid-campaign.

Live exchange orders stay off until all of these exist: a written (exchange-filed) strategy spec, a broker principal path, Algo-ID tagging, a static IP for order endpoints, an order-rate cap under the exchange threshold, a kill switch a human can hit (Stop Autopilot / `data/KILL`), and the disclosure that **+10pp vs Nifty is a target, not a promise**. Selling access to others is a different license.

## Run locally

Needs Go 1.22+ and Node 20+.

```bash
cp .env.example .env   # optional
make proto             # if you change proto/
make tidy
make test
bash scripts/dev-backend.sh &   # gRPC 9081–9085 + gateway :8080
cd apps/web && npm install && npx next dev --turbopack -p 43127
```

Open [http://127.0.0.1:43127](http://127.0.0.1:43127). With `AUTOSTART_CAMPAIGN=true` (default in the script) the 30-day paper book starts on its own. A missing `INDSTOCKS_ACCESS_TOKEN` shows **MOCK** on the hero — not a pretty fake +8%.

`docker compose up --build` boots the desk plus marketdata, trading, learning, sentiment, and advisor (gateway in front). Do not put API keys in `NEXT_PUBLIC_*`.

## Environment

| Variable | Purpose |
|----------|---------|
| `MARKET_CLOCK_OVERRIDE=open` | Force paper fills after IST close (off by default) |
| `SCALP_MODE=true` | Opt-in 45s / +1.2% / −0.8% exits. Default `false` is positional cash. |
| `AUTOSTART_CAMPAIGN=true` | Start the 30-day book when trading boots |
| `NEWSAPI_KEY` / `CURRENTS_API_KEY` / `FINNHUB_API_KEY` | Live headlines; otherwise mock India tape |
| `OPENAI_API_KEY` | Advisor via OpenAI if set |
| `GOOGLE_API_KEY` | Gemini; on 429 the desk tries Groq → Mistral → OpenRouter → NVIDIA → Z.AI |
| `GROQ_API_KEY` / `MISTRAL_API_KEY` / `OPENROUTER_API_KEY` / `NVIDIA_API_KEY` / `ZAI_API_KEY` | OpenAI-compatible fallbacks |
| `INVESTIGATION_LLM=true` | LLM reasoning on investigations (on by default) |
| `INVESTIGATION_LLM_MAX` | Hard cap on LLM calls per IST day (default 5) |
| `INVESTIGATION_TOKENS_DAY` | Hard cap on the day’s investigation token bill (default 50000) |
| `INDSTOCKS_ACCESS_TOKEN` or `INDMONEY_API_TOKEN` | INDstocks access token for live NSE quotes and history. Empty = mock tape. |
| `INDSTOCKS_API_KEY` / `INDSTOCKS_MPIN` / `INDSTOCKS_TOTP_SECRET` | Optional TOTP mint if you do not paste a dashboard token |
| `AUTOPILOT_LIVE_IND` | Ignored. Orders are never sent to INDstocks. |
| `NEXT_PUBLIC_API_URL` | Frontend → gateway (default `http://127.0.0.1:8080`). Never an API key. |
| `GATEWAY_RPS` / `GATEWAY_BURST` | Gateway token-bucket rate limit (default 40/s, burst 80) |
| `LOG_FORMAT` | `json` (default) or `text` |

## Layout

```
apps/web          Next.js desk
campaign/         frozen public 30-day paper ledger
cmd/              thin binaries (config, wiring, serve)
LICENSE           Apache-2.0
internal/         service implementations (not importable outside the module)
pkg/              shared libraries (strategies, prices, config, serve)
proto/            gRPC contracts
gen/              generated protobuf Go stubs
```

Service-to-service is gRPC only. The browser never sees proto, MPIN, or news API keys.

`cmd/` is wiring only (config, clients, graceful shutdown). Domain logic lives in `internal/`. Shared math and adapters live in `pkg/`. Strategies are a registry of interchangeable evaluators; news providers implement a common `Source` interface; the advisor LLM is injected behind a `Completer` so tests can stub it.
