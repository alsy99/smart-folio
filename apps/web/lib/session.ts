import type { Campaign, TickResult } from "./types";

/** Paper fills only while the NSE cash session is open and an IPS is bound. */
export function fillAllowed(
  campaign: Pick<Campaign, "marketOpen" | "sessionStatus" | "ipsId"> | null | undefined
): boolean {
  if (!campaign) return false;
  if (!campaign.ipsId) return false;
  if (!campaign.marketOpen) return false;
  const st = campaign.sessionStatus || "";
  if (st === "closed" || st === "weekend") return false;
  return true;
}

/** A closed session must not add fills, even if a tick payload looks busy. */
export function fillsFromTick(
  campaign: Pick<Campaign, "marketOpen" | "sessionStatus"> | null | undefined,
  tick: Pick<TickResult, "skipped" | "fills" | "reason"> | null | undefined
): number {
  if (!fillAllowed(campaign)) return 0;
  if (!tick || tick.skipped) return 0;
  return tick.fills || 0;
}
