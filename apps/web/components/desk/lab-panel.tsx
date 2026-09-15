"use client";

import { Empty } from "@/components/desk/hero";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { useDesk } from "@/hooks/use-desk";
import { labNote, rosterLine } from "@/lib/roster";
import { SATELLITE_EMPTY, SATELLITE_EMPTY_WHY, satelliteEmpty } from "@/lib/satellite";
import { pct } from "@/lib/utils";

export function LabPanel({ desk }: { desk: ReturnType<typeof useDesk> }) {
  const { portfolio, journal, backtest, btBusy, runBacktest } = desk;
  return (
    <div className="space-y-10">
      <div className="grid gap-10 lg:grid-cols-2">
        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Satellite Weights</h2>
          {satelliteEmpty(portfolio?.weights) && (
            <p data-testid="lab-satellite-empty" className="mt-2 text-sm font-semibold text-brass">
              {SATELLITE_EMPTY} — {SATELLITE_EMPTY_WHY} Nothing below is trading.
            </p>
          )}
          <div className="mt-4 space-y-3">
            {(portfolio?.weights || []).length === 0 ? (
              <Empty>Weights appear once learning has a prior.</Empty>
            ) : (
              (portfolio?.weights || []).map((w) => (
                <div key={w.strategyId} className="min-w-0">
                  <div className="mb-1 flex justify-between gap-3 text-sm">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="min-w-0 truncate" title={w.strategyId}>
                        {w.strategyId}
                      </span>
                      {w.regime === "failing-gate" && <Badge tone="bad">failing gate · weight 0</Badge>}
                    </span>
                    <span className="num shrink-0">{(w.weight * 100).toFixed(1)}%</span>
                  </div>
                  <div className="h-1 bg-rule">
                    <div className="h-full bg-ink" style={{ width: `${Math.min(100, w.weight * 100)}%` }} />
                  </div>
                </div>
              ))
            )}
          </div>
        </section>

        <section>
          <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Journal</h2>
          <div className="mt-4 max-h-80 space-y-4 overflow-auto">
            {!journal?.entries?.length ? (
              <Empty>Closed trades store facts for the weekly review; a caption is not a retrain.</Empty>
            ) : (
              journal.entries.map((e) => (
                <div key={e.id} className="border-t border-rule pt-3">
                  <p className="prose-research text-[0.95rem]">{e.lesson}</p>
                  <p className="mt-1 text-sm text-steel">
                    {[e.trade?.symbol, e.trade?.strategyId, ...(e.tags || [])].filter(Boolean).join(", ")}
                  </p>
                </div>
              ))
            )}
          </div>
        </section>
      </div>

      <section>
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <h2 className="scroll-mt-6 text-lg font-semibold tracking-tight">Five-Year Method Lab</h2>
            <p className="mt-1 text-sm text-steel">
              {backtest?.status === "complete"
                ? `${backtest.variantsTested} variants, ${backtest.variantsPromoted} promoted, Nifty ${pct(backtest.niftyReturnPct)}`
                : backtest?.status === "running"
                  ? "Backtest running…"
                  : "Walk-forward on five years of real daily bars. Read-only: the lab is the satellite admission exam, not the book. Mock-tape runs report but never promote."}
            </p>
            <p className="mt-1 font-mono text-xs text-steel">{rosterLine(backtest)}</p>
          </div>
          <Button variant="outline" onClick={runBacktest} disabled={btBusy}>
            {btBusy ? "Running Five-Year Backtest…" : "Run Five-Year Backtest"}
          </Button>
        </div>
        <div className="mt-4">
          {!backtest?.variants?.length ? (
            <Empty>Run the backtest to rank new strategy frames.</Empty>
          ) : (
            <div className="max-h-[28rem] overflow-auto overscroll-contain">
              <table className="w-full text-left text-sm">
                <thead className="text-steel">
                  <tr>
                    <th className="py-2 font-medium">Method</th>
                    <th className="font-medium">Frame</th>
                    <th className="font-medium">Excess vs Nifty</th>
                    <th className="font-medium">Win</th>
                    <th className="font-medium">Trades</th>
                    <th className="font-medium">Roster</th>
                  </tr>
                </thead>
                <tbody>
                  {backtest.variants.map((v) => (
                    <tr key={v.strategyId} className="border-t border-rule [content-visibility:auto] [contain-intrinsic-size:auto_2.5rem]">
                      <td className="py-2 font-medium">{v.method}</td>
                      <td>{v.timeframe}</td>
                      <td className={`num ${v.excessPct >= 0 ? "text-up" : "text-down"}`}>{pct(v.excessPct)}</td>
                      <td className="num">{Math.round((v.winRate || 0) * 100)}%</td>
                      <td className="num">{v.trades}</td>
                      <td>
                        {/* "roster" is a gate verdict, not a claim that it is trading now. */}
                        {v.promoted ? <Badge tone="good">roster</Badge> : <Badge>lab</Badge>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {labNote(backtest) && <p className="mt-3 text-sm text-steel">{labNote(backtest)}</p>}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
