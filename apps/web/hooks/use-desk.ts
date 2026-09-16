"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { IPS_STORAGE_KEY, type IPS } from "@/lib/ips";
import type {
  BacktestReport,
  Benchmarks,
  Campaign,
  EquityPoint,
  Health,
  Investigations,
  Journal,
  NewsFeed,
  Portfolio,
  PickResearch,
  PublicCampaign,
  Sentiment,
  Targets,
} from "@/lib/types";

const POLL_MS = 4000;

export function useDesk() {
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [portfolio, setPortfolio] = useState<Portfolio | null>(null);
  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [benchmarks, setBenchmarks] = useState<Benchmarks | null>(null);
  const [journal, setJournal] = useState<Journal | null>(null);
  const [investigations, setInvestigations] = useState<Investigations | null>(null);
  const [news, setNews] = useState<NewsFeed | null>(null);
  const [picks, setPicks] = useState<PickResearch[]>([]);
  const [sentiment, setSentiment] = useState<Sentiment[]>([]);
  const [history, setHistory] = useState<EquityPoint[]>([]);
  const [backtest, setBacktest] = useState<BacktestReport | null>(null);
  const [btBusy, setBtBusy] = useState(false);
  const [health, setHealth] = useState<Health | null>(null);
  const [pub, setPub] = useState<PublicCampaign | null>(null);
  const [campaigns, setCampaigns] = useState<PublicCampaign[]>([]);
  const [ips, setIPS] = useState<IPS | null>(null);
  const [ipsMissing, setIPSMissing] = useState(false);
  const [targets, setTargets] = useState<Targets | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [p, c, b, j, inv, n, s, bt, rs, h, pubCamp, pubs] = await Promise.all([
        api.portfolio(),
        api.campaign(),
        api.benchmarks(),
        api.journal(),
        api.investigations(),
        api.news(),
        api.sentiment(),
        api.backtest(),
        api.research().catch(() => ({ picks: [] as PickResearch[] })),
        api.health().catch(() => null),
        api.publicCampaign().catch(() => null),
        api.publicCampaigns().catch(() => ({ campaigns: [] as PublicCampaign[] })),
      ]);
      setPortfolio(p);
      setCampaign(c);
      setBenchmarks(b);
      setJournal(j);
      setInvestigations(inv);
      setNews(n);
      setPicks(rs.picks || []);
      setSentiment(s.scores || []);
      setBacktest(bt);
      setHealth(h);
      setPub(pubCamp);
      setCampaigns(pubs?.campaigns ?? []);
      setErr(null);
      // The IPS id lives on this browser session; the statement lives on
      // the policy service. No id yet is a first run, not an error.
      const id = typeof window !== "undefined" ? window.localStorage.getItem(IPS_STORAGE_KEY) : null;
      if (id) {
        const cur = await api.ips(id).catch(() => null);
        setIPS(cur);
        setIPSMissing(!cur);
        setTargets(cur ? await api.previewTargets(id).catch(() => null) : null);
      } else {
        setIPS(null);
        setIPSMissing(true);
        setTargets(null);
      }
      // Live paper marks, not the frozen public ledger. Published days live
      // on the Campaign tab so a new IPS is not drawn as last month's cash.
      const niftyPt = b.benchmarks?.find((x) => x.id === "NIFTY50");
      setHistory((prev) => {
        const next = [
          ...prev,
          {
            t: new Date().toLocaleTimeString("en-IN", {
              hour: "2-digit",
              minute: "2-digit",
              second: "2-digit",
            }),
            equity: p.equity,
            nifty: niftyPt ? niftyPt.start * (1 + niftyPt.returnPct / 100) : 0,
          },
        ];
        return next.slice(-24);
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "gateway unreachable");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, POLL_MS);
    return () => clearInterval(id);
  }, [refresh]);

  const nifty = benchmarks?.benchmarks?.find((x) => x.id === "NIFTY50");
  const onTrack = Boolean(nifty?.onTrack);
  const positions = useMemo(
    () => (portfolio?.positions || []).filter((p) => p.qty > 0),
    [portfolio]
  );

  async function start() {
    setBusy(true);
    try {
      await api.startCampaign(30);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "start failed");
    } finally {
      setBusy(false);
    }
  }

  async function kill() {
    setBusy(true);
    try {
      await api.setAutopilot(false);
      await refresh();
    } finally {
      setBusy(false);
    }
  }

  async function resume() {
    setBusy(true);
    try {
      await api.setAutopilot(true);
      await refresh();
    } finally {
      setBusy(false);
    }
  }

  async function saveIPS(next: IPS) {
    const saved = await api.putIPS(next);
    window.localStorage.setItem(IPS_STORAGE_KEY, saved.id);
    setIPS(saved);
    setIPSMissing(false);
  }

  async function runBacktest() {
    setBtBusy(true);
    try {
      const r = await api.runBacktest(5);
      setBacktest(r);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "backtest failed");
    } finally {
      setBtBusy(false);
    }
  }

  return {
    err,
    loading,
    busy,
    btBusy,
    portfolio,
    campaign,
    benchmarks,
    journal,
    investigations,
    news,
    picks,
    sentiment,
    history,
    backtest,
    health,
    pub,
    campaigns,
    nifty,
    onTrack,
    positions,
    start,
    kill,
    resume,
    runBacktest,
    ips,
    ipsMissing,
    targets,
    saveIPS,
  };
}
