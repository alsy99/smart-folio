import { describe, expect, it } from "vitest";
import { buildIPS, DEFAULT_FORM, deskIPSLine, ipsLine, type IPSForm } from "./ips";

const good: IPSForm = { ...DEFAULT_FORM, id: "c-1" };

describe("IPS form", () => {
  it("builds a valid default statement with a derived satellite", () => {
    const r = buildIPS(good);
    expect(r.error).toBeNull();
    expect(r.ips?.satellitePct).toBe(0);
    const r2 = buildIPS({ ...good, corePct: 0.85 });
    expect(r2.ips?.satellitePct).toBeCloseTo(0.15, 9);
  });

  it("cannot submit a return promise as a goal", () => {
    // The goal is an enum; a free string is a type error at compile time
    // and a validation error at runtime.
    const r = buildIPS({ ...good, goal: "guaranteed 10%" as unknown as IPSForm["goal"] });
    expect(r.ips).toBeNull();
    expect(r.error).toMatch(/promise/i);
    expect(JSON.stringify(r)).not.toMatch(/"guaranteed 10%"/);
  });

  it("refuses walls instead of clamping", () => {
    expect(buildIPS({ ...good, maxDd: 0.2 }).error).toMatch(/15%/);
    expect(buildIPS({ ...good, maxDd: 0 }).error).toMatch(/15%/);
    expect(buildIPS({ ...good, corePct: 0.5 }).error).toMatch(/80–100%/);
    expect(buildIPS({ ...good, benchmark: "" as unknown as IPSForm["benchmark"] }).error).toMatch(/benchmark/i);
    expect(buildIPS({ ...good, horizonYears: 2 as unknown as IPSForm["horizonYears"] }).error).toMatch(/horizon/i);
    expect(buildIPS({ ...good, id: "../x" }).error).toMatch(/id/i);
  });

  it("has no free-text field on the wire", () => {
    const r = buildIPS(good);
    const keys = Object.keys(r.ips ?? {}).sort();
    expect(keys).toEqual(
      ["benchmark", "corePct", "foldSatellite", "goal", "horizonYears", "id", "maxDd", "rebalance", "satellitePct", "startCash"].sort(),
    );
  });

  it("hero line says target, never promise", () => {
    expect(ipsLine(null)).toMatch(/No IPS yet/);
    const line = ipsLine(buildIPS(good).ips);
    expect(line).toMatch(/core 100%/);
    expect(line).not.toMatch(/guarantee|promise/i);
  });

  it("GET line is what every client paints; holdings are never CASH", () => {
    const server = { ...buildIPS(good).ips!, line: "beat_nifty · 5y · DD cap 15% · vs NIFTY50 · core 100% / satellite 0% · monthly" };
    expect(deskIPSLine(server, undefined, false)).toBe(server.line);
    expect(deskIPSLine(null, server.line, true)).toBe(server.line);
    expect(deskIPSLine(null, undefined, true)).toBe("Core is invested.");
    expect(deskIPSLine(null, undefined, true)).not.toMatch(/No IPS|CASH/i);
    expect(deskIPSLine(null, undefined, false)).toMatch(/No IPS yet/);
  });
});
