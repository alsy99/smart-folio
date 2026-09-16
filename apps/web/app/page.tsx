"use client";

import { Suspense, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AdvisorTooltip } from "@/components/advisor-tooltip";
import { BookPanel } from "@/components/desk/book-panel";
import { CampaignPanel } from "@/components/desk/campaign-panel";
import { Stat } from "@/components/desk/hero";
import { IPSForm } from "@/components/desk/ips-form";
import { LabPanel } from "@/components/desk/lab-panel";
import { ResearchPanel } from "@/components/desk/research-panel";
import { SessionIris } from "@/components/desk/session-iris";
import { DeskTabs, isDeskTab, type DeskTab } from "@/components/desk/tabs";
import { Button } from "@/components/ui/button";
import { useDesk } from "@/hooks/use-desk";
import { inr, istStamp, pct } from "@/lib/utils";
import { mockTape } from "@/lib/tape";
import { heroExcess, heroExcessHint, heroTape } from "@/lib/hero";
import { ipsLine } from "@/lib/ips";
import { rosterIsDefault, rosterLine } from "@/lib/roster";
import { satelliteEmpty, satelliteState, TARGET_NOT_PROMISE } from "@/lib/satellite";
import { fillAllowed } from "@/lib/session";
import { HALT_HOLDS } from "@/lib/core";

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
    pub,
    backtest,
  } = desk;
  const open = Boolean(campaign?.marketOpen);
  const isMock = mockTape(health);
  const published = Boolean(pub?.manifest?.frozen);
  const tape = heroTape(health);
  // Phase 0 honesty: with every method at weight 0 the desk says SATELLITE
  // EMPTY rather than rendering failed methods as live. Use this session's
  // weights, not the frozen cash-month roster.
  const satEmpty = satelliteEmpty(portfolio?.weights);
  const satellite = satelliteState(portfolio?.weights, Boolean(desk.ips));
  const niftyHero = isMock ? heroExcess(health, nifty?.excessPct) : nifty ? pct(nifty.excessPct) : "—";

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
        <p className={`mt-8 text-xs leading-relaxed md:mt-auto ${isMock ? "font-semibold text-down" : "text-steel"}`}>
          {isMock
            ? "MOCK TAPE. Not live NSE. Paper PnL is synthetic until an INDstocks token is set."
            : "Paper trading only. Not advice. Live brokerage is off."}
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
                    ? `Day ${campaign.daysElapsed} of ${campaign.daysTotal}, ${campaign.ticks} ticks, ${campaign.autopilot ? "autopilot on" : "paused"} · ${istStamp(campaign.startedAtUnixMs)} → ${istStamp(campaign.endsAtUnixMs)} IST`
                    : "No campaign running"}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <AdvisorTooltip sentiment={sentiment} />
              {published ? (
                <Button disabled title="Published ledgers stay frozen. This session is the live paper book.">
                  Published ledgers frozen
                </Button>
              ) : pending === "restart" ? (
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

          {published && pub && (
            <div
              role="status"
              className="mt-6 border border-ink/15 bg-paper px-4 py-3 text-sm text-ink"
            >
              Published 30-day ledgers are frozen on {pub.manifest.tape} (Campaign tab). This Book is today&apos;s live
              paper session. Reproduce a ledger with{" "}
              <span className="font-mono text-xs">{pub.manifest.reproduce}</span>.
            </div>
          )}

          {isMock && !err && (
            <div
              role="alert"
              data-testid="mock-tape-banner"
              className="mt-6 border border-down bg-down px-4 py-3 text-sm font-semibold text-white"
            >
              MOCK TAPE — not live NSE. Paper PnL is synthetic until INDSTOCKS_ACCESS_TOKEN is set. Do not read this book as a live India fill.
            </div>
          )}

          {loading && !portfolio && (
            <p aria-live="polite" className="mt-6 text-sm text-steel">
              Connecting to the desk…
            </p>
          )}

          <p data-testid="hero-ips" className="mt-4 text-sm text-steel">
            <span className="font-semibold text-ink">IPS</span> · {ipsLine(desk.ips)}
            {desk.ipsMissing && !desk.ips ? (
              <>
                {" "}
                <button type="button" className="underline decoration-brass underline-offset-2" onClick={() => setTab("policy")}>
                  Set a policy
                </button>
              </>
            ) : null}
          </p>
          <p
            data-testid="hero-roster"
            className={`mt-4 truncate font-mono text-xs ${rosterIsDefault(backtest) ? "text-brass" : "text-steel"}`}
            title={rosterLine(backtest)}
          >
            {rosterLine(backtest)}
          </p>

          <section className="mt-6 grid grid-cols-1 gap-6 border-b border-rule pb-6 sm:grid-cols-2 xl:grid-cols-4">
            <Stat
              label={isMock ? "Mock paper equity" : "Paper equity"}
              value={portfolio ? inr(portfolio.equity) : "—"}
              hint={
                isMock
                  ? "Synthetic. Not live NSE."
                  : portfolio
                    ? `Cash ${inr(portfolio.cash)} · ${HALT_HOLDS}`
                    : "Awaiting a campaign"
              }
              wrapHint
              tone={isMock ? "bad" : undefined}
              testId="hero-dd"
            />
            <Stat
              label="Versus Nifty 50"
              value={niftyHero}
              hint={
                isMock
                  ? heroExcessHint(health)
                  : nifty
                    ? `Target, not a promise · annualised ${pct(nifty.excessAnnPct)}`
                    : "Start a campaign"
              }
              tone={isMock ? "bad" : (nifty?.excessPct || 0) >= 0 ? "good" : "bad"}
            />
            <Stat
              label={satEmpty ? "Satellite" : "Ten-point track"}
              value={satEmpty ? satellite.value : onTrack ? "On track" : "Off pace"}
              hint={
                satEmpty
                  ? satellite.hint
                  : `${TARGET_NOT_PROMISE} Beat ${nifty ? Math.round((nifty.beatRate || 0) * 100) : 0}% of ticks.`
              }
              tone={satEmpty ? "warn" : onTrack ? "good" : "warn"}
              testId="hero-satellite"
            />
            <Stat
              label={tape.label}
              value={tape.value}
              hint={
                isMock
                  ? tape.hint
                  : open && fillAllowed(campaign)
                    ? "INDstocks tape, positional fills"
                    : campaign?.clockOverride
                      ? "Clock override is on"
                      : "Research only. Market closed — no fills."
              }
              testId="hero-tape"
              tone={isMock ? "bad" : open ? "good" : "warn"}
            />
          </section>

          <div
            className="pt-6"
            role="tabpanel"
            id={`panel-${tab}`}
            aria-labelledby={`tab-${tab}`}
          >
            {tab === "book" && <BookPanel desk={desk} />}
            {tab === "policy" && (
              <IPSForm
                key={desk.ips?.id ?? "new"}
                current={desk.ips}
                frozen={Boolean(published && pub?.manifest.ipsId && pub.manifest.ipsId === desk.ips?.id)}
                onSave={desk.saveIPS}
              />
            )}
            {tab === "campaign" && <CampaignPanel desk={desk} />}
            {tab === "research" && <ResearchPanel desk={desk} />}
            {tab === "lab" && <LabPanel desk={desk} />}
          </div>
        </div>
      </main>
    </div>
  );
}
