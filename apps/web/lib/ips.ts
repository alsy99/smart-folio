/**
 * Investment Policy Statement, desk side. Every field is an enum, a
 * bounded number, or a boolean. There is no free-text field, so
 * "guaranteed 10%" has nowhere to be typed; the server re-validates.
 */

export const GOALS = ["beat_nifty", "beat_inflation", "max_dd"] as const;
export type Goal = (typeof GOALS)[number];

export const HORIZONS = [1, 3, 5, 10] as const;
export type Horizon = (typeof HORIZONS)[number];

export const BENCHMARKS = ["NIFTY50", "NIFTY500", "SENSEX"] as const;
export type Benchmark = (typeof BENCHMARKS)[number];

export const REBALANCES = ["monthly", "quarterly"] as const;
export type Rebalance = (typeof REBALANCES)[number];

/** v1 walls. Mirrors pkg/ips; the server is the authority. */
export const MAX_DD_CAP = 0.15;
export const CORE_MIN = 0.8;
export const CORE_MAX = 1.0;

export const GOAL_LABEL: Record<Goal, string> = {
  beat_nifty: "Beat Nifty 50 after costs",
  beat_inflation: "Beat inflation",
  max_dd: "Stay inside a drawdown cap",
};

export const BENCH_LABEL: Record<Benchmark, string> = {
  NIFTY50: "Nifty 50",
  NIFTY500: "Nifty 500",
  SENSEX: "S&P BSE Sensex",
};

export type IPSForm = {
  id: string;
  goal: Goal;
  horizonYears: Horizon;
  /** 0 < maxDd ≤ 0.15 */
  maxDd: number;
  benchmark: Benchmark;
  /** 0.80 ≤ corePct ≤ 1.00; satellite is derived. */
  corePct: number;
  rebalance: Rebalance;
  startCash: number;
  foldSatellite: boolean;
};

/** Wire shape, matches proto policy.v1.IPS (protojson). */
export type IPS = IPSForm & {
  satellitePct: number;
  hash?: string;
  line?: string;
};

export const DEFAULT_FORM: IPSForm = {
  id: "",
  goal: "beat_nifty",
  horizonYears: 5,
  maxDd: 0.15,
  benchmark: "NIFTY50",
  corePct: 1,
  rebalance: "monthly",
  startCash: 1_000_000,
  foldSatellite: true,
};

export function isGoal(v: unknown): v is Goal {
  return typeof v === "string" && (GOALS as readonly string[]).includes(v);
}
export function isBenchmark(v: unknown): v is Benchmark {
  return typeof v === "string" && (BENCHMARKS as readonly string[]).includes(v);
}
export function isHorizon(v: unknown): v is Horizon {
  return typeof v === "number" && (HORIZONS as readonly number[]).includes(v);
}
export function isRebalance(v: unknown): v is Rebalance {
  return typeof v === "string" && (REBALANCES as readonly string[]).includes(v);
}

/**
 * Turn form state into the wire IPS, or explain why not. Never rounds a
 * value into range: a 20% cap is refused, not clamped to 15%.
 */
export function buildIPS(f: IPSForm): { ips: IPS; error: null } | { ips: null; error: string } {
  const id = f.id.trim();
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(id)) return { ips: null, error: "Client id: letters, digits, . _ - only." };
  if (!isGoal(f.goal)) return { ips: null, error: "Pick a goal from the list. Return promises are not goals." };
  if (!isHorizon(f.horizonYears)) return { ips: null, error: "Horizon must be 1, 3, 5 or 10 years." };
  if (!(f.maxDd > 0 && f.maxDd <= MAX_DD_CAP + 1e-12)) return { ips: null, error: "Drawdown cap must be between 0 and 15%." };
  if (!isBenchmark(f.benchmark)) return { ips: null, error: "Pick a benchmark." };
  if (!(f.corePct >= CORE_MIN - 1e-12 && f.corePct <= CORE_MAX + 1e-12)) return { ips: null, error: "Core must be 80–100% of the book." };
  if (!isRebalance(f.rebalance)) return { ips: null, error: "Rebalance must be monthly or quarterly." };
  if (!(f.startCash > 0)) return { ips: null, error: "Start cash must be positive." };
  const corePct = Math.round(f.corePct * 100) / 100;
  const satellitePct = Math.round((1 - corePct) * 100) / 100;
  return {
    ips: {
      id,
      goal: f.goal,
      horizonYears: f.horizonYears,
      maxDd: Math.round(f.maxDd * 1000) / 1000,
      benchmark: f.benchmark,
      corePct,
      satellitePct,
      rebalance: f.rebalance,
      startCash: f.startCash,
      foldSatellite: f.foldSatellite,
    },
    error: null,
  };
}

/** One-liner for the hero when the server has not supplied one. */
export function ipsLine(p: IPS | null | undefined): string {
  if (!p) return "No IPS yet. Set a goal and a pain limit to start a paper book.";
  if (p.line) return p.line;
  return `${GOAL_LABEL[p.goal] ?? p.goal} · ${p.horizonYears}y · DD cap ${Math.round(p.maxDd * 100)}% · vs ${BENCH_LABEL[p.benchmark] ?? p.benchmark} · core ${Math.round(p.corePct * 100)}% / satellite ${Math.round(p.satellitePct * 100)}%`;
}

export const IPS_STORAGE_KEY = "aperture.ipsId";
