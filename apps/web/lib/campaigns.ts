import type { PublicCampaign } from "./types";

export function lastOpen(c: PublicCampaign) {
  const days = c.days ?? [];
  for (let i = days.length - 1; i >= 0; i--) {
    if (days[i].session === "open") return days[i];
  }
  return days[days.length - 1];
}
