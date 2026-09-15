"use client";

import { Suspense, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AdvisorTooltip } from "@/components/advisor-tooltip";
import { BookPanel } from "@/components/desk/book-panel";
import { Stat } from "@/components/desk/hero";
import { LabPanel } from "@/components/desk/lab-panel";
import { ResearchPanel } from "@/components/desk/research-panel";
import { SessionIris } from "@/components/desk/session-iris";
import { DeskTabs, isDeskTab, type DeskTab } from "@/components/desk/tabs";
import { Button } from "@/components/ui/button";
import { useDesk } from "@/hooks/use-desk";
import { inr, pct } from "@/lib/utils";

function sessionLabel(status?: string, marketOpen?: boolean) {
  if (!status && marketOpen == null) return "—";
  if (status?.includes("override")) return "Forced open";
  if (status === "weekend") return "Weekend";
  if (marketOpen || status === "open") return "Open";
  if (status === "closed") return "Closed";
  return status || "Closed";
}

export default function Home() {
  return (
    <Suspense fallback={<p className="p-6 text-sm text-steel">Connecting to the desk…</p>}>
      <DeskShell />
    </Suspense>
  );
}

function DeskShell() {
  const desk = useDesk();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const rawTab = searchParams.get("tab");
  const tab: DeskTab = isDeskTab(rawTab) ? rawTab : "book";
  const [pending, setPending] = useState<"stop" | "restart" | null>(null);
  const {
    err,
    loading,
    busy,
    portfolio,
    campaign,
    sentiment,
    nifty,
    onTrack,
    investigations,
    health,
    start,
    kill,
    resume,
  } = desk;
  const open = Boolean(campaign?.marketOpen);

  function setTab(id: DeskTab) {
    const next = new URLSearchParams(searchParams.toString());
    if (id === "book") next.delete("tab");
    else next.set("tab", id);
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }

  return (
    <div className="min-h-screen overflow-x-hidden md:pl-56">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:bg-blotter focus:px-3 focus:py-2 focus:text-sm focus:font-semibold"
      >
        Skip to desk
      </a>
      <aside className="border-b border-ink/10 px-5 py-5 pt-[max(1.25rem,env(safe-area-inset-top))] pb-16 md:fixed md:inset-y-0 md:left-0 md:flex md:w-56 md:flex-col md:border-b-0 md:border-r md:px-5 md:py-8 md:pb-16">
        <h1
          translate="no"
          className="text-2xl font-extrabold tracking-tight text-ink"
        >
          Aperture
        </h1>
        <p className="mt-3 max-w-[12.5rem] text-sm leading-snug text-steel text-pretty">
          Fills 09:15–15:30&nbsp;IST. Research does not sleep.
        </p>
        <div className="mt-8">
          <DeskTabs
            value={tab}
            onChange={setTab}
            counts={{
              research: investigations?.reports?.length || 0,
            }}
          />
        </div>
        <p className="mt-8 text-xs leading-relaxed text-steel md:mt-auto">
          Paper trading only. Not advice. Live brokerage is off.
        </p>
      </aside>

      <main id="main" className="min-w-0 p-4 md:p-6">
        <div className="min-h-[calc(100vh-2rem)] min-w-0 overflow-x-hidden bg-blotter px-5 py-6 md:px-10 md:py-8">
          <div className="flex flex-col gap-6 border-b border-rule pb-6">
            <div className="grid w-full grid-cols-[auto_minmax(0,1fr)] items-center gap-4">
              <SessionIris open={open} />
              <div className="min-w-0">
                <p className="text-sm text-steel">Session</p>
                <p className="text-4xl font-extrabold tracking-tight">
                  {sessionLabel(campaign?.sessionStatus, campaign?.marketOpen)}
                </p>
                <p className="mt-1 text-sm whitespace-normal text-steel">
                  {campaign?.active
                    ? `Day ${campaign.daysElapsed} of ${campaign.daysTotal}, ${campaign.ticks} ticks, ${campaign.autopilot ? "autopilot on" : "paused"}`
                    : "No campaign running"}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <AdvisorTooltip sentiment={sentiment} />
              {pending === "restart" ? (
                <>
                  <Button variant="danger" onClick={() => { setPending(null); start(); }} disabled={busy}>
                    Confirm Restart
                  </Button>
                  <Button variant="ghost" onClick={() => setPending(null)}>
                    Keep Book
                  </Button>
                </>
              ) : (
                <Button onClick={() => (campaign?.active ? setPending("restart") : start())} disabled={busy}>
                  {campaign?.active ? "Restart 30-Day Book" : "Start 30-Day Book"}
                </Button>
              )}
              {campaign?.autopilot ? (
                pending === "stop" ? (
                  <>
                    <Button variant="danger" onClick={() => { setPending(null); kill(); }} disabled={busy}>
                      Confirm Stop
                    </Button>
                    <Button variant="ghost" onClick={() => setPending(null)}>
                      Keep Autopilot
                    </Button>
                  </>
                ) : (
                  <Button variant="danger" onClick={() => setPending("stop")} disabled={busy}>
                    Stop Autopilot
                  </Button>
                )
              ) : (
                <Button variant="outline" onClick={resume} disabled={busy || !campaign?.active}>
                  Resume Autopilot
                </Button>
              )}
            </div>
          </div>

          {err && (
            <div role="alert" className="mt-6 border border-down/30 bg-down/5 px-4 py-3 text-sm text-down">
              Gateway is not reachable on port 8080. Start the backend, then refresh.
            </div>
          )}

          {loading && !portfolio && (
            <p aria-live="polite" className="mt-6 text-sm text-steel">
              Connecting to the desk…
            </p>
          )}

          <section className="mt-6 grid grid-cols-1 gap-6 border-b border-rule pb-6 sm:grid-cols-2 xl:grid-cols-4">
            <Stat
              label="Paper equity"
              value={portfolio ? inr(portfolio.equity) : "—"}
              hint={portfolio ? `Cash ${inr(portfolio.cash)}` : "Awaiting a campaign"}
            />
            <Stat
              label="Versus Nifty 50"
              value={nifty ? pct(nifty.excessPct) : "—"}
              hint={nifty ? `Annualised ${pct(nifty.excessAnnPct)}` : "Start a campaign"}
              tone={nifty && nifty.excessPct >= 0 ? "good" : "bad"}
            />
            <Stat
              label="Ten-point track"
              value={onTrack ? "On track" : "Off pace"}
              hint={`Beat rate ${nifty ? Math.round((nifty.beatRate || 0) * 100) : 0}% of ticks`}
              tone={onTrack ? "good" : "warn"}
            />
            <Stat
              label="Window"
              value={open ? "Fills on" : "Research only"}
              hint={
                health?.indstocks?.mode === "live"
                  ? "INDstocks tape, paper fills"
                  : campaign?.clockOverride
                    ? "Clock override is on"
                    : "Mock tape until an INDstocks token is set"
              }
              tone={open ? "good" : "warn"}
            />
          </section>

          <div
            className="pt-6"
            role="tabpanel"
            id={`panel-${tab}`}
            aria-labelledby={`tab-${tab}`}
          >
            {tab === "book" && <BookPanel desk={desk} />}
            {tab === "research" && <ResearchPanel desk={desk} />}
            {tab === "lab" && <LabPanel desk={desk} />}
          </div>
        </div>
      </main>
    </div>
  );
}
