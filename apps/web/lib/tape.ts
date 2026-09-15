export function mockTape(health: { tape?: string; indstocks?: { mode?: string; configured?: boolean } } | null) {
  if (!health) return true;
  if (health.tape === "live") return false;
  if (health.tape === "mock") return true;
  return health.indstocks?.mode !== "live" || !health.indstocks?.configured;
}
