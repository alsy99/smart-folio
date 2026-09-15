import { cn } from "@/lib/utils";

export function Badge({
  className,
  tone = "neutral",
  ...props
}: React.ComponentProps<"span"> & { tone?: "neutral" | "good" | "bad" | "teal" | "warn" }) {
  const tones = {
    neutral: "text-steel",
    good: "text-up",
    bad: "text-down",
    teal: "text-ink",
    warn: "text-brass",
  };
  return (
    <span className={cn("inline-flex items-center text-[13px] font-medium", tones[tone], className)} {...props} />
  );
}
