import { mockTape } from "./tape";
import { pct } from "./utils";
import type { Health, PublicManifest } from "./types";

/**
 * One line under the tape stat while the public campaign is frozen: which
 * tape, how many methods were allowed to trade, and that cash is the honest
 * outcome when the gate admitted none.
 */
export function campaignTapeHint(m: PublicManifest): string {
  const roster = m.roster?.length ?? 0;
  const failing = m.failing?.length ?? 0;
  const book =
    roster === 0
      ? `gate admitted 0 of ${failing} — book held cash`
      : `${roster} trading · ${failing} failing at weight 0`;
  return `${m.tape} · MOC fills · ${book}. Clone the SHA to replay.`;
}

/** Token is present but INDstocks rejected it (401 / degraded). Not the same as missing. */
export function deadINDstocksToken(health: Health | null): boolean {
  const ind = health?.indstocks;
  return Boolean(ind?.configured && ind.mode !== "live");
}

/** Hero copy when the INDstocks token is missing or dead: MOCK, never a pretty fake +8%. */
export function heroTape(health: Health | null): { label: string; value: string; hint: string } {
  if (mockTape(health)) {
    return {
      label: "Tape",
      value: "MOCK",
      hint: deadINDstocksToken(health)
        ? "INDstocks token expired or invalid. Refresh it — not live NSE."
        : "No INDstocks token. Not live NSE.",
    };
  }
  return {
    label: "Window",
    value: "Live quotes",
    hint: "INDstocks tape, paper fills",
  };
}

export function heroExcess(health: Health | null, liveExcessPct: number | undefined): string {
  if (mockTape(health)) {
    return "MOCK";
  }
  if (!Number.isFinite(liveExcessPct as number)) {
    return "—";
  }
  const n = liveExcessPct as number;
  return `${n >= 0 ? "+" : ""}${n.toFixed(2)}%`;
}

export function heroExcessHint(health: Health | null): string {
  if (deadINDstocksToken(health)) {
    return "INDstocks token expired. Not a live excess vs Nifty.";
  }
  return "Mock tape. Not a live excess vs Nifty.";
}

/**
 * Versus Nifty line. A session (Day 0) is not a year — do not print an
 * annualised rate until a full calendar day has elapsed.
 */
export function heroNiftyHint(
  health: Health | null,
  nifty: { excessAnnPct: number } | null | undefined,
  daysElapsed: number | undefined,
): string {
  if (mockTape(health)) {
    return heroExcessHint(health);
  }
  if (!nifty) {
    return "Start a campaign";
  }
  if (!Number.isFinite(daysElapsed as number) || (daysElapsed as number) < 1) {
    return "Target, not a promise · session excess, not a year";
  }
  return `Target, not a promise · annualised ${pct(nifty.excessAnnPct)}`;
}
