import { describe, expect, it } from "vitest";
import { labNote, ROSTER_UNKNOWN, rosterIsDefault, rosterLine } from "./roster";
import type { BacktestReport } from "./types";

function report(note: string): BacktestReport {
  return { years: 5, variantsTested: 0, variantsPromoted: 0, status: "idle", ranAtUnixMs: 0, niftyReturnPct: 0, variants: [], note };
}

describe("roster provenance on the hero", () => {
  it("no file ⇒ says default daily specs, out loud", () => {
    const r = report("Roster: default daily specs (no data/roster snapshot)\nRun a 5-year backtest on real daily bars.");
    expect(rosterLine(r)).toBe("Roster: default daily specs (no data/roster snapshot)");
    expect(rosterIsDefault(r)).toBe(true);
    expect(labNote(r)).toBe("Run a 5-year backtest on real daily bars.");
  });

  it("snapshot ⇒ names file, tape, closes and admissions", () => {
    const r = report("Roster: data/roster/2026-09-15.json · indstocks-1d · 1240 closes · 0 admitted · 6 defaults failing gate\nWalk-forward…");
    expect(rosterLine(r)).toMatch(/indstocks-1d/);
    expect(rosterIsDefault(r)).toBe(false);
  });

  it("nothing from learning yet ⇒ unknown, never a fake vetted roster", () => {
    expect(rosterLine(null)).toBe(ROSTER_UNKNOWN);
    expect(rosterLine(report("Run the backtest."))).toBe(ROSTER_UNKNOWN);
    expect(labNote(report("Run the backtest."))).toBe("Run the backtest.");
  });
});
