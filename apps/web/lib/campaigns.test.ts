import { describe, expect, it } from "vitest";
import { lastOpen } from "./campaigns";
import type { PublicCampaign } from "./types";

describe("frozen campaign list", () => {
  it("uses the last open session, not a weekend print", () => {
    const c = {
      manifest: { name: "public-30d-core" },
      days: [
        { date: "2026-09-11", session: "open", equity: 1, excessNifty50Pct: 0.1 },
        { date: "2026-09-12", session: "weekend", equity: 1, excessNifty50Pct: 0.1 },
      ],
    } as unknown as PublicCampaign;
    expect(lastOpen(c)?.date).toBe("2026-09-11");
  });
});
