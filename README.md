# Aperture

India-only **paper-trading lab** for NSE cash equities. A Next.js desk (`apps/web`) talks HTTP/JSON to a Go **gateway**. Six backend services speak **gRPC**.

This is educational software. It is **not** financial advice. It does **not** guarantee beating Nifty, Sensex, or mutual funds by 10 percentage points — that number is a **measured target** on the dashboard.

## What it does

- 30-day paper campaign: **fills only in NSE hours** (09:15–15:30 IST, weekdays). News, investigations, and strategy plans run **24×7**.
- Multi-strategy book (momentum, SMA cross, mean-reversion, breakout, swing, ORB, sentiment tilt)
- Parallel **news investigations** with LLM reasoning (fundamental card + technicals). Fills only in NSE hours; research runs 24×7.
- Learning journal after every closed fill (helped vs hurt Nifty → evolving weights)
- **5-year backtest lab** across methods and timeframes; winners are promoted into the live roster
- Benchmark table: Nifty 50, Nifty 500, Sensex, large-cap / flexi-cap MF **peer proxies**

Quotes and bars use the [INDstocks API](https://api-docs.indstocks.com/api-overview/) when `INDSTOCKS_ACCESS_TOKEN` is set. Fills stay on the **paper book**. Live order routing is off.

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

Open [http://127.0.0.1:43127](http://127.0.0.1:43127). With `AUTOSTART_CAMPAIGN=true` (default in the script) the 30-day paper book starts on its own.

Docker Compose is provided (`docker compose up --build`) if you prefer containers.

## Environment

| Variable | Purpose |
|----------|---------|
| `MARKET_CLOCK_OVERRIDE=open` | Force paper fills after IST close (off by default) |
| `AUTOSTART_CAMPAIGN=true` | Start the 30-day book when trading boots |
| `NEWSAPI_KEY` / `CURRENTS_API_KEY` / `FINNHUB_API_KEY` | Live headlines; otherwise mock India tape |
| `OPENAI_API_KEY` | Advisor via OpenAI if set |
| `GOOGLE_API_KEY` | Gemini; on 429 the desk tries Groq → Mistral → OpenRouter → NVIDIA → Z.AI |
| `GROQ_API_KEY` / `MISTRAL_API_KEY` / `OPENROUTER_API_KEY` / `NVIDIA_API_KEY` / `ZAI_API_KEY` | OpenAI-compatible fallbacks |
| `INVESTIGATION_LLM=true` | LLM reasoning on investigations and picked names (on by default; cap with `INVESTIGATION_LLM_MAX`) |
| `INDSTOCKS_ACCESS_TOKEN` or `INDMONEY_API_TOKEN` | INDstocks access token for live NSE quotes and history. Empty = mock tape. |
| `INDSTOCKS_API_KEY` / `INDSTOCKS_MPIN` / `INDSTOCKS_TOTP_SECRET` | Optional TOTP mint if you do not paste a dashboard token |
| `AUTOPILOT_LIVE_IND` | Ignored. Orders are never sent to INDstocks. |
| `NEXT_PUBLIC_API_URL` | Frontend → gateway (default `http://127.0.0.1:8080`) |

## Layout

```
apps/web          Next.js desk
cmd/              thin binaries (config, wiring, serve)
internal/         service implementations (not importable outside the module)
pkg/              shared libraries (strategies, prices, config, serve)
proto/            gRPC contracts
gen/              generated protobuf Go stubs
```

Service-to-service is gRPC only. The browser never sees proto, MPIN, or news API keys.

`cmd/` is wiring only (config, clients, graceful shutdown). Domain logic lives in `internal/`. Shared math and adapters live in `pkg/`. Strategies are a registry of interchangeable evaluators; news providers implement a common `Source` interface; the advisor LLM is injected behind a `Completer` so tests can stub it.
