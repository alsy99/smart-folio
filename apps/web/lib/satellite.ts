import type { Weight } from "./types";

/** Desk copy. Fixed strings so tests and the spec agree word for word. */
export const SATELLITE_EMPTY = "SATELLITE EMPTY";
export const SATELLITE_EMPTY_WHY = "No method cleared walk-forward after delivery costs.";
export const NO_CORE_YET = "No IPS core yet — the book is CASH.";
export const TARGET_NOT_PROMISE = "+10pp vs Nifty is a target, not a promise.";

/**
 * The satellite is empty when learning has answered and nothing carries
 * weight. `undefined`/`[]` means learning has not spoken, which is not the
 * same as an empty roster and is reported as unknown by the caller.
 */
export function satelliteEmpty(weights: Weight[] | null | undefined): boolean {
  if (!weights || weights.length === 0) return false;
  return weights.every((w) => !(w.weight > 0));
}

/** Methods that may actually trade: weight > 0 and not tagged failing. */
export function liveMethods(weights: Weight[] | null | undefined): Weight[] {
  return (weights ?? []).filter((w) => w.weight > 0 && w.regime !== "failing-gate");
}

/** One-line satellite state for the hero stat and the holdings panel. */
export function satelliteState(
  weights: Weight[] | null | undefined,
  hasCore: boolean,
): { value: string; hint: string } {
  if (satelliteEmpty(weights)) {
    return {
      value: SATELLITE_EMPTY,
      hint: hasCore ? SATELLITE_EMPTY_WHY : `${SATELLITE_EMPTY_WHY} ${NO_CORE_YET}`,
    };
  }
  const live = liveMethods(weights);
  if (live.length === 0) {
    return { value: "—", hint: "Awaiting learning service." };
  }
  return {
    value: `${live.length} method${live.length === 1 ? "" : "s"}`,
    hint: `Gated in on indstocks-1d: ${live.map((w) => w.strategyId).join(", ")}`,
  };
}
