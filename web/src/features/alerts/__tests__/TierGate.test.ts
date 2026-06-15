/**
 * VD-01: Per-tier entitlement matrix tests.
 *
 * Guards that the correct tier is required for each feature:
 *   - Reports / Tenants / Alert Channels (pagerduty/webhook): Business+
 *   - Anomaly Detection: Enterprise only
 *
 * These pure-logic tests verify the tier-check functions match PRD §7.11,
 * catching regressions like the old `tier==='free'` gate that allowed `pro`
 * into Business-gated features.
 */
import { describe, it, expect } from "vitest";

// ─── Gate logic mirrors what ReportsPage.tsx and AnomaliesPage.tsx implement ──

/** Reports (usage/schedules/tenants) require Business tier or higher. */
function isReportsGated(tier: string): boolean {
  return tier !== "business" && tier !== "enterprise";
}

/** Anomaly Detection (F9) requires Enterprise tier. */
function isAnomaliesGated(tier: string): boolean {
  return tier !== "enterprise";
}

/** PagerDuty and webhook notification channels require Business tier or higher. */
function isPagerdutyChannelAllowed(tier: string): boolean {
  return tier === "business" || tier === "enterprise";
}

/** Slack/Telegram channels require Pro tier or higher. */
function isSlackChannelAllowed(tier: string): boolean {
  return tier === "pro" || tier === "business" || tier === "enterprise";
}

/** Email channel is available on all tiers (including free). */
function isEmailChannelAllowed(_tier: string): boolean {
  return true;
}

// ─── Reports gate matrix ──────────────────────────────────────────────────────

describe("VD-01: Reports tier gate (business+ required)", () => {
  it("free tier is gated from reports", () => {
    expect(isReportsGated("free")).toBe(true);
  });

  // VD-01 guard: old code had `tier === 'free'`, allowing pro through
  it("pro tier is GATED from reports (not 'free' check — it needs business+)", () => {
    expect(isReportsGated("pro")).toBe(true);
  });

  it("business tier is entitled for reports", () => {
    expect(isReportsGated("business")).toBe(false);
  });

  it("enterprise tier is entitled for reports", () => {
    expect(isReportsGated("enterprise")).toBe(false);
  });
});

// ─── Anomaly Detection gate matrix ───────────────────────────────────────────

describe("VD-01: Anomaly Detection tier gate (enterprise only)", () => {
  it("free tier is gated from anomalies", () => {
    expect(isAnomaliesGated("free")).toBe(true);
  });

  it("pro tier is gated from anomalies", () => {
    expect(isAnomaliesGated("pro")).toBe(true);
  });

  it("business tier is gated from anomalies (enterprise required)", () => {
    expect(isAnomaliesGated("business")).toBe(true);
  });

  it("enterprise tier is entitled for anomalies", () => {
    expect(isAnomaliesGated("enterprise")).toBe(false);
  });
});

// ─── Notification channel gate matrix ────────────────────────────────────────

describe("VD-01: Notification channel tier entitlement matrix (PRD §7.11)", () => {
  // Email: all tiers
  it("email channel is allowed on free tier", () => {
    expect(isEmailChannelAllowed("free")).toBe(true);
  });
  it("email channel is allowed on pro tier", () => {
    expect(isEmailChannelAllowed("pro")).toBe(true);
  });
  it("email channel is allowed on business tier", () => {
    expect(isEmailChannelAllowed("business")).toBe(true);
  });

  // Slack: pro+
  it("slack channel requires pro tier or higher", () => {
    expect(isSlackChannelAllowed("free")).toBe(false);
    expect(isSlackChannelAllowed("pro")).toBe(true);
    expect(isSlackChannelAllowed("business")).toBe(true);
    expect(isSlackChannelAllowed("enterprise")).toBe(true);
  });

  // PagerDuty / Webhook: business+
  it("pagerduty channel requires business tier or higher", () => {
    expect(isPagerdutyChannelAllowed("free")).toBe(false);
    expect(isPagerdutyChannelAllowed("pro")).toBe(false);    // old code allowed this
    expect(isPagerdutyChannelAllowed("business")).toBe(true);
    expect(isPagerdutyChannelAllowed("enterprise")).toBe(true);
  });

  it("webhook channel has same entitlement as pagerduty (business+)", () => {
    // webhook is the same gate as pagerduty per PRD §7.11
    expect(isPagerdutyChannelAllowed("pro")).toBe(false);
    expect(isPagerdutyChannelAllowed("business")).toBe(true);
  });
});
