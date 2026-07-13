/**
 * TrialBanner component tests (D-089 A3):
 *
 * (a) no expiry (daysRemaining null) => renders nothing
 * (b) daysRemaining > 14 => renders nothing
 * (c) 0 < daysRemaining <= 14 => warning strip with role="alert" and dismiss ×
 * (d) isTrialExpired => error strip (non-dismissable) with role="alert"
 * (e) dismiss × writes to sessionStorage and hides warning banner
 * (f) sessionStorage 'trial-banner-dismissed' suppresses warning banner on mount
 * (g) expired error strip ignores 'trial-banner-dismissed' (always shown)
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// Mock LicenseContext so TrialBanner tests are self-contained
vi.mock("@/lib/LicenseContext", () => ({
  useLicense: vi.fn(),
}));

import { useLicense } from "@/lib/LicenseContext";
import { TrialBanner } from "../TrialBanner";

const mockUseLicense = vi.mocked(useLicense);

describe("TrialBanner", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.resetAllMocks();
  });

  it("(a) renders nothing when daysRemaining is null (perpetual / no expiry)", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "free", valid: true },
      daysRemaining: null,
      isTrialExpired: false,
    });
    const { container } = render(<TrialBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("(b) renders nothing when daysRemaining > 14", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 30 * 86400000 },
      daysRemaining: 30,
      isTrialExpired: false,
    });
    const { container } = render(<TrialBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("(c) renders warning strip with role=alert when 0 < daysRemaining <= 14", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 7 * 86400000 },
      daysRemaining: 7,
      isTrialExpired: false,
    });
    render(<TrialBanner />);
    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert.textContent).toMatch(/License expires in 7 day/);
    // Warning must mention Settings › License link path
    expect(alert.textContent).toMatch(/Settings.*License/i);
  });

  it("(c) warning strip: daysRemaining === 1 uses singular 'day'", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 1 * 86400000 },
      daysRemaining: 1,
      isTrialExpired: false,
    });
    render(<TrialBanner />);
    expect(screen.getByRole("alert").textContent).toMatch(/1 day\b/);
  });

  it("(c) warning strip: daysRemaining === 14 shows (boundary)", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 14 * 86400000 },
      daysRemaining: 14,
      isTrialExpired: false,
    });
    render(<TrialBanner />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("(d) renders error strip (non-dismissable) when isTrialExpired", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "free", valid: false, expires_at: Date.now() - 1000 },
      daysRemaining: -1,
      isTrialExpired: true,
    });
    render(<TrialBanner />);
    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert.textContent).toMatch(/License expired/);
    // No dismiss button
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("(e) dismiss × writes sessionStorage key and hides warning banner", () => {
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 5 * 86400000 },
      daysRemaining: 5,
      isTrialExpired: false,
    });
    render(<TrialBanner />);
    // Banner is visible
    expect(screen.getByRole("alert")).toBeInTheDocument();
    // Click dismiss
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    // Banner is gone
    expect(screen.queryByRole("alert")).toBeNull();
    // sessionStorage key set
    expect(sessionStorage.getItem("trial-banner-dismissed")).toBe("1");
  });

  it("(f) warning banner is hidden on mount when sessionStorage 'trial-banner-dismissed' is set", () => {
    sessionStorage.setItem("trial-banner-dismissed", "1");
    mockUseLicense.mockReturnValue({
      license: { tier: "pro", valid: true, expires_at: Date.now() + 5 * 86400000 },
      daysRemaining: 5,
      isTrialExpired: false,
    });
    const { container } = render(<TrialBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("(g) expired error strip is shown even when sessionStorage 'trial-banner-dismissed' is set", () => {
    sessionStorage.setItem("trial-banner-dismissed", "1");
    mockUseLicense.mockReturnValue({
      license: { tier: "free", valid: false, expires_at: Date.now() - 86400000 },
      daysRemaining: -1,
      isTrialExpired: true,
    });
    render(<TrialBanner />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });
});
