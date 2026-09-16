"use client";

import { Empty } from "@/components/desk/hero";
import type { useDesk } from "@/hooks/use-desk";
import { lastOpen } from "@/lib/campaigns";
import { inr, pct, shortSha } from "@/lib/utils";

export function CampaignPanel({ desk }: { desk: ReturnType<typeof useDesk> }) {
  const campaigns = desk.campaigns;
  return (
    <div className="space-y-6">
      <section>
        <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Frozen ledgers</h2>
        <p className="mt-1 text-sm text-steel">
          Four published 30-day books on the same INDstocks tape. The cash month is historical truth; A/B/C are the
          policy books. A clone of the SHA must replay each equity line within ₹1.
        </p>
      </section>
      {!campaigns.length ? (
        <Empty>No frozen ledgers on this checkout.</Empty>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[52rem] text-left text-sm">
            <thead>
              <tr className="border-b border-rule text-steel">
                <th className="py-2 pr-4 font-medium">Book</th>
                <th className="py-2 pr-4 font-medium">IPS</th>
                <th className="py-2 pr-4 font-medium">Last equity</th>
                <th className="py-2 pr-4 font-medium">Vs Nifty 50</th>
                <th className="py-2 pr-4 font-medium">Core</th>
                <th className="py-2 pr-4 font-medium">Halt</th>
                <th className="py-2 font-medium">Reproduce</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((c) => {
                const m = c.manifest;
                const d = lastOpen(c);
                return (
                  <tr key={m.name} className="border-b border-rule align-top">
                    <td className="py-3 pr-4">
                      <p className="font-semibold">{m.name}</p>
                      <p className="mt-1 font-mono text-xs text-steel">SHA {shortSha(m.gitSha)}</p>
                      {m.synthetic ? <p className="mt-1 text-xs text-brass">Synthetic fixture</p> : null}
                    </td>
                    <td className="py-3 pr-4 text-steel">
                      {m.ipsLine || "pre-IPS · satellite empty · cash"}
                      {m.note ? <p className="mt-1 text-xs">{m.note}</p> : null}
                    </td>
                    <td className="num py-3 pr-4">{d ? inr(d.equity) : "—"}</td>
                    <td className="num py-3 pr-4">{d ? pct(d.excessNifty50Pct) : "—"}</td>
                    <td className="num py-3 pr-4">
                      {m.policy ? (d?.coreInr != null ? inr(d.coreInr) : "—") : "—"}
                    </td>
                    <td className="py-3 pr-4">{d?.halted ? "Halted" : "Not halted"}</td>
                    <td className="py-3 font-mono text-xs text-steel">{m.reproduce}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
