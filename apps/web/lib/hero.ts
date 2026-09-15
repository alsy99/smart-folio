import { mockTape } from "./tape";
import type { Health } from "./types";

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
