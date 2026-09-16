"use client";

import { useRef } from "react";
import { cn } from "@/lib/utils";

export type DeskTab = "book" | "policy" | "campaign" | "research" | "lab";

const TABS: { id: DeskTab; label: string }[] = [
  { id: "book", label: "Book" },
  { id: "policy", label: "Policy" },
  { id: "campaign", label: "Campaign" },
  { id: "research", label: "Research" },
  { id: "lab", label: "Lab" },
];

export function isDeskTab(value: string | null): value is DeskTab {
  return value === "book" || value === "policy" || value === "campaign" || value === "research" || value === "lab";
}

export function DeskTabs({
  value,
  onChange,
  counts,
}: {
  value: DeskTab;
  onChange: (id: DeskTab) => void;
  counts?: Partial<Record<DeskTab, number>>;
}) {
  const refs = useRef<Array<HTMLButtonElement | null>>([]);

  function move(from: number, delta: number) {
    const next = (from + delta + TABS.length) % TABS.length;
    onChange(TABS[next].id);
    refs.current[next]?.focus();
  }

  return (
    <div role="tablist" aria-label="Desk sections" aria-orientation="vertical" className="flex flex-col gap-1">
      {TABS.map((t, i) => {
        const active = value === t.id;
        const n = counts?.[t.id];
        return (
          <button
            key={t.id}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`tab-${t.id}`}
            aria-selected={active}
            aria-controls={`panel-${t.id}`}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(t.id)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowRight") {
                e.preventDefault();
                move(i, 1);
              } else if (e.key === "ArrowUp" || e.key === "ArrowLeft") {
                e.preventDefault();
                move(i, -1);
              } else if (e.key === "Home") {
                e.preventDefault();
                onChange(TABS[0].id);
                refs.current[0]?.focus();
              } else if (e.key === "End") {
                e.preventDefault();
                onChange(TABS[TABS.length - 1].id);
                refs.current[TABS.length - 1]?.focus();
              }
            }}
            className={cn(
              "rounded-none border-l-2 px-3 py-2 text-left text-base font-semibold transition-colors",
              active
                ? "border-l-brass text-ink"
                : "border-l-transparent text-steel hover:text-ink"
            )}
          >
            {t.label}
            {typeof n === "number" && n > 0 ? (
              <span className="ml-2 text-sm font-medium text-steel">{n}</span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
