"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  api,
  type Benchmarks,
  type Campaign,
  type Investigations,
  type Journal,
  type NewsFeed,
  type Portfolio,
  type Sentiment,
} from "@/lib/api";
import { inr, pct } from "@/lib/utils";

const POLL_MS = 4000;

export default function Home() {
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [portfolio, setPortfolio] = useState<Portfolio | null>(null);
  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [benchmarks, setBenchmarks] = useState<Benchmarks | null>(null);
  const [journal, setJournal] = useState<Journal | null>(null);
  const [investigations, setInvestigations] = useState<Investigations | null>(null);
  const [news, setNews] = useState<NewsFeed | null>(null);
  const [sentiment, setSentiment] = useState<Sentiment[]>([]);
  const [history, setHistory] = useState<{ t: string; equity: number; nifty: number }[]>([]);

  const refresh = useCallback(async () => {
    try {
      const [p, c, b, j, inv, n, s] = await Promise.all([
        api.portfolio(),
        api.campaign(),
        api.benchmarks(),
        api.journal(),
        api.investigations(),
        api.news(),
        api.sentiment(),
      ]);
      setPortfolio(p);
      setCampaign(c);
      setBenchmarks(b);
      setJournal(j);
      setInvestigations(inv);
      setNews(n);
      setSentiment(s.scores || []);
      setErr(null);
      const nifty = b.benchmarks?.find((x) => x.id === "NIFTY50");
      setHistory((h) => {
        const next = [
          ...h,
          {
            t: new Date().toLocaleTimeString("en-IN", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
            equity: p.equity,
            nifty: nifty ? nifty.start * (1 + nifty.returnPct / 100) : 0,
          },
        ];
        return next.slice(-24);
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "gateway unreachable");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, POLL_MS);
    return () => clearInterval(id);
  }, [refresh]);

  const nifty = benchmarks?.benchmarks?.find((x) => x.id === "NIFTY50");
  const onTrack = Boolean(nifty?.onTrack);

  async function start() {
    setBusy(true);
    try {
      await api.startCampaign(30);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "start failed");
    } finally {
      setBusy(false);
    }
  }

  async function kill() {
    setBusy(true);
    try {
      await api.setAutopilot(false);
      await refresh();
    } finally {
      setBusy(false);
    }
  }

  async function resume() {
    setBusy(true);
    try {
      await api.setAutopilot(true);
      await refresh();
    } finally {
      setBusy(false);
    }
  }

  const positions = useMemo(
    () => (portfolio?.positions || []).filter((p) => p.qty > 0),
    [portfolio]
  );

  return (
    <div className="mx-auto max-w-[1400px] px-4 py-6 md:px-8 md:py-10">
      <header className="mb-8 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.28em] text-teal-800">India paper desk · NSE</p>
          <h1 className="font-[family-name:var(--font-display)] text-4xl md:text-5xl leading-[1.05] text-stone-900">
            Aperture
          </h1>
          <p className="mt-2 max-w-xl text-sm text-stone-600">
            Parallel news investigations tilt a multi-strategy book. Success is excess versus Nifty 50,
            Nifty 500, Sensex, and top-fund peers — a target, not a promise.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <AdvisorTooltip sentiment={sentiment} />
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
            <Badge tone={campaign?.clockOverride ? "warn" : "teal"}>
              {campaign?.clockOverride ? "clock override" : "live IST clock"}
            </Badge>
          </CardHeader>
          <CardContent className="h-64">
            {history.length < 2 ? (
              <Empty>Start the campaign — the curve fills as ticks land.</Empty>
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
              investigations.reports.map((r) => (
                <article key={r.id} className="rounded-xl border border-stone-200 bg-stone-50/70 p-3">
                  <div className="mb-1 flex flex-wrap items-center gap-2">
                    <Badge tone={r.stance === "bullish" ? "good" : r.stance === "bearish" ? "bad" : "neutral"}>
                      {r.stance}
                    </Badge>
                    <Badge tone="teal">{r.eventType}</Badge>
                    <Badge>{r.corroboration} sources</Badge>
                    {r.standAside && <Badge tone="warn">stand aside</Badge>}
                  </div>
                  <p className="text-sm font-medium text-stone-900">{r.headline}</p>
                  <p className="mt-1 text-xs text-stone-600">{r.thesis}</p>
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

      <div className="mt-5 grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Holdings</CardTitle>
          </CardHeader>
          <CardContent>
            {positions.length === 0 ? (
              <Empty>Autopilot has not opened names yet — wait a tick or start the campaign.</Empty>
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

function Hero({
  label,
  value,
  sub,
  tone,
}: {
  label: string;
  value: string;
  sub: string;
  tone?: "good" | "bad" | "warn";
}) {
  const color =
    tone === "good" ? "text-emerald-800" : tone === "bad" ? "text-rose-800" : "text-stone-900";
  return (
    <Card>
      <CardContent className="pt-5">
        <p className="text-[11px] uppercase tracking-[0.18em] text-stone-500">{label}</p>
        <p className={`num mt-1 font-[family-name:var(--font-display)] text-2xl ${color}`}>{value}</p>
        <p className="mt-1 text-xs text-stone-500">{sub}</p>
      </CardContent>
    </Card>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-stone-500">{children}</p>;
}
