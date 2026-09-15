import { Card, CardContent } from "@/components/ui/card";

export function Hero({
  label,
  value,
  sub,
  tone,
}: {
  label: string;
  value: string;
  sub: string;
  tone?: "good" | "bad" | "warn";
}) {
  const color =
    tone === "good" ? "text-emerald-800" : tone === "bad" ? "text-rose-800" : "text-stone-900";
  return (
    <Card>
      <CardContent className="pt-5">
        <p className="text-[11px] uppercase tracking-[0.18em] text-stone-500">{label}</p>
        <p className={`num mt-1 font-[family-name:var(--font-display)] text-2xl ${color}`}>{value}</p>
        <p className="mt-1 text-xs text-stone-500">{sub}</p>
      </CardContent>
    </Card>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-stone-500">{children}</p>;
}
