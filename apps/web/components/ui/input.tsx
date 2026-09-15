import { cn } from "@/lib/utils";

export function Input({ className, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      className={cn(
        "flex h-10 w-full min-w-0 border border-rule bg-blotter px-3 text-sm text-ink placeholder:text-steel focus-visible:border-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink",
        className
      )}
      {...props}
    />
  );
}
