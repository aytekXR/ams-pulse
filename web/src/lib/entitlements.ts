/**
 * Tier entitlement rules for the web UI — the single client-side source of truth.
 *
 * WHY THIS FILE EXISTS
 * --------------------
 * These rules used to live inline in each page, and the test that "guarded" them
 * (features/alerts/__tests__/TierGate.test.ts) defined its OWN copy of the same
 * predicates and asserted against that. So the test could never observe the
 * production rule at all. When anomaly detection moved from Enterprise-only to
 * Business+ on the server, the pages were updated and the test kept passing
 * while asserting the opposite — a green suite stating a false thing.
 *
 * Every predicate here mirrors a named check in
 * `server/internal/license/license.go`. The server is authoritative: the client
 * gate exists only to show an explanatory upgrade prompt instead of a raw 403,
 * and MUST NOT be treated as enforcement. If the two ever disagree, the server
 * wins and this file is the bug.
 *
 * Keep the mapping comment on each function — it is what makes a drift review
 * mechanical rather than archaeological.
 */

import type { components } from "./api/schema";

export type Tier = components["schemas"]["LicenseInfo"]["tier"];

/** Ascending capability order. Index comparisons below rely on this ordering. */
export const TIER_ORDER: readonly Tier[] = ["free", "pro", "business", "enterprise"] as const;

/**
 * True when `tier` is at least `min`.
 *
 * An unrecognised tier string returns false (deny), matching the server's
 * positive-membership checks. The server deliberately moved off `t === "free"`
 * style gates for exactly this reason (D-133/S71): a typo or a future tier name
 * must not silently grant access.
 */
export function tierAtLeast(tier: string | undefined | null, min: Tier): boolean {
  const i = TIER_ORDER.indexOf(tier as Tier);
  if (i < 0) return false;
  return i >= TIER_ORDER.indexOf(min);
}

/** F6 usage/billing reports — server: CheckReports (Business, Enterprise). */
export function canUseReports(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "business");
}

/** F6 multi-tenant billing — server: CheckMultiTenant (Business, Enterprise). */
export function canUseMultiTenant(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "business");
}

/**
 * F9 anomaly detection — server: CheckAnomalies (Business, Enterprise).
 *
 * Business+, NOT Enterprise-only. This is the rule that drifted: the API served
 * Business while the UI showed an "upgrade to Enterprise" wall, so an entitled
 * Business tenant met an upgrade prompt over data they were already paying for.
 */
export function canUseAnomalies(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "business");
}

/** F8 Prometheus /metrics — server: CheckPrometheus (Business, Enterprise). */
export function canUsePrometheus(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "business");
}

/** F10 synthetic probes — server: CheckProbes (Pro, Business, Enterprise). */
export function canUseProbes(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "pro");
}

/** F3 QoE beacon ingest — server: CheckBeaconIngest (Pro, Business, Enterprise). */
export function canUseBeaconIngest(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "pro");
}

/**
 * F2 historical analytics, F3 QoE summary, F4 ingest health, F8 Data API —
 * server: CheckDataAPI, which tests the DataAPI entitlement flag. That flag is
 * false on Free and true on Pro/Business/Enterprise, so the effective rule is
 * Pro+.
 *
 * This one gate covers /analytics/audience, /analytics/geo, /analytics/devices,
 * /qoe/summary and /qoe/ingest.
 */
export function canUseDataAPI(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "pro");
}

/** SSO / OIDC login — server: CheckSSO (Enterprise only). */
export function canUseSSO(tier: string | undefined | null): boolean {
  return tierAtLeast(tier, "enterprise");
}

/**
 * Notification channels allowed per tier — server: CheckChannelAllowed, which
 * tests membership of the tier's Channels entitlement list.
 */
export const TIER_CHANNELS: Readonly<Record<Tier, readonly string[]>> = {
  free: ["email"],
  pro: ["email", "slack", "telegram"],
  business: ["email", "slack", "telegram", "pagerduty", "webhook"],
  enterprise: ["email", "slack", "telegram", "pagerduty", "webhook"],
} as const;

export function isChannelAllowed(tier: string | undefined | null, channelType: string): boolean {
  const i = TIER_ORDER.indexOf(tier as Tier);
  if (i < 0) return false;
  return TIER_CHANNELS[TIER_ORDER[i]].includes(channelType);
}

/**
 * The minimum tier that unlocks a feature, for building upgrade copy. Returning
 * the tier name (rather than hardcoding "Enterprise" in each page) is what stops
 * an upgrade prompt naming the wrong tier after a rule changes.
 */
export function minTierLabel(min: Tier): string {
  return min.charAt(0).toUpperCase() + min.slice(1);
}
