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
import { Empty } from "@/components/desk/hero";
import type { useDesk } from "@/hooks/use-desk";
import { mockTape } from "@/lib/tape";
import { NO_CORE_YET, SATELLITE_EMPTY, SATELLITE_EMPTY_WHY, satelliteEmpty } from "@/lib/satellite";
import { fillAllowed } from "@/lib/session";
import { inr, pct } from "@/lib/utils";
import type { EquityPoint } from "@/lib/types";

function EquityChart({ data, open }: { data: EquityPoint[]; open: boolean }) {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data}>
        <CartesianGrid stroke="#d0d4dc" />
        <XAxis dataKey="t" tick={{ fontSize: 11, fill: "#5c6573" }} />
        <YAxis
          width={56}
          domain={["dataMin", "dataMax"]}
          tick={{ fontSize: 11, fill: "#5c6573" }}
          tickFormatter={(v) =>
            new Intl.NumberFormat("en-IN", { notation: "compact", maximumFractionDigits: 1 }).format(v)
          }
        />
        <Tooltip />
        <Area
          type="monotone"
          dataKey="equity"
          stroke="#1c2430"
          fill={open ? "#b0892a" : "#c5cad3"}
          fillOpacity={0.28}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

export function BookPanel({ desk }: { desk: ReturnType<typeof useDesk> }) {
  const { campaign, benchmarks, history, positions } = desk;
  const open = Boolean(campaign?.marketOpen);
  const frozen = Boolean(desk.pub?.days?.length);
  const satEmpty =
    satelliteEmpty(desk.portfolio?.weights) ||
    Boolean(frozen && (desk.pub?.manifest.roster?.length ?? 0) === 0 && desk.pub?.manifest.failing?.length);
  return (
    <div className="space-y-8">
      <div className="grid gap-10 lg:grid-cols-[1.35fr_0.65fr]">
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Book versus the Tape</h2>
          <p className="mt-1 text-sm text-steel">
            {desk.pub?.manifest?.frozen
              ? `Frozen public 30-day path. SHA ${desk.pub.manifest.gitSha.slice(0, 12)}. One MOC mark per session on ${desk.pub.manifest.tape}.`
              : "Equity path. 8% name, 90% gross, 10% cash, 25% sector, 15% peak-to-trough halt. Fills 09:15–15:30 IST."}
            {desk.health && mockTape(desk.health) ? " MOCK TAPE — this path is not live NSE." : ""}
          </p>
          <div className="mt-4 h-64">
            {frozen && history.length >= 2 ? (
              <EquityChart data={history} open={open} />
            ) : frozen ? (
              <Empty>No published daily prints yet.</Empty>
            ) : !fillAllowed(campaign) ? (
              <Empty>Market closed. No fills. The cash window is 09:15–15:30 IST.</Empty>
            ) : history.length < 2 ? (
              <Empty>No ticks yet. The fill window opens at 09:15 IST.</Empty>
            ) : (
              <EquityChart data={history} open={open} />
            )}
          </div>
        </section>

        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Peers</h2>
          <div className="mt-3 space-y-0">
            {desk.pub?.days?.length ? (
              [
                { id: "NIFTY50", name: "Nifty 50", excessPct: desk.pub.days[desk.pub.days.length - 1].excessNifty50Pct },
                { id: "NIFTY500", name: "Nifty 500", excessPct: desk.pub.days[desk.pub.days.length - 1].excessNifty500Pct },
                { id: "SENSEX", name: "S&P BSE Sensex", excessPct: desk.pub.days[desk.pub.days.length - 1].excessSensexPct },
              ].map((b) => (
                <div key={b.id} className="flex min-w-0 items-baseline justify-between gap-3 border-b border-rule py-2.5">
                  <p className="min-w-0 truncate text-sm">{b.name}</p>
                  <p className={`num shrink-0 text-sm ${b.excessPct >= 0 ? "text-up" : "text-down"}`}>
                    {pct(b.excessPct)}
                  </p>
                </div>
              ))
            ) : !benchmarks?.benchmarks?.length ? (
              <Empty>No benchmarks yet.</Empty>
            ) : (
              benchmarks.benchmarks.map((b) => (
                <div key={b.id} className="flex min-w-0 items-baseline justify-between gap-3 border-b border-rule py-2.5">
                  <p className="min-w-0 truncate text-sm">{b.name}</p>
                  <p className={`num shrink-0 text-sm ${b.excessPct >= 0 ? "text-up" : "text-down"}`}>
                    {pct(b.excessPct)}
                  </p>
                </div>
              ))
            )}
          </div>
        </section>
      </div>

      {desk.pub?.days?.length ? (
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Daily print</h2>
          <p className="mt-1 text-sm text-steel">
            Equity, excess vs Nifty 50 / Nifty 500 / Sensex, drawdown, turnover, fills, halted-or-not.
          </p>
          <div className="mt-3 overflow-x-auto">
            <table className="w-full min-w-[44rem] text-left text-sm">
              <thead className="text-steel">
                <tr>
                  <th className="py-2 font-medium">Date</th>
                  <th className="font-medium">Equity</th>
                  <th className="font-medium">Nifty 50</th>
                  <th className="font-medium">Nifty 500</th>
                  <th className="font-medium">Sensex</th>
                  <th className="font-medium">DD</th>
                  <th className="font-medium">Turnover</th>
                  <th className="font-medium">Fills</th>
                  <th className="font-medium">Halt</th>
                </tr>
              </thead>
              <tbody>
                {desk.pub.days.map((d) => (
                  <tr key={d.date} className="border-t border-rule">
                    <td className="py-2 font-medium">{d.date}</td>
                    <td className="num">{inr(d.equity)}</td>
                    <td className={`num ${d.excessNifty50Pct >= 0 ? "text-up" : "text-down"}`}>
                      {pct(d.excessNifty50Pct)}
                    </td>
                    <td className={`num ${d.excessNifty500Pct >= 0 ? "text-up" : "text-down"}`}>
                      {pct(d.excessNifty500Pct)}
                    </td>
                    <td className={`num ${d.excessSensexPct >= 0 ? "text-up" : "text-down"}`}>
                      {pct(d.excessSensexPct)}
                    </td>
                    <td className="num">{pct(d.drawdownPct)}</td>
                    <td className="num">{pct(d.turnoverPct)}</td>
                    <td className="num">{d.fills}</td>
                    <td>{d.halted ? "halted" : "open"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      <section>
        <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Holdings</h2>
        <div className="mt-3">
          {positions.length === 0 && satEmpty ? (
            <div data-testid="book-satellite-empty" className="border border-brass/40 bg-paper px-4 py-3 text-sm">
              <p className="font-semibold">{SATELLITE_EMPTY}</p>
              <p className="mt-1 text-steel">
                {SATELLITE_EMPTY_WHY} {NO_CORE_YET}
                {desk.portfolio ? ` Cash ${inr(desk.portfolio.cash)}.` : ""}
              </p>
            </div>
          ) : positions.length === 0 ? (
            <Empty>No open names. Research still runs after the close.</Empty>
          ) : (
            <table className="w-full text-left text-sm">
              <thead className="text-steel">
                <tr>
                  <th className="py-2 font-medium">Symbol</th>
                  <th className="font-medium">Qty</th>
                  <th className="font-medium">Last</th>
                  <th className="font-medium">P&L</th>
                </tr>
              </thead>
              <tbody>
                {positions.map((p) => (
                  <tr key={p.symbol} className="border-t border-rule">
                    <td className="py-2 font-medium">{p.symbol}</td>
                    <td className="num">{p.qty}</td>
                    <td className="num">{inr(p.last)}</td>
                    <td className={`num ${p.pnl >= 0 ? "text-up" : "text-down"}`}>{inr(p.pnl)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </section>
    </div>
  );
}
