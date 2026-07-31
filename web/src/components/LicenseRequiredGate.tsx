/**
 * LicenseRequiredGate — renders the designed upgrade prompt for a 403
 * LICENSE_REQUIRED response, instead of a red error banner.
 *
 * WHY THIS EXISTS
 * ---------------
 * ReportsPage, AnomaliesPage and ProbesPage each check the tier BEFORE calling
 * the API and render <TierGate>. QoePage, AnalyticsPage and IngestPage do not —
 * they call the API and funnel every failure into <ErrorBanner>. Those three
 * endpoints are CheckDataAPI-gated, so a Free user got a red banner reading
 *
 *     LICENSE_REQUIRED: Data API (F8) requires Pro tier or higher (current: "free")
 *
 * which reads as a malfunction rather than a tier boundary, on three of the
 * product's main screens.
 *
 * Deriving the gate from the RESPONSE rather than from a client-side tier check
 * is deliberate: it stays correct no matter which tier a feature is later
 * assigned to, and it cannot drift from the server the way a duplicated
 * client-side rule can. The server's own message names the required tier, so
 * the prompt cannot claim a tier the server does not enforce.
 */
import type { ReactNode } from "react";

import { ApiError } from "@/api/client";
import { TierGate } from "@/components/TierGate";

/**
 * True when the error is specifically a licence-tier refusal, not any 403.
 * An auth/permission 403 must still surface as an error — it is a real problem,
 * not an upsell.
 */
export function isLicenseError(err: unknown): err is ApiError {
  return (
    err instanceof ApiError &&
    err.status === 403 &&
    (err.body as { code?: string } | undefined)?.code === "LICENSE_REQUIRED"
  );
}

/**
 * Pull the required tier out of the server's message, which has the shape
 * '... requires Pro tier or higher (current: "free")'. Returns null when the
 * message does not carry one, so copy is never invented.
 */
export function requiredTierFromError(err: ApiError): string | null {
  const m = /requires?\s+(Free|Pro|Business|Enterprise)\s+tier/i.exec(err.message);
  if (!m) return null;
  return m[1].charAt(0).toUpperCase() + m[1].slice(1).toLowerCase();
}

interface Props {
  /** The caught error. Render this component only when isLicenseError(err). */
  error: ApiError;
  /** Human name of the screen, e.g. "Player QoE". Used in the heading. */
  feature: string;
  /** Current licence tier label, if the page knows it. */
  tier?: string;
  /** What the feature unlocks, appended to the upgrade sentence. */
  unlocks?: string;
  icon: ReactNode;
  descriptionMaxWidth?: number;
}

export function LicenseRequiredGate({
  error,
  feature,
  tier,
  unlocks,
  icon,
  descriptionMaxWidth,
}: Props) {
  const required = requiredTierFromError(error);

  // Fall back to the server's own sentence when the tier cannot be parsed —
  // better a slightly generic prompt than a confidently wrong tier name.
  const heading = required
    ? `${feature} requires ${required} tier or higher`
    : `${feature} requires a higher plan`;

  const upgradeText = required
    ? `Upgrade to ${required} to unlock ${unlocks ?? feature.toLowerCase()}.`
    : error.message;

  return (
    <TierGate
      icon={icon}
      heading={heading}
      tier={tier ?? "your current"}
      upgradeText={upgradeText}
      descriptionMaxWidth={descriptionMaxWidth}
    />
  );
}
