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
import { CORE_STAYS_INVESTED, coreInvested, coreRows, HALT_HOLDS, rebalanceLine } from "@/lib/core";
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
          baseValue="dataMin"
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
  const satEmpty = satelliteEmpty(desk.portfolio?.weights);
  return (
    <div className="space-y-8">
      <div className="grid gap-10 lg:grid-cols-[1.35fr_0.65fr]">
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Book versus the Tape</h2>
          <p className="mt-1 text-sm text-steel">
            Live paper equity this session. 8% name, 90% gross, 10% cash, 25% sector, 15% peak-to-trough halt. New buys
            capped at 10% of equity per IST session, so a 100% core builds across days. Fills 09:15–15:30 IST.
            {desk.health && mockTape(desk.health) ? " MOCK TAPE — this path is not live NSE." : ""}
          </p>
          <div className="mt-4 h-64">
            {history.length >= 1 ? (
              <EquityChart data={history} open={open} />
            ) : !campaign?.ipsId ? (
              <Empty>No IPS bound. Set a policy — the book will not fill until then.</Empty>
            ) : !fillAllowed(campaign) ? (
              <Empty>Market closed. No fills. The cash window is 09:15–15:30 IST.</Empty>
            ) : (
              <Empty>No ticks yet. The fill window opens at 09:15 IST.</Empty>
            )}
          </div>
        </section>

        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Peers</h2>
          <div className="mt-3 space-y-0">
            {!benchmarks?.benchmarks?.length ? (
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

      {desk.ips && (
        <section data-testid="book-core">
          <div className="flex flex-col gap-1 md:flex-row md:items-baseline md:justify-between">
            <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Core versus target</h2>
            <p className="text-sm text-steel">{rebalanceLine(desk.targets, desk.ips.rebalance)}</p>
          </div>
          <p className="mt-1 text-sm text-steel">
            {CORE_STAYS_INVESTED} Invested {pct(coreInvested(desk.targets).actual * 100)} of a{" "}
            {pct(coreInvested(desk.targets).target * 100)} target after the name and sector caps.
          </p>
          <p data-testid="halt-holds" className="mt-1 text-sm text-steel">
            {HALT_HOLDS}
          </p>
          {coreRows(desk.targets).length === 0 ? (
            <Empty>Core targets appear once the policy service has marked the book.</Empty>
          ) : (
            <div className="mt-3 overflow-x-auto">
              <table className="w-full min-w-[32rem] text-left text-sm">
                <thead className="text-steel">
                  <tr>
                    <th className="py-2 font-medium">Name</th>
                    <th className="font-medium">Target</th>
                    <th className="font-medium">Held</th>
                    <th className="font-medium">Drift</th>
                    <th className="font-medium">Next session</th>
                  </tr>
                </thead>
                <tbody>
                  {coreRows(desk.targets).map((c) => (
                    <tr key={c.symbol} className="border-t border-rule">
                      <td className="py-2 font-medium">{c.symbol}</td>
                      <td className="num">{pct(c.target * 100)}</td>
                      <td className="num">{pct(c.actual * 100)}</td>
                      <td className={`num ${Math.abs(c.drift) > 0.01 ? "text-brass" : "text-steel"}`}>{pct(c.drift * 100)}</td>
                      <td>{c.ticket ? "ticket" : "inside band"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}

      <section>
        <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Holdings</h2>
        <div className="mt-3">
          {positions.length === 0 && satEmpty && !desk.ips && !desk.campaign?.ipsId ? (
            <div data-testid="book-satellite-empty" className="border border-brass/40 bg-paper px-4 py-3 text-sm">
              <p className="font-semibold">{SATELLITE_EMPTY}</p>
              <p className="mt-1 text-steel">
                {SATELLITE_EMPTY_WHY} {desk.ips || desk.campaign?.ipsId ? CORE_STAYS_INVESTED : NO_CORE_YET}
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
