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

export type BacktestVariant = {
  strategyId: string;
  method: string;
  timeframe: string;
  params: string;
  returnPct: number;
  excessPct: number;
  winRate: number;
  trades: number;
  promoted: boolean;
  lesson: string;
};

export type BacktestReport = {
  years: number;
  variantsTested: number;
  variantsPromoted: number;
  status: string;
  ranAtUnixMs: string | number;
  niftyReturnPct: number;
  variants: BacktestVariant[];
  note: string;
};

export type PickResearch = {
  symbol: string;
  name?: string;
  strategyId?: string;
  score: number;
  direction: number;
  headline?: string;
  fundamental: string;
  technical: string;
  conclusion: string;
  risks: string;
  stance: string;
  horizon: string;
  standAside?: boolean;
  mode: string;
};

export type ResearchFeed = { picks: PickResearch[] };

export type EquityPoint = { t: string; equity: number; nifty: number };

export type Health = {
  status: string;
  indstocks?: {
    configured: boolean;
    mode: string;
    profileOk: boolean;
    scrips: number;
    orders: string;
    error?: string;
  };
};
