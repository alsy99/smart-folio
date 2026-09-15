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
import { inr, pct } from "@/lib/utils";

export function BookPanel({ desk }: { desk: ReturnType<typeof useDesk> }) {
  const { campaign, benchmarks, history, positions } = desk;
  return (
    <div className="space-y-8">
      <div className="grid gap-10 lg:grid-cols-[1.35fr_0.65fr]">
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Book versus the Tape</h2>
          <p className="mt-1 text-sm text-steel">
            Equity path. Positional cash: 8% name cap, 15% book halt. Fills 09:15–15:30 IST.
          </p>
          <div className="mt-4 h-64">
            {history.length < 2 ? (
              <Empty>No ticks yet. The fill window opens at 09:15 IST.</Empty>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={history}>
                  <CartesianGrid stroke="#d0d4dc" />
                  <XAxis dataKey="t" tick={{ fontSize: 11, fill: "#5c6573" }} />
                  <YAxis
                    width={56}
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
                    fill={campaign?.marketOpen ? "#b0892a" : "#c5cad3"}
                    fillOpacity={0.28}
                  />
                </AreaChart>
              </ResponsiveContainer>
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

      <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Holdings</h2>
        <div className="mt-3">
          {positions.length === 0 ? (
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
