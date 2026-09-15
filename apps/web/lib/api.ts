import type {
  BacktestReport,
  Benchmarks,
  Campaign,
  Health,
  Investigations,
  Journal,
  NewsFeed,
  Portfolio,
  ResearchFeed,
  Sentiment,
  TickResult,
  Weight,
} from "./types";

export * from "./types";

const API = process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(`${API}${path}`, { cache: "no-store" });
  if (!r.ok) throw new Error(`${path} ${r.status}`);
  return r.json();
}

async function post<T>(path: string, body?: unknown, timeoutMs = 20000): Promise<T> {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const r = await fetch(`${API}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body ?? {}),
      signal: ctrl.signal,
    });
    if (!r.ok) throw new Error(`${path} ${r.status}`);
    return r.json();
  } finally {
    clearTimeout(t);
  }
}

export const api = {
  health: () => get<Health>("/health"),
  portfolio: () => get<Portfolio>("/portfolio"),
  campaign: () => get<Campaign>("/campaign"),
  startCampaign: (days = 30) => post<Campaign>("/campaign", { days }),
  setAutopilot: (enabled: boolean) => post<{ enabled: boolean }>("/autopilot", { enabled }),
  benchmarks: () => get<Benchmarks>("/benchmarks"),
  tick: () => post<TickResult>("/paper/tick"),
  journal: () => get<Journal>("/journal"),
  weights: () => get<{ weights: Weight[] }>("/weights"),
  sentiment: () => get<{ scores: Sentiment[] }>("/sentiment"),
  news: () => get<NewsFeed>("/news"),
  investigations: () => get<Investigations>("/investigations"),
  research: () => get<ResearchFeed>("/research"),
  chat: (message: string) => post<{ reply: string; mode: string }>("/advisor/chat", { message }),
  backtest: () => get<BacktestReport>("/backtest"),
  runBacktest: (years = 5) => post<BacktestReport>("/backtest", { years }, 90000),
};
