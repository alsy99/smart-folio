const API = process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080";

async function get<T>(path: string): Promise<T> {
  const r = await fetch(`${API}${path}`, { cache: "no-store" });
  if (!r.ok) throw new Error(`${path} ${r.status}`);
  return r.json();
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const r = await fetch(`${API}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!r.ok) throw new Error(`${path} ${r.status}`);
  return r.json();
}

export const api = {
  health: () => get<{ status: string }>("/health"),
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
  chat: (message: string) => post<{ reply: string; mode: string }>("/advisor/chat", { message }),
};

export type Position = {
  symbol: string;
  qty: number;
  avgPrice: number;
  last: number;
  marketValue: number;
  pnl: number;
  weight: number;
};

export type Weight = {
  strategyId: string;
  weight: number;
  expectancy: number;
  winRate: number;
  regime: string;
};

export type Portfolio = {
  cash: number;
  equity: number;
  startEquity: number;
  returnPct: number;
  positions: Position[];
  openTrades: OpenTrade[];
  weights: Weight[];
};

export type OpenTrade = {
  id: string;
  symbol: string;
  side: string;
  strategyId: string;
  qty: number;
  entry: number;
  open: boolean;
};

export type Campaign = {
  active: boolean;
  autopilot: boolean;
  marketOpen: boolean;
  startedAtUnixMs: string | number;
  endsAtUnixMs: string | number;
  daysElapsed: number;
  daysTotal: number;
  sessionStatus: string;
  ticks: number;
  clockOverride: boolean;
};

export type Benchmark = {
  id: string;
  name: string;
  last: number;
  start: number;
  returnPct: number;
  excessPct: number;
  excessAnnPct: number;
  onTrack: boolean;
  beatRate: number;
};

export type Benchmarks = {
  benchmarks: Benchmark[];
  portfolioReturnPct: number;
};

export type TickResult = {
  skipped: boolean;
  reason: string;
  fills: number;
  closes: number;
  equity: number;
};

export type JournalEntry = {
  id: string;
  lesson: string;
  tags: string[];
  tsUnixMs: string | number;
  trade?: {
    id: string;
    symbol: string;
    strategyId: string;
    pnl: number;
    pnlPct: number;
    excessReturn: number;
    niftyReturn: number;
  };
};

export type Journal = { entries: JournalEntry[] };

export type Sentiment = {
  symbol: string;
  score: number;
  label: string;
  confidence: number;
  drivers: string[];
};

export type NewsItem = {
  id: string;
  provider: string;
  title: string;
  summary: string;
  url: string;
  symbols: string[];
};

export type NewsFeed = {
  items: NewsItem[];
  mode: string;
  lastRefreshUnixMs: string | number;
};

export type Investigation = {
  id: string;
  headline: string;
  symbols: string[];
  eventType: string;
  stance: string;
  score: number;
  confidence: number;
  corroboration: number;
  horizon: string;
  strategyImplications: { strategyId: string; tilt: number; reason: string }[];
  standAside: boolean;
  thesis: string;
  risks: string;
  status: string;
  mode: string;
};

export type Investigations = {
  reports: Investigation[];
  workersActive: number;
  workersMax: number;
  lastRefreshUnixMs: string | number;
  mode: string;
};
