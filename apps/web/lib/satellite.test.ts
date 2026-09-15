import { describe, expect, it } from "vitest";
import { liveMethods, satelliteEmpty, satelliteState, SATELLITE_EMPTY } from "./satellite";
import type { Weight } from "./types";

const failing = (id: string): Weight => ({
  strategyId: id,
  weight: 0,
  expectancy: 0,
  winRate: 0,
  regime: "failing-gate",
});

describe("empty satellite roster", () => {
  // data/roster/2026-09-15.json: roster [], six defaults failing at weight 0.
  const weights = ["sentiment_tilt", "breakout_1d", "swing_daily", "sma_cross_1d", "mean_reversion_1d", "momentum_1d"].map(
    failing,
  );

  it("is EMPTY and CASH, never a live method", () => {
    expect(satelliteEmpty(weights)).toBe(true);
    expect(liveMethods(weights)).toEqual([]);
    const s = satelliteState(weights, false);
    expect(s.value).toBe(SATELLITE_EMPTY);
    expect(s.value).toMatch(/EMPTY/);
    expect(s.hint).toMatch(/CASH/);
    expect(s.hint).not.toMatch(/sma|breakout/i);
  });

  it("with a core, says why but not CASH", () => {
    const s = satelliteState(weights, true);
    expect(s.value).toMatch(/EMPTY/);
    expect(s.hint).not.toMatch(/CASH/);
  });

  it("does not call an unanswered desk empty", () => {
    expect(satelliteEmpty(undefined)).toBe(false);
    expect(satelliteEmpty([])).toBe(false);
    expect(satelliteState([], false).value).toBe("—");
  });

  it("lists only gated-in methods as live", () => {
    const w = [...weights, { strategyId: "x_1d", weight: 1, expectancy: 0, winRate: 0.5, regime: "mixed" }];
    expect(satelliteEmpty(w)).toBe(false);
    expect(liveMethods(w).map((x) => x.strategyId)).toEqual(["x_1d"]);
    expect(satelliteState(w, true).value).toBe("1 method");
  });
});
