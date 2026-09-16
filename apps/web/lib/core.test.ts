import { describe, expect, it } from "vitest";
import { coreInvested, coreRows, HALT_HOLDS, rebalanceLine } from "./core";
import type { Targets } from "./types";

const t: Targets = {
  ipsId: "c-1",
  ipsHash: "abc",
  asOfUnixMs: 0,
  core: [
    { symbol: "TCS", target: 0.08, actual: 0.0893, drift: 0.0093, ticket: false },
    { symbol: "ITC", target: 0.08, actual: 0.0775, drift: -0.0025, ticket: false },
    { symbol: "SBIN", target: 0.05, actual: 0.0, drift: -0.05, ticket: true },
  ],
  corePct: 0.81,
  satelliteCap: 0,
  cash: 0.19,
  reason: "not a rebalance session; next 2026-09-30",
  nextRebalance: "2026-09-30",
  rebalanceSession: false,
};

describe("core desk helpers", () => {
  it("sorts by absolute drift", () => {
    expect(coreRows(t).map((r) => r.symbol)).toEqual(["SBIN", "TCS", "ITC"]);
    expect(coreRows(null)).toEqual([]);
  });
  it("sums invested weight against the rails-trimmed target", () => {
    const { actual, target } = coreInvested(t);
    expect(actual).toBeCloseTo(0.1668, 4);
    expect(target).toBe(0.81);
  });
  it("names the next NSE session", () => {
    expect(rebalanceLine(t, "monthly")).toMatch(/2026-09-30/);
    expect(rebalanceLine({ ...t, rebalanceSession: true }, "monthly")).toMatch(/Today/);
    expect(rebalanceLine(null)).toMatch(/No IPS/);
  });
  it("says halt is a buy stop, not a 15% floor on marks", () => {
    expect(HALT_HOLDS).toBe("Halt = no new buys. Marks can still go through 15%.");
  });
});
