import type { Targets } from "./types";

/** Desk copy for the core sleeve. */
export const CORE_STAYS_INVESTED = "Core stays invested unless your drawdown cap is hit.";

/** Rows for the Book "core vs target" table, largest drift first. */
export function coreRows(t: Targets | null | undefined) {
  if (!t?.core?.length) return [];
  return [...t.core].sort((a, b) => Math.abs(b.drift) - Math.abs(a.drift) || a.symbol.localeCompare(b.symbol));
}

/** How far the whole core sits from target, in weight terms. */
export function coreInvested(t: Targets | null | undefined): { actual: number; target: number } {
  if (!t?.core?.length) return { actual: 0, target: t?.corePct ?? 0 };
  const actual = t.core.reduce((s, c) => s + (c.actual || 0), 0);
  return { actual, target: t.corePct };
}

/** One line for the calendar: next rebalance and whether today is it. */
export function rebalanceLine(t: Targets | null | undefined, rebalance?: string): string {
  if (!t) return "No IPS yet — no rebalance calendar.";
  const cadence = rebalance ? `${rebalance} ` : "";
  if (t.rebalanceSession) return `Today is a ${cadence}rebalance session. ${t.reason}`;
  return t.nextRebalance ? `Next ${cadence}rebalance ${t.nextRebalance} (last NSE session of the period).` : t.reason;
}
