"use client";

import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { AdvisorTooltip } from "@/components/advisor-tooltip";
import { Empty, Hero } from "@/components/desk/hero";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useDesk } from "@/hooks/use-desk";
import { inr, pct } from "@/lib/utils";

export default function Home() {
  const {
    err,
    loading,
    busy,
    btBusy,
    portfolio,
    campaign,
    benchmarks,
    journal,
    investigations,
    news,
    picks,
    sentiment,
    history,
    backtest,
    nifty,
    onTrack,
    positions,
    start,
    kill,
    resume,
    runBacktest,
  } = useDesk();

  return (
    <div className="mx-auto max-w-[1400px] px-4 py-6 md:px-8 md:py-10">
      <header className="mb-8 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.28em] text-teal-800">India paper desk · NSE</p>
          <h1 className="font-[family-name:var(--font-display)] text-4xl md:text-5xl leading-[1.05] text-stone-900">
            Aperture
          </h1>
          <p className="mt-2 max-w-xl text-sm text-stone-600">
            Parallel news investigations and strategy plans run around the clock. Paper fills
            only during NSE hours (09:15–15:30 IST). Excess versus Nifty 50, Nifty 500, Sensex,
            and top-fund peers is a target, not a promise.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <AdvisorTooltip sentiment={sentiment} />
          <Button variant="outline" onClick={runBacktest} disabled={btBusy}>
            {btBusy ? "Backtesting 5y…" : "Run 5-year backtest"}
          </Button>
          <Button onClick={start} disabled={busy}>
            {campaign?.active ? "Restart 30-day campaign" : "Start 30-day campaign"}
          </Button>
          {campaign?.autopilot ? (
            <Button variant="danger" onClick={kill} disabled={busy}>
              Kill switch
            </Button>
          ) : (
            <Button variant="outline" onClick={resume} disabled={busy || !campaign?.active}>
              Resume autopilot
            </Button>
          )}
        </div>
      </header>

      {err && (
        <div className="mb-6 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-900">
          {err}. Start the Go stack (`make dev-backend`) so the gateway is on :8080.
        </div>
      )}

      {loading && !portfolio && (
        <p className="mb-6 text-sm text-stone-500">Connecting to the desk…</p>
      )}

      <section className="mb-6 grid gap-4 md:grid-cols-4">
        <Hero
          label="Paper equity"
          value={portfolio ? inr(portfolio.equity) : "—"}
          sub={portfolio ? `cash ${inr(portfolio.cash)}` : "awaiting campaign"}
        />
        <Hero
          label="vs Nifty 50"
          value={nifty ? pct(nifty.excessPct) : "—"}
          sub={nifty ? `ann. run-rate ${pct(nifty.excessAnnPct)}` : "start a campaign"}
          tone={nifty && nifty.excessPct >= 0 ? "good" : "bad"}
        />
        <Hero
          label="+10pp track"
          value={onTrack ? "On track" : "Off pace"}
          sub={`beat rate ${nifty ? Math.round((nifty.beatRate || 0) * 100) : 0}% of ticks`}
          tone={onTrack ? "good" : "warn"}
        />
        <Hero
          label="Session"
          value={campaign?.sessionStatus || "unknown"}
          sub={
            campaign
              ? `day ${campaign.daysElapsed}/${campaign.daysTotal} · ${campaign.ticks} ticks · ${campaign.autopilot ? "autopilot on" : "paused"}`
              : "idle"
          }
        />
      </section>

      <div className="grid gap-5 lg:grid-cols-[1.4fr_0.8fr]">
        <Card>
          <CardHeader className="flex flex-row items-start justify-between gap-3">
            <div>
              <CardTitle>Book versus Indian tape</CardTitle>
              <p className="text-xs text-stone-500">Equity overlay vs Nifty mock path this session</p>
            </div>
            <Badge tone={campaign?.clockOverride ? "warn" : campaign?.marketOpen ? "teal" : "warn"}>
              {campaign?.clockOverride
                ? "clock override"
                : campaign?.marketOpen
                  ? "NSE open · fills on"
                  : "NSE closed · research on"}
            </Badge>
          </CardHeader>
          <CardContent className="h-64">
            {history.length < 2 ? (
              <Empty>Paper fills land during NSE hours (09:15–15:30 IST). Research still runs overnight.</Empty>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={history}>
                  <CartesianGrid stroke="#e7e0d4" />
                  <XAxis dataKey="t" tick={{ fontSize: 11 }} />
                  <YAxis tick={{ fontSize: 11 }} />
                  <Tooltip />
                  <Area type="monotone" dataKey="equity" stroke="#0f766e" fill="#99f6e4" fillOpacity={0.45} />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Peer scoreboard</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {!benchmarks?.benchmarks?.length ? (
              <Empty>No benchmarks yet.</Empty>
            ) : (
              benchmarks.benchmarks.map((b) => (
                <div key={b.id} className="flex items-center justify-between gap-2 border-b border-stone-100 py-2 last:border-0">
                  <div>
                    <p className="text-sm font-medium">{b.name}</p>
                    <p className="text-[11px] text-stone-500">{b.id}</p>
                  </div>
                  <div className="text-right">
                    <p className={`num text-sm ${b.excessPct >= 0 ? "text-emerald-800" : "text-rose-800"}`}>
                      {pct(b.excessPct)}
                    </p>
                    <Badge tone={b.onTrack ? "good" : "neutral"}>
                      {b.onTrack ? "≥10pp ann." : "behind 10pp"}
                    </Badge>
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5 grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Parallel investigations</CardTitle>
              <p className="text-xs text-stone-500">
                {investigations
                  ? `${investigations.reports?.length || 0} reports · workers max ${investigations.workersMax} · ${investigations.mode}`
                  : "loading"}
              </p>
            </div>
          </CardHeader>
          <CardContent className="max-h-[420px] space-y-3 overflow-auto">
            {!investigations?.reports?.length ? (
              <Empty>No material clusters yet — mock tape appears after the sentiment service boots.</Empty>
            ) : (
              investigations.reports.map((r, i) => (
                <article key={`${r.id}-${i}`} className="rounded-xl border border-stone-200 bg-stone-50/70 p-3">
                  <div className="mb-1 flex flex-wrap items-center gap-2">
                    <Badge tone={r.stance === "bullish" ? "good" : r.stance === "bearish" ? "bad" : "neutral"}>
                      {r.stance}
                    </Badge>
                    <Badge tone="teal">{r.eventType}</Badge>
                    <Badge>{r.corroboration} sources</Badge>
                    {r.standAside && <Badge tone="warn">stand aside</Badge>}
                  </div>
                  <p className="text-sm font-medium text-stone-900">{r.headline}</p>
                  <p className="mt-1 whitespace-pre-wrap text-xs text-stone-600">{r.thesis}</p>
                  {r.risks && <p className="mt-1 text-[11px] text-stone-500">{r.risks}</p>}
                  <p className="mt-1 text-[11px] text-stone-500">
                    {(r.symbols || []).join(", ")} · {r.horizon} · tilts{" "}
                    {(r.strategyImplications || [])
                      .slice(0, 3)
                      .map((t) => t.strategyId)
                      .join(", ") || "none"}
                  </p>
                </article>
              ))
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Strategy weights</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {(portfolio?.weights || []).length === 0 ? (
              <Empty>Weights appear once learning has a prior (equal at start).</Empty>
            ) : (
              (portfolio?.weights || []).map((w) => (
                <div key={w.strategyId}>
                  <div className="mb-1 flex justify-between text-xs">
                    <span>{w.strategyId}</span>
                    <span className="num">{(w.weight * 100).toFixed(1)}%</span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-stone-200">
                    <div className="h-full bg-teal-700" style={{ width: `${Math.min(100, w.weight * 100)}%` }} />
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5">
        <Card>
          <CardHeader>
            <CardTitle>Pick research</CardTitle>
            <p className="text-xs text-stone-500">
              Fundamental card plus computed technicals, with LLM reasoning when a name is selected for the book.
            </p>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-2">
            {!picks.length ? (
              <Empty>Waiting for the planner to select names (signal score ≥ 0.25).</Empty>
            ) : (
              picks.map((p) => (
                <article key={p.symbol} className="rounded-xl border border-stone-200 bg-stone-50/70 p-3">
                  <div className="mb-1 flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-stone-900">{p.symbol}</span>
                    <Badge tone={p.stance === "bullish" ? "good" : p.stance === "bearish" ? "bad" : "neutral"}>
                      {p.stance}
                    </Badge>
                    <Badge tone="teal">{p.mode}</Badge>
                    {p.strategyId && <Badge>{p.strategyId}</Badge>}
                  </div>
                  <p className="text-sm text-stone-800">{p.conclusion}</p>
                  <p className="mt-2 text-[11px] uppercase tracking-wide text-stone-500">Technical</p>
                  <p className="text-xs text-stone-600">{p.technical}</p>
                  <p className="mt-2 text-[11px] uppercase tracking-wide text-stone-500">Fundamental</p>
                  <p className="text-xs text-stone-600">{p.fundamental}</p>
                  {p.risks && <p className="mt-2 text-[11px] text-stone-500">{p.risks}</p>}
                </article>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5">
        <Card>
          <CardHeader className="flex flex-col gap-1 md:flex-row md:items-start md:justify-between">
            <div>
              <CardTitle>5-year method lab</CardTitle>
              <p className="text-xs text-stone-500">
                {backtest?.status === "complete"
                  ? `${backtest.variantsTested} variants · ${backtest.variantsPromoted} promoted · Nifty ${pct(backtest.niftyReturnPct)}`
                  : backtest?.status === "running"
                    ? "Backtest running…"
                    : "Searches methods and timeframes on a 5-year weekday mock tape, then seeds the live roster."}
              </p>
            </div>
            <Badge tone={backtest?.status === "complete" ? "teal" : "neutral"}>
              {backtest?.status || "idle"}
            </Badge>
          </CardHeader>
          <CardContent>
            {!backtest?.variants?.length ? (
              <Empty>Run the 5-year backtest to invent and rank new strategy frames.</Empty>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead className="text-[11px] uppercase tracking-wide text-stone-500">
                    <tr>
                      <th className="py-2">Method</th>
                      <th>Frame</th>
                      <th>Excess vs Nifty</th>
                      <th>Win</th>
                      <th>Trades</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {backtest.variants.map((v) => (
                      <tr key={v.strategyId} className="border-t border-stone-100">
                        <td className="py-2 font-medium">{v.method}</td>
                        <td>{v.timeframe}</td>
                        <td className={`num ${v.excessPct >= 0 ? "text-emerald-800" : "text-rose-800"}`}>
                          {pct(v.excessPct)}
                        </td>
                        <td className="num">{Math.round((v.winRate || 0) * 100)}%</td>
                        <td className="num">{v.trades}</td>
                        <td>
                          {v.promoted ? <Badge tone="good">live</Badge> : <Badge>lab</Badge>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {backtest.note && <p className="mt-3 text-[11px] text-stone-500">{backtest.note}</p>}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5 grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Holdings</CardTitle>
          </CardHeader>
          <CardContent>
            {positions.length === 0 ? (
              <Empty>No open names yet — fills wait for NSE hours; overnight is research only.</Empty>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead className="text-[11px] uppercase tracking-wide text-stone-500">
                    <tr>
                      <th className="py-2">Symbol</th>
                      <th>Qty</th>
                      <th>Last</th>
                      <th>P&L</th>
                    </tr>
                  </thead>
                  <tbody>
                    {positions.map((p) => (
                      <tr key={p.symbol} className="border-t border-stone-100">
                        <td className="py-2 font-medium">{p.symbol}</td>
                        <td className="num">{p.qty}</td>
                        <td className="num">{inr(p.last)}</td>
                        <td className={`num ${p.pnl >= 0 ? "text-emerald-800" : "text-rose-800"}`}>{inr(p.pnl)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Journal & lessons</CardTitle>
          </CardHeader>
          <CardContent className="max-h-80 space-y-3 overflow-auto">
            {!journal?.entries?.length ? (
              <Empty>Closed trades write why they helped or hurt versus Nifty.</Empty>
            ) : (
              journal.entries.map((e) => (
                <div key={e.id} className="rounded-lg border border-stone-200 p-3">
                  <p className="text-sm">{e.lesson}</p>
                  <p className="mt-1 text-[11px] text-stone-500">
                    {e.trade?.symbol} · {e.trade?.strategyId} · {e.tags?.join(" · ")}
                  </p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5">
        <Card>
          <CardHeader>
            <CardTitle>Tape</CardTitle>
          </CardHeader>
          <CardContent className="max-h-72 space-y-2 overflow-auto">
            {!news?.items?.length ? (
              <Empty>Headline cache is empty.</Empty>
            ) : (
              news.items.slice(0, 12).map((n) => (
                <div key={n.id} className="border-b border-stone-100 pb-2">
                  <p className="text-sm">{n.title}</p>
                  <p className="text-[11px] text-stone-500">
                    {n.provider} · {(n.symbols || []).join(", ")}
                  </p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <footer className="mt-10 border-t border-stone-300/70 pt-4 text-xs text-stone-500">
        Educational paper trading only. Not investment advice. Past or simulated excess versus Nifty,
        Sensex, or mutual-fund peers is not future performance. Live INDstocks orders are off.
      </footer>
    </div>
  );
}
