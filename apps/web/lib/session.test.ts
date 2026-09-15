import { describe, expect, it } from "vitest";
import { fillsFromTick, fillAllowed } from "./session";

const closed = { marketOpen: false, sessionStatus: "closed" as const };
const weekend = { marketOpen: false, sessionStatus: "weekend" as const };
const open = { marketOpen: true, sessionStatus: "open" as const };

describe("market closed ⇒ no fill", () => {
  it("refuses fills when the cash session is closed", () => {
    expect(fillAllowed(closed)).toBe(false);
    expect(
      fillsFromTick(closed, { skipped: false, fills: 6, reason: "" })
    ).toBe(0);
  });

  it("refuses fills on the weekend", () => {
    expect(fillAllowed(weekend)).toBe(false);
    expect(fillsFromTick(weekend, { skipped: false, fills: 3, reason: "" })).toBe(0);
  });

  it("refuses a skipped closed-market tick even if fills>0", () => {
    expect(
      fillsFromTick(open, { skipped: true, fills: 4, reason: "market closed" })
    ).toBe(0);
  });

  it("counts fills only while the session is open", () => {
    expect(fillAllowed(open)).toBe(true);
    expect(fillsFromTick(open, { skipped: false, fills: 2, reason: "" })).toBe(2);
  });
});
