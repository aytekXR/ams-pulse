/**
 * VD-01: Per-tier entitlement matrix tests.
 *
 * ⚠ HISTORY — READ BEFORE EDITING. This file used to define its OWN copies of
 * the gate predicates ("gate logic mirrors what ReportsPage.tsx and
 * AnomaliesPage.tsx implement") and assert against those copies. That makes the
 * test structurally incapable of catching production drift, and it did exactly
 * that: when anomaly detection moved from Enterprise-only to Business+, this
 * file kept passing while asserting `isAnomaliesGated("business") === true`,
 * i.e. a green test stating the opposite of the shipped behaviour.
 *
 * It now imports the real predicates from `src/lib/entitlements`. Do NOT
 * reintroduce a local copy of a rule here — if a rule is worth testing it is
 * worth importing.
 *
 * The matrix below mirrors `server/internal/license/license.go`. The server is
 * authoritative; these client-side gates only decide whether the user sees an
 * explanatory upgrade prompt or a raw 403.
 */
import { describe, it, expect } from "vitest";

import {
  TIER_ORDER,
  canUseAnomalies,
  canUseBeaconIngest,
  canUseDataAPI,
  canUseMultiTenant,
  canUsePrometheus,
  canUseProbes,
  canUseReports,
  canUseSSO,
  isChannelAllowed,
  tierAtLeast,
  type Tier,
} from "../../../lib/entitlements";

const ALL_TIERS: readonly Tier[] = TIER_ORDER;

/**
 * The whole entitlement matrix in one table, so a rule change is a one-line
 * diff and an unreviewed change is loud. Each row names the server check it
 * mirrors.
 */
const MATRIX: ReadonlyArray<{
  feature: string;
  serverCheck: string;
  fn: (t: string) => boolean;
  entitled: readonly Tier[];
}> = [
  {
    feature: "F6 usage/billing reports",
    serverCheck: "CheckReports",
    fn: canUseReports,
    entitled: ["business", "enterprise"],
  },
  {
    feature: "F6 multi-tenant billing",
    serverCheck: "CheckMultiTenant",
    fn: canUseMultiTenant,
    entitled: ["business", "enterprise"],
  },
  {
    // The rule that drifted. Business+, NOT Enterprise-only.
    feature: "F9 anomaly detection",
    serverCheck: "CheckAnomalies",
    fn: canUseAnomalies,
    entitled: ["business", "enterprise"],
  },
  {
    feature: "F8 Prometheus /metrics",
    serverCheck: "CheckPrometheus",
    fn: canUsePrometheus,
    entitled: ["business", "enterprise"],
  },
  {
    feature: "F10 synthetic probes",
    serverCheck: "CheckProbes",
    fn: canUseProbes,
    entitled: ["pro", "business", "enterprise"],
  },
  {
    feature: "F3 QoE beacon ingest",
    serverCheck: "CheckBeaconIngest",
    fn: canUseBeaconIngest,
    entitled: ["pro", "business", "enterprise"],
  },
  {
    // Covers /analytics/audience, /analytics/geo, /analytics/devices,
    // /qoe/summary and /qoe/ingest — i.e. F2, F3, F4 and F8.
    feature: "F2/F4/F8 Data API + historical analytics + ingest health",
    serverCheck: "CheckDataAPI",
    fn: canUseDataAPI,
    entitled: ["pro", "business", "enterprise"],
  },
  {
    feature: "SSO / OIDC login",
    serverCheck: "CheckSSO",
    fn: canUseSSO,
    entitled: ["enterprise"],
  },
];

describe("VD-01: tier entitlement matrix (mirrors server/internal/license/license.go)", () => {
  for (const row of MATRIX) {
    describe(`${row.feature} (server: ${row.serverCheck})`, () => {
      for (const tier of ALL_TIERS) {
        const want = row.entitled.includes(tier);
        it(`${tier} tier is ${want ? "ENTITLED" : "GATED"}`, () => {
          expect(row.fn(tier)).toBe(want);
        });
      }
    });
  }
});

describe("VD-01: unknown tiers are denied, never granted", () => {
  // D-133/S71: the server moved off `t === "free"` style gates precisely so an
  // unrecognised tier string is blocked rather than silently allowed through.
  const bogus = ["", "enterprize", "Free", "premium", "unlimited"];
  for (const row of MATRIX) {
    for (const t of bogus) {
      it(`${row.feature}: ${JSON.stringify(t)} is denied`, () => {
        expect(row.fn(t)).toBe(false);
      });
    }
  }
  it("null and undefined are denied", () => {
    expect(canUseAnomalies(null)).toBe(false);
    expect(canUseAnomalies(undefined)).toBe(false);
    expect(tierAtLeast(undefined, "free")).toBe(false);
  });
});

describe("VD-01: tierAtLeast ordering", () => {
  it("is reflexive at every tier", () => {
    for (const t of ALL_TIERS) expect(tierAtLeast(t, t)).toBe(true);
  });

  it("is monotonic — a higher tier satisfies every lower minimum", () => {
    for (let i = 0; i < ALL_TIERS.length; i++) {
      for (let j = 0; j <= i; j++) {
        expect(tierAtLeast(ALL_TIERS[i], ALL_TIERS[j])).toBe(true);
      }
      for (let j = i + 1; j < ALL_TIERS.length; j++) {
        expect(tierAtLeast(ALL_TIERS[i], ALL_TIERS[j])).toBe(false);
      }
    }
  });
});

describe("VD-01: notification channel entitlement matrix (PRD §7.11)", () => {
  const cases: ReadonlyArray<[string, Tier[], Tier[]]> = [
    ["email", ["free", "pro", "business", "enterprise"], []],
    ["slack", ["pro", "business", "enterprise"], ["free"]],
    ["telegram", ["pro", "business", "enterprise"], ["free"]],
    ["pagerduty", ["business", "enterprise"], ["free", "pro"]],
    ["webhook", ["business", "enterprise"], ["free", "pro"]],
  ];

  for (const [channel, allowed, denied] of cases) {
    for (const t of allowed) {
      it(`${channel} is allowed on ${t}`, () => {
        expect(isChannelAllowed(t, channel)).toBe(true);
      });
    }
    for (const t of denied) {
      it(`${channel} is DENIED on ${t}`, () => {
        expect(isChannelAllowed(t, channel)).toBe(false);
      });
    }
  }

  it("an unknown channel type is denied on every tier", () => {
    for (const t of ALL_TIERS) expect(isChannelAllowed(t, "carrier-pigeon")).toBe(false);
  });
});
