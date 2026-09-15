import { cn } from "@/lib/utils";

export function SessionIris({ open }: { open: boolean }) {
  const stroke = open ? "#B0892A" : "#6D7380";
  return (
    <svg
      viewBox="0 0 88 88"
      width="80"
      height="80"
      className={cn("h-20 w-20 shrink-0", open && "iris-open")}
      aria-hidden="true"
    >
      <circle cx="44" cy="44" r="38" fill="none" stroke={stroke} strokeWidth="2.5" />
      <circle cx="44" cy="44" r="22" fill="none" stroke={stroke} strokeWidth="1.5" opacity="0.55" />
      <path
        d="M68 44h12M56 64.78l6 10.39M32 64.78l-6 10.39M20 44H8M32 23.22l-6-10.39M56 23.22l6-10.39"
        fill="none"
        stroke={stroke}
        strokeWidth="2"
      />
      <circle cx="44" cy="44" r="8" fill={stroke} />
    </svg>
  );
}
