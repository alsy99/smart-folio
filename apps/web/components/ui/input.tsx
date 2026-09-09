import { cn } from "@/lib/utils";

export function Input({ className, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      className={cn(
        "flex h-10 w-full rounded-md border border-stone-300 bg-white px-3 text-sm outline-none focus:ring-2 focus:ring-teal-500/40",
        className
      )}
      {...props}
    />
  );
}
