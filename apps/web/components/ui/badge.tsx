import { cn } from "@/lib/utils";

export function Badge({
  className,
  tone = "neutral",
  ...props
}: React.ComponentProps<"span"> & { tone?: "neutral" | "good" | "bad" | "teal" | "warn" }) {
  const tones = {
    neutral: "bg-stone-100 text-stone-700",
    good: "bg-emerald-50 text-emerald-800",
    bad: "bg-rose-50 text-rose-800",
    teal: "bg-teal-50 text-teal-800",
    warn: "bg-amber-50 text-amber-900",
  };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium tracking-wide",
        tones[tone],
        className
      )}
      {...props}
    />
  );
}
