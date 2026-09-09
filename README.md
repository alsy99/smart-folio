# Aperture

India-only **paper-trading lab** for NSE cash equities. A Next.js desk (`apps/web`) talks HTTP/JSON to a Go **gateway**. Six backend services speak **gRPC**.

This is educational software. It is **not** financial advice. It does **not** guarantee beating Nifty, Sensex, or mutual funds by 10 percentage points — that number is a **measured target** on the dashboard.

## What it does

- 30-day paper campaign during NSE hours (09:15–15:30 IST), or `MARKET_CLOCK_OVERRIDE=open` for demos
- Multi-strategy book (momentum, SMA cross, mean-reversion, breakout, swing, ORB, sentiment tilt)
- Parallel **news investigations** (NewsAPI, Currents, Finnhub — mocked without keys) that tilt those strategies
- Learning journal after every closed fill (helped vs hurt Nifty → evolving weights)
- Benchmark table: Nifty 50, Nifty 500, Sensex, large-cap / flexi-cap MF **peer proxies**

Live INDstocks order routing is **off**. Optional quote keys can be added later.

## Run locally

Needs Go 1.22+ and Node 20+.

```bash
cp .env.example .env   # optional
make proto             # if you change proto/
make tidy
bash scripts/dev-backend.sh &   # gRPC 9081–9085 + gateway :8080
cd apps/web && npm install && npx next dev --turbopack -p 43127
```

Open [http://127.0.0.1:43127](http://127.0.0.1:43127). With `AUTOSTART_CAMPAIGN=true` (default in the script) the 30-day paper book starts on its own.

Docker Compose is provided (`docker compose up --build`) if you prefer containers.

## Environment

| Variable | Purpose |
|----------|---------|
| `MARKET_CLOCK_OVERRIDE=open` | Trade even after IST close (needed for most demos) |
| `AUTOSTART_CAMPAIGN=true` | Start the 30-day book when trading boots |
| `NEWSAPI_KEY` / `CURRENTS_API_KEY` / `FINNHUB_API_KEY` | Live headlines; otherwise mock India tape |
| `OPENAI_API_KEY` | Optional deeper investigation thesis + advisor |
| `NEXT_PUBLIC_API_URL` | Frontend → gateway (default `http://127.0.0.1:8080`) |

## Layout

```
apps/web          Next.js desk
services/*        gateway, marketdata, trading, learning, sentiment, advisor
proto/            gRPC contracts
pkg/              clock, prices, strategies, excess math
```

Service-to-service is gRPC only. The browser never sees proto, MPIN, or news API keys.
