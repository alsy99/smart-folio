import { describe, expect, it } from "vitest";
import { heroExcess, heroTape } from "./hero";
import { mockTape } from "./tape";

describe("missing INDstocks token", () => {
  const health = { status: "ok" as const, tape: "mock" as const, indstocks: { configured: false, mode: "mock" } };

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
