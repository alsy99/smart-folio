import { mockTape } from "./tape";
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

/** Hero copy when the INDstocks token is missing: MOCK, never a pretty fake +8%. */
export function heroTape(health: Health | null): { label: string; value: string; hint: string } {
  if (mockTape(health)) {
    return {
      label: "Tape",
      value: "MOCK",
      hint: "No INDstocks token. Not live NSE.",
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
