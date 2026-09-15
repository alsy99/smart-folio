import { describe, expect, it } from "vitest";
import { campaignTapeHint, heroExcess, heroTape } from "./hero";
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
});
