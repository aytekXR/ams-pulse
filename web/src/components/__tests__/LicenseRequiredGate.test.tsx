/**
 * LicenseRequiredGate — a 403 LICENSE_REQUIRED must read as a tier boundary,
 * not as a malfunction.
 *
 * Before this component, QoePage / AnalyticsPage / IngestPage funnelled every
 * failure into <ErrorBanner>, so a Free user met a red alert saying
 *   "LICENSE_REQUIRED: Data API (F8) requires Pro tier or higher (current: \"free\")"
 * on three of the product's main screens.
 *
 * The required tier is parsed from the SERVER's message rather than from a
 * client-side tier table on purpose: it stays correct when a feature is
 * reassigned to a different tier, and it cannot drift from the server. These
 * tests pin both that behaviour and its fallback.
 */
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { ApiError } from "@/api/client";
import {
  LicenseRequiredGate,
  isLicenseError,
  requiredTierFromError,
} from "@/components/LicenseRequiredGate";

const icon = <svg data-testid="icon" aria-hidden="true" />;

function licenseErr(message: string): ApiError {
  return new ApiError(403, { code: "LICENSE_REQUIRED", message });
}

describe("isLicenseError", () => {
  it("accepts a 403 with code LICENSE_REQUIRED", () => {
    expect(isLicenseError(licenseErr('requires Pro tier or higher (current: "free")'))).toBe(true);
  });

  it("REJECTS a 403 that is an auth/permission failure, not a tier boundary", () => {
    // A permission 403 is a real problem and must keep surfacing as an error.
    expect(isLicenseError(new ApiError(403, { code: "FORBIDDEN", message: "not allowed" }))).toBe(
      false,
    );
  });

  it("rejects other statuses even with the licence code", () => {
    expect(isLicenseError(new ApiError(500, { code: "LICENSE_REQUIRED", message: "x" }))).toBe(
      false,
    );
  });

  it("rejects non-ApiError values", () => {
    expect(isLicenseError(new Error("network error"))).toBe(false);
    expect(isLicenseError(null)).toBe(false);
    expect(isLicenseError(undefined)).toBe(false);
    expect(isLicenseError("LICENSE_REQUIRED")).toBe(false);
  });
});

describe("requiredTierFromError", () => {
  it.each([
    ['Data API (F8) requires Pro tier or higher (current: "free")', "Pro"],
    ['usage/billing reports (F6) require Business tier or higher (current: "pro")', "Business"],
    ['SSO / OIDC login requires Enterprise tier (current: "business")', "Enterprise"],
  ])("parses %s", (message, want) => {
    expect(requiredTierFromError(licenseErr(message))).toBe(want);
  });

  it("returns null rather than guessing when the message names no tier", () => {
    expect(requiredTierFromError(licenseErr("licence check failed"))).toBeNull();
  });
});

describe("LicenseRequiredGate rendering", () => {
  it("names the tier the SERVER requires, not a hardcoded one", () => {
    render(
      <LicenseRequiredGate
        error={licenseErr('Data API (F8) requires Pro tier or higher (current: "free")')}
        feature="Historical analytics"
        tier="free"
        unlocks="geo and device breakdowns"
        icon={icon}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /historical analytics requires pro tier/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/upgrade to pro to unlock geo and device breakdowns/i)).toBeInTheDocument();
  });

  it("tracks a reassigned tier without a code change", () => {
    // The same screen, if the server later gates it at Business, must say Business.
    render(
      <LicenseRequiredGate
        error={licenseErr('Ingest health (F4) requires Business tier or higher (current: "pro")')}
        feature="Ingest health"
        tier="pro"
        icon={icon}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /ingest health requires business tier/i }),
    ).toBeInTheDocument();
  });

  it("falls back to the server's sentence instead of inventing a tier", () => {
    render(
      <LicenseRequiredGate
        error={licenseErr("licence check failed")}
        feature="Player QoE"
        tier="free"
        icon={icon}
      />,
    );
    expect(
      screen.getByRole("heading", { level: 2, name: /player qoe requires a higher plan/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/licence check failed/i)).toBeInTheDocument();
  });

  it("renders no error-alert role — this is an upsell, not a failure", () => {
    render(
      <LicenseRequiredGate
        error={licenseErr('requires Pro tier or higher (current: "free")')}
        feature="Player QoE"
        tier="free"
        icon={icon}
      />,
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
