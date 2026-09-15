import type { BacktestReport } from "./types";

/** Hero line when the learning service has said nothing yet. */
export const ROSTER_UNKNOWN = "Roster: awaiting learning service";

/**
 * The learning service prefixes every backtest note with one line,
 * `Roster: …`, naming where the live roster came from — a dated
 * data/roster snapshot (tape, closes, admissions, failing defaults) or
 * "default daily specs (no data/roster snapshot)". The hero prints it so
 * a default book is never mistaken for a lab-vetted one.
 */
export function rosterLine(report: BacktestReport | null | undefined): string {
  const note = report?.note ?? "";
  const first = note.split("\n")[0]?.trim() ?? "";
  if (first.startsWith("Roster:")) return first;
  return ROSTER_UNKNOWN;
}

/** True when the desk booted on shipped defaults with no snapshot file. */
export function rosterIsDefault(report: BacktestReport | null | undefined): boolean {
  return /default daily specs/i.test(rosterLine(report));
}

/** The note body below the roster line, for the lab panel. */
export function labNote(report: BacktestReport | null | undefined): string {
  const note = report?.note ?? "";
  const lines = note.split("\n");
  if (lines[0]?.trim().startsWith("Roster:")) return lines.slice(1).join("\n").trim();
  return note.trim();
}
