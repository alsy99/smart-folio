"use client";

import { Empty } from "@/components/desk/hero";
import { Badge } from "@/components/ui/badge";
import type { useDesk } from "@/hooks/use-desk";

export function ResearchPanel({ desk }: { desk: ReturnType<typeof useDesk> }) {
  const { investigations, picks, news } = desk;
  return (
    <div className="space-y-10">
      <div className="grid gap-10 lg:grid-cols-2">
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Investigations</h2>
          <p className="mt-1 text-sm text-steel">
            {investigations
              ? `${investigations.reports?.length || 0} reports, ${investigations.mode}`
              : "Loading…"}
          </p>
          <div className="mt-4 max-h-[520px] space-y-6 overflow-auto pr-1">
            {!investigations?.reports?.length ? (
              <Empty>No material clusters yet. Wait for the sentiment service to ingest headlines.</Empty>
            ) : (
              investigations.reports.map((r, i) => (
                <article key={`${r.id}-${i}`} className="border-t border-rule pt-4">
                  <div className="mb-2 flex flex-wrap items-baseline gap-x-3 gap-y-1">
                    <Badge tone={r.stance === "bullish" ? "good" : r.stance === "bearish" ? "bad" : "neutral"}>
                      {r.stance}
                    </Badge>
                    <span className="text-sm text-steel">{r.eventType}</span>
                    {r.standAside && <Badge tone="warn">stand aside</Badge>}
                  </div>
                  <p className="font-semibold break-words text-ink">{r.headline}</p>
                  <p className="prose-research mt-2 whitespace-pre-wrap text-ink/85">{r.thesis}</p>
                  {r.risks && <p className="mt-2 text-sm text-steel">{r.risks}</p>}
                  <p className="mt-2 text-sm text-steel">
                    {(r.symbols || []).filter(Boolean).join(", ") || "Unmapped"} {r.horizon}
                  </p>
                </article>
              ))
            )}
          </div>
        </section>

        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Tape</h2>
          <div className="mt-4 max-h-[520px] space-y-3 overflow-auto pr-1">
            {!news?.items?.length ? (
              <Empty>No headlines in cache.</Empty>
            ) : (
              news.items.slice(0, 20).map((n) => (
                <div key={n.id} className="border-t border-rule pt-3">
                  <p className="text-sm break-words">{n.title}</p>
                  <p className="mt-1 text-sm text-steel">
                    {n.provider}
                    {(n.symbols || []).length ? `, ${(n.symbols || []).join(", ")}` : ""}
                  </p>
                </div>
              ))
            )}
          </div>
        </section>
      </div>

      <section>
        <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Pick Research</h2>
        <p className="mt-1 text-sm text-steel">
          Technicals on the desk tape, plus a fundamental card. LLM writes the conclusion when a name is selected.
        </p>
        <div className="mt-4 grid gap-8 md:grid-cols-2">
          {!picks.length ? (
            <Empty>Waiting for the planner to select names.</Empty>
          ) : (
            picks.map((p) => (
              <article key={p.symbol} className="border-t border-rule pt-4">
                <div className="mb-2 flex flex-wrap items-baseline gap-x-3">
                  <span className="font-semibold">{p.symbol}</span>
                  <Badge tone={p.stance === "bullish" ? "good" : p.stance === "bearish" ? "bad" : "neutral"}>
                    {p.stance}
                  </Badge>
                  <span className="text-sm text-steel">{p.mode}</span>
                </div>
                <p className="prose-research">{p.conclusion}</p>
                <p className="mt-3 text-sm text-steel">{p.technical}</p>
                <p className="mt-2 text-sm text-steel">{p.fundamental}</p>
                {p.risks && <p className="mt-2 text-sm text-steel">{p.risks}</p>}
              </article>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
