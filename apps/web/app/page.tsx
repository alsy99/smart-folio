"use client";

import { Suspense, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AdvisorTooltip } from "@/components/advisor-tooltip";
import { BookPanel } from "@/components/desk/book-panel";
import { Stat } from "@/components/desk/hero";
import { IPSForm } from "@/components/desk/ips-form";
import { LabPanel } from "@/components/desk/lab-panel";
import { ResearchPanel } from "@/components/desk/research-panel";
import { SessionIris } from "@/components/desk/session-iris";
import { DeskTabs, isDeskTab, type DeskTab } from "@/components/desk/tabs";
import { Button } from "@/components/ui/button";
import { useDesk } from "@/hooks/use-desk";
import { inr, istStamp, pct, shortSha } from "@/lib/utils";
import { mockTape } from "@/lib/tape";
import { campaignTapeHint, heroExcess, heroTape } from "@/lib/hero";
import { ipsLine } from "@/lib/ips";
import { rosterIsDefault, rosterLine } from "@/lib/roster";
import { satelliteEmpty, satelliteState, TARGET_NOT_PROMISE } from "@/lib/satellite";
import { fillAllowed } from "@/lib/session";

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
  const frozen = Boolean(pub?.manifest?.frozen);
  const lastDay = pub?.days?.length ? pub.days[pub.days.length - 1] : null;
  const tape = heroTape(health);
  // Phase 0 honesty: with every method at weight 0 and no IPS core yet, the
  // desk says SATELLITE EMPTY and CASH rather than rendering failed methods
  // as live. The frozen cash ledger (roster []) reads the same way.
  const satEmpty = satelliteEmpty(portfolio?.weights) || Boolean(frozen && pub && (pub.manifest.roster?.length ?? 0) === 0 && pub.manifest.failing?.length);
  const satellite = satelliteState(
    satEmpty && !satelliteEmpty(portfolio?.weights)
      ? (pub?.manifest.failing ?? []).map((id) => ({ strategyId: id, weight: 0, expectancy: 0, winRate: 0, regime: "failing-gate" }))
      : portfolio?.weights,
    false,
  );
  const niftyHero = isMock ? heroExcess(health, nifty?.excessPct) : lastDay ? pct(lastDay.excessNifty50Pct) : nifty ? pct(nifty.excessPct) : "—";

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
                  {frozen && pub
                    ? `Frozen SHA ${shortSha(pub.manifest.gitSha)} · ${istStamp(pub.manifest.startUnixMs)} → ${istStamp(pub.manifest.endUnixMs)} IST`
                    : campaign?.active
                      ? `Day ${campaign.daysElapsed} of ${campaign.daysTotal}, ${campaign.ticks} ticks, ${campaign.autopilot ? "autopilot on" : "paused"} · ${istStamp(campaign.startedAtUnixMs)} → ${istStamp(campaign.endsAtUnixMs)} IST`
                      : "No campaign running"}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <AdvisorTooltip sentiment={sentiment} />
              {frozen ? (
                <Button disabled>
                  Book frozen
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

          {frozen && pub && (
            <div
              role="status"
              className="mt-6 border border-ink/15 bg-paper px-4 py-3 text-sm text-ink"
            >
              Public 30-day paper campaign is frozen on {pub.manifest.tape}. Same SHA, settings, universe, and
              delivery cost model. Roster as-of {pub.manifest.rosterAsOf ?? "—"}:{" "}
              {pub.manifest.roster?.length ? pub.manifest.roster.join(", ") : "none cleared the gate"}
              {pub.manifest.failing?.length
                ? ` · failing at weight 0: ${pub.manifest.failing.join(", ")}`
                : ""}
              . Reproduce with <span className="font-mono text-xs">{pub.manifest.reproduce}</span>.
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
              label={frozen ? "Frozen paper equity" : isMock ? "Mock paper equity" : "Paper equity"}
              value={lastDay ? inr(lastDay.equity) : portfolio ? inr(portfolio.equity) : "—"}
              hint={
                frozen
                  ? lastDay?.halted
                    ? "Halted · 15% peak-to-trough"
                    : `Drawdown ${pct(lastDay?.drawdownPct || 0)}`
                  : isMock
                    ? "Synthetic. Not live NSE."
                    : portfolio
                      ? `Cash ${inr(portfolio.cash)}`
                      : "Awaiting a campaign"
              }
              tone={isMock && !frozen ? "bad" : lastDay?.halted ? "bad" : undefined}
            />
            <Stat
              label="Versus Nifty 50"
              value={niftyHero}
              hint={
                isMock
                  ? "Mock tape. Not a live excess vs Nifty."
                  : lastDay
                    ? `Nifty 500 ${pct(lastDay.excessNifty500Pct)} · Sensex ${pct(lastDay.excessSensexPct)}`
                    : nifty
                      ? `Target, not a promise · annualised ${pct(nifty.excessAnnPct)}`
                      : "Start a campaign"
              }
              tone={isMock ? "bad" : (lastDay ? lastDay.excessNifty50Pct : nifty?.excessPct || 0) >= 0 ? "good" : "bad"}
            />
            <Stat
              label={satEmpty ? "Satellite" : frozen ? "Halt" : "Ten-point track"}
              value={
                satEmpty
                  ? satellite.value
                  : frozen
                    ? lastDay?.halted
                      ? "Halted"
                      : "Not halted"
                    : onTrack
                      ? "On track"
                      : "Off pace"
              }
              hint={
                satEmpty
                  ? satellite.hint
                  : frozen && lastDay
                    ? `Turnover ${pct(lastDay.turnoverPct)} · ${lastDay.fills} fills on last session`
                    : `${TARGET_NOT_PROMISE} Beat ${nifty ? Math.round((nifty.beatRate || 0) * 100) : 0}% of ticks.`
              }
              tone={satEmpty ? "warn" : frozen ? (lastDay?.halted ? "bad" : "good") : onTrack ? "good" : "warn"}
              testId="hero-satellite"
            />
            <Stat
              label={tape.label}
              value={tape.value}
              hint={
                isMock
                  ? tape.hint
                  : frozen && pub
                    ? campaignTapeHint(pub.manifest)
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
                frozen={Boolean(frozen && pub?.manifest.ipsId && pub.manifest.ipsId === desk.ips?.id)}
                onSave={desk.saveIPS}
              />
            )}
            {tab === "research" && <ResearchPanel desk={desk} />}
            {tab === "lab" && <LabPanel desk={desk} />}
          </div>
        </div>
      </main>
    </div>
  );
}
