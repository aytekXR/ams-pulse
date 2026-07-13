/**
 * LicenseContext unit tests (D-089):
 *
 * (a) null expires_at => daysRemaining null, isTrialExpired false
 * (b) future expires_at, valid => daysRemaining > 0, isTrialExpired false
 * (c) past expires_at => daysRemaining <= 0, isTrialExpired true
 * (d) future expires_at, valid = false (server stale) => isTrialExpired true
 * (e) fetch-error => license stays null; no console.error thrown
 * (f) LicenseProvider fetches on mount and populates context
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";

// Mock the API client before imports that depend on it
vi.mock("@/api/client", () => ({
  adminApi: {
    getLicense: vi.fn(),
  },
}));

import { adminApi } from "@/api/client";
import {
  LicenseProvider,
  useLicense,
  computeDaysRemaining,
  computeIsTrialExpired,
} from "./LicenseContext";
import type { LicenseInfo } from "@/lib/api/types";

const mockGetLicense = vi.mocked(adminApi.getLicense);

// ─── Pure helpers ─────────────────────────────────────────────────────────────

describe("computeDaysRemaining", () => {
  it("(a) null license => null", () => {
    expect(computeDaysRemaining(null)).toBeNull();
  });

  it("(a) null expires_at => null", () => {
    const lic: LicenseInfo = { tier: "free", valid: true, expires_at: null };
    expect(computeDaysRemaining(lic)).toBeNull();
  });

  it("(b) future expires_at => positive days", () => {
    const in7Days = Date.now() + 7 * 86400000;
    const lic: LicenseInfo = { tier: "pro", valid: true, expires_at: in7Days };
    const days = computeDaysRemaining(lic);
    expect(days).not.toBeNull();
    expect(days!).toBeGreaterThan(0);
    expect(days!).toBeLessThanOrEqual(8); // ceil(7) = 7, small clock drift tolerance
  });

  it("(c) past expires_at => non-positive days", () => {
    const yesterday = Date.now() - 86400000;
    const lic: LicenseInfo = { tier: "pro", valid: false, expires_at: yesterday };
    const days = computeDaysRemaining(lic);
    expect(days).not.toBeNull();
    expect(days!).toBeLessThanOrEqual(0);
  });
});

describe("computeIsTrialExpired", () => {
  it("(a) null license => false", () => {
    expect(computeIsTrialExpired(null)).toBe(false);
  });

  it("(a) null expires_at => false", () => {
    const lic: LicenseInfo = { tier: "free", valid: true, expires_at: null };
    expect(computeIsTrialExpired(lic)).toBe(false);
  });

  it("(b) future expires_at + valid => false", () => {
    const in7Days = Date.now() + 7 * 86400000;
    const lic: LicenseInfo = { tier: "pro", valid: true, expires_at: in7Days };
    expect(computeIsTrialExpired(lic)).toBe(false);
  });

  it("(c) past expires_at => true", () => {
    const yesterday = Date.now() - 86400000;
    const lic: LicenseInfo = { tier: "pro", valid: true, expires_at: yesterday };
    expect(computeIsTrialExpired(lic)).toBe(true);
  });

  it("(d) future expires_at + valid=false => true (stale-server guard)", () => {
    const in30Days = Date.now() + 30 * 86400000;
    const lic: LicenseInfo = { tier: "pro", valid: false, expires_at: in30Days };
    expect(computeIsTrialExpired(lic)).toBe(true);
  });
});

// ─── LicenseProvider + useLicense hook ───────────────────────────────────────

const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <LicenseProvider>{children}</LicenseProvider>
);

describe("LicenseProvider / useLicense", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("(a) starts with null license while fetching", () => {
    // Never-resolving promise keeps loading state indefinitely
    mockGetLicense.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useLicense(), { wrapper: Wrapper });
    expect(result.current.license).toBeNull();
    expect(result.current.daysRemaining).toBeNull();
    expect(result.current.isTrialExpired).toBe(false);
  });

  it("(f) fetches license on mount and updates context", async () => {
    const fakeLic: LicenseInfo = { tier: "pro", valid: true, expires_at: Date.now() + 10 * 86400000 };
    mockGetLicense.mockResolvedValue(fakeLic);

    const { result } = renderHook(() => useLicense(), { wrapper: Wrapper });

    await waitFor(() => {
      expect(result.current.license).toEqual(fakeLic);
    });

    expect(result.current.daysRemaining).not.toBeNull();
    expect(result.current.isTrialExpired).toBe(false);
  });

  it("(e) fetch error => license stays null; no console.error spam", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockGetLicense.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useLicense(), { wrapper: Wrapper });

    await waitFor(() => {
      // License stays null; no crash
      expect(result.current.license).toBeNull();
    });

    // Provider must NOT call console.error (zero-console-error gate)
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("(c) past expires_at license => isTrialExpired true in context", async () => {
    const expiredLic: LicenseInfo = {
      tier: "pro",
      valid: false,
      expires_at: Date.now() - 86400000,
    };
    mockGetLicense.mockResolvedValue(expiredLic);

    const { result } = renderHook(() => useLicense(), { wrapper: Wrapper });

    await waitFor(() => {
      expect(result.current.license).toEqual(expiredLic);
    });

    expect(result.current.isTrialExpired).toBe(true);
    expect(result.current.daysRemaining).not.toBeNull();
    expect(result.current.daysRemaining!).toBeLessThanOrEqual(0);
  });

  it("throws when useLicense called outside LicenseProvider", () => {
    // Suppress React's unhandled error output in this test
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => renderHook(() => useLicense())).toThrow(
      "useLicense must be used inside <LicenseProvider>",
    );
    consoleSpy.mockRestore();
  });
});
