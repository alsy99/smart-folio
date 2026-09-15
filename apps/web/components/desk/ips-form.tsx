"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  BENCH_LABEL,
  BENCHMARKS,
  buildIPS,
  CORE_MAX,
  CORE_MIN,
  DEFAULT_FORM,
  GOAL_LABEL,
  GOALS,
  HORIZONS,
  isBenchmark,
  isGoal,
  isRebalance,
  MAX_DD_CAP,
  REBALANCES,
  type IPS,
  type IPSForm,
} from "@/lib/ips";
import { TARGET_NOT_PROMISE } from "@/lib/satellite";

const selectCls =
  "flex h-10 w-full min-w-0 border border-rule bg-blotter px-3 text-sm text-ink focus-visible:border-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink";

export function IPSForm({
  current,
  frozen,
  onSave,
}: {
  current: IPS | null;
  frozen: boolean;
  onSave: (ips: IPS) => Promise<void>;
}) {
  const [form, setForm] = useState<IPSForm>(() => (current ? { ...current } : { ...DEFAULT_FORM }));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const built = buildIPS(form);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (built.ips === null) {
      setError(built.error);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSave(built.ips);
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  const set = <K extends keyof IPSForm>(k: K, v: IPSForm[K]) => setForm((f) => ({ ...f, [k]: v }));

  return (
    <form onSubmit={submit} data-testid="ips-form" className="max-w-2xl space-y-5">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">Investment Policy Statement</h2>
        <p className="mt-1 text-sm text-steel">
          A goal and a pain limit. The core stays invested against your benchmark unless your drawdown cap is hit; the
          satellite is empty until a method clears the gate. {TARGET_NOT_PROMISE}
        </p>
      </div>

      <label className="block text-sm">
        <span className="text-steel">Client id</span>
        <Input
          value={form.id}
          onChange={(e) => set("id", e.target.value)}
          placeholder="e.g. c-1"
          disabled={Boolean(current) || frozen}
          className="mt-1"
          pattern="[A-Za-z0-9][A-Za-z0-9._-]{0,63}"
          required
        />
      </label>

      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block text-sm">
          <span className="text-steel">Goal</span>
          <select
            className={`${selectCls} mt-1`}
            value={form.goal}
            disabled={frozen}
            onChange={(e) => isGoal(e.target.value) && set("goal", e.target.value)}
          >
            {GOALS.map((g) => (
              <option key={g} value={g}>
                {GOAL_LABEL[g]}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm">
          <span className="text-steel">Horizon</span>
          <select
            className={`${selectCls} mt-1`}
            value={form.horizonYears}
            disabled={frozen}
            onChange={(e) => {
              const n = Number(e.target.value);
              if (n === 1 || n === 3 || n === 5 || n === 10) set("horizonYears", n);
            }}
          >
            {HORIZONS.map((h) => (
              <option key={h} value={h}>
                {h} year{h === 1 ? "" : "s"}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm">
          <span className="text-steel">Benchmark</span>
          <select
            className={`${selectCls} mt-1`}
            value={form.benchmark}
            disabled={frozen}
            onChange={(e) => isBenchmark(e.target.value) && set("benchmark", e.target.value)}
          >
            {BENCHMARKS.map((b) => (
              <option key={b} value={b}>
                {BENCH_LABEL[b]}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm">
          <span className="text-steel">Rebalance</span>
          <select
            className={`${selectCls} mt-1`}
            value={form.rebalance}
            disabled={frozen}
            onChange={(e) => isRebalance(e.target.value) && set("rebalance", e.target.value)}
          >
            {REBALANCES.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label className="block text-sm">
        <span className="flex justify-between text-steel">
          <span>Max drawdown (pain limit)</span>
          <span className="num text-ink">{Math.round(form.maxDd * 100)}%</span>
        </span>
        <input
          type="range"
          min={1}
          max={Math.round(MAX_DD_CAP * 100)}
          step={1}
          value={Math.round(form.maxDd * 100)}
          disabled={frozen}
          onChange={(e) => set("maxDd", Number(e.target.value) / 100)}
          className="mt-2 w-full accent-ink"
          aria-label="Max drawdown percent, capped at 15"
        />
        <span className="text-xs text-steel">Capped at {Math.round(MAX_DD_CAP * 100)}%. Hitting it liquidates to cash until the next rebalance.</span>
      </label>

      <label className="block text-sm">
        <span className="flex justify-between text-steel">
          <span>Core / satellite</span>
          <span className="num text-ink">
            {Math.round(form.corePct * 100)}% / {Math.round((1 - form.corePct) * 100)}%
          </span>
        </span>
        <input
          type="range"
          min={Math.round(CORE_MIN * 100)}
          max={Math.round(CORE_MAX * 100)}
          step={1}
          value={Math.round(form.corePct * 100)}
          disabled={frozen}
          onChange={(e) => set("corePct", Number(e.target.value) / 100)}
          className="mt-2 w-full accent-ink"
          aria-label="Core percent, 80 to 100"
        />
        <span className="text-xs text-steel">Satellite is at most 20% and stays empty until a method clears the gate.</span>
      </label>

      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={form.foldSatellite}
          disabled={frozen}
          onChange={(e) => set("foldSatellite", e.target.checked)}
        />
        Fold an empty satellite into the core at rebalance
      </label>

      <label className="block text-sm">
        <span className="text-steel">Start cash (₹)</span>
        <Input
          type="number"
          min={1}
          step={1000}
          value={form.startCash}
          disabled={Boolean(current) || frozen}
          onChange={(e) => set("startCash", Number(e.target.value))}
          className="mt-1"
        />
      </label>

      {(error || built.error) && (
        <p role="alert" className="text-sm font-semibold text-down">
          {error ?? built.error}
        </p>
      )}
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={saving || frozen || Boolean(built.error)}>
          {saving ? "Saving…" : current ? "Update policy" : "Create policy"}
        </Button>
        {frozen && <span className="text-sm text-steel">Frozen campaign: the statement cannot change mid-window.</span>}
        {current?.hash && <span className="font-mono text-xs text-steel">hash {current.hash}</span>}
      </div>
    </form>
  );
}
