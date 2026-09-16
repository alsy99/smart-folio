import { describe, expect, it } from "vitest";
import { campaignTapeHint, heroExcess, heroNiftyHint, heroTape } from "./hero";
import { mockTape } from "./tape";
import type { PublicManifest } from "./types";

describe("frozen campaign tape hint", () => {
  const base = { tape: "indstocks-1d", roster: [], failing: ["a", "b"] } as unknown as PublicManifest;

  it("says the book held cash when the gate admitted nothing", () => {
    const hint = campaignTapeHint(base);
    expect(hint).toContain("indstocks-1d");
    expect(hint).toContain("admitted 0 of 2");
    expect(hint).toContain("held cash");
    expect(hint).not.toContain("prices.Last");
  });

  it("counts trading vs failing otherwise", () => {
    expect(campaignTapeHint({ ...base, roster: ["x"] })).toContain("1 trading · 2 failing at weight 0");
  });
});

describe("missing INDstocks token", () => {
  const health = {
    status: "ok" as const,
    tape: "mock" as const,
    indstocks: { configured: false, mode: "mock", profileOk: false, scrips: 0, orders: "off" },
  };

  it("shows MOCK on the hero, not a pretty fake +8%", () => {
    expect(mockTape(health)).toBe(true);
    expect(heroTape(health).value).toBe("MOCK");
    expect(heroExcess(health, 8)).toBe("MOCK");
    expect(heroExcess(health, 8)).not.toMatch(/\+8/);
  });

  it("treats empty health as mock", () => {
    expect(mockTape(null)).toBe(true);
    expect(heroTape(null).value).toBe("MOCK");
  });

  it("says expired when a token is present but INDstocks rejects it", () => {
    const dead = {
      status: "ok" as const,
      tape: "mock" as const,
      indstocks: {
        configured: true,
        mode: "degraded",
        profileOk: false,
        scrips: 15,
        orders: "compile-off",
        error: "http 401",
      },
    };
    expect(mockTape(dead)).toBe(true);
    expect(heroTape(dead).hint).toMatch(/expired or invalid/i);
    expect(heroTape(dead).hint).not.toMatch(/No INDstocks token/);
    expect(heroExcess(dead, 8)).toBe("MOCK");
  });
});

describe("Day-0 is not an annualised return", () => {
  const live = {
    status: "ok" as const,
    tape: "live" as const,
    indstocks: { configured: true, mode: "live", profileOk: true, scrips: 15, orders: "off" },
  };
  const nifty = { excessAnnPct: 847 };

  it("calls a session move session excess, not a year", () => {
    const hint = heroNiftyHint(live, nifty, 0);
    expect(hint).toMatch(/session excess/i);
    expect(hint).not.toMatch(/annualis/i);
    expect(hint).not.toMatch(/847/);
  });

  it("may annualise after a full day", () => {
    expect(heroNiftyHint(live, nifty, 1)).toMatch(/annualised \+847\.00%/);
  });
});
