import { cn } from "@/lib/utils";

export function Stat({
  label,
  value,
  hint,
  tone,
  testId,
}: {
  label: string;
  value: string;
  hint?: string;
  tone?: "good" | "bad" | "warn";
  testId?: string;
}) {
  const color =
    tone === "good" ? "text-up" : tone === "bad" ? "text-down" : tone === "warn" ? "text-brass" : "text-ink";
  const hintColor = tone === "bad" ? "text-down" : "text-steel";
  return (
    <div className="min-w-0" data-testid={testId}>
      <p className="text-sm text-steel">{label}</p>
      <p className={cn("num mt-0.5 truncate text-2xl font-semibold tracking-tight", color)} title={value}>
        {value}
      </p>
      {hint ? (
        <p className={cn("mt-0.5 truncate text-sm", hintColor)} title={hint}>
          {hint}
        </p>
      ) : null}
    </div>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <p className="max-w-md text-sm text-steel">{children}</p>;
}
