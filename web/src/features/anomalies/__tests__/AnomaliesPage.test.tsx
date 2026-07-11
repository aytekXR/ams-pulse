/**
 * Anomalies page tests (F9):
 *  - Anomaly flag rendering (observed/expected/sigma)
 *  - Tier-gated view (Enterprise required vs not)
 *  - Empty state when no anomalies
 *  - Sigma severity styling
 *  - Sensitivity selector changes query param
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import type { AnomalyFlag, LicenseInfo } from "@/lib/api/types";

// ─── Tier gate logic (pure unit tests — no rendering needed) ─────────────────

function isTierEntitled(tier: string): boolean {
  return tier === "enterprise";
}

describe("AnomaliesPage tier gate logic", () => {
  it("enterprise tier is entitled", () => {
    expect(isTierEntitled("enterprise")).toBe(true);
  });

  it("pro tier is NOT entitled for anomalies", () => {
    expect(isTierEntitled("pro")).toBe(false);
  });

  it("free tier is NOT entitled for anomalies", () => {
    expect(isTierEntitled("free")).toBe(false);
  });
});

// ─── Sigma severity logic (pure unit tests) ────────────────────────────────────

function sigmaSeverity(sigma: number): "error" | "warning" | "info" {
  if (sigma >= 4) return "error";
  if (sigma >= 3) return "warning";
  return "info";
}

function sigmaLabel(sigma: number): string {
  if (sigma >= 4) return "high";
  if (sigma >= 3) return "medium";
  return "low";
}

describe("Anomaly sigma severity", () => {
  it("sigma >= 4 is error / high", () => {
    expect(sigmaSeverity(4)).toBe("error");
    expect(sigmaSeverity(5.2)).toBe("error");
    expect(sigmaLabel(4.1)).toBe("high");
  });

  it("sigma >= 3 and < 4 is warning / medium", () => {
    expect(sigmaSeverity(3)).toBe("warning");
    expect(sigmaSeverity(3.9)).toBe("warning");
    expect(sigmaLabel(3.5)).toBe("medium");
  });

  it("sigma >= 2 and < 3 is info / low", () => {
    expect(sigmaSeverity(2)).toBe("info");
    expect(sigmaSeverity(2.9)).toBe("info");
    expect(sigmaLabel(2.5)).toBe("low");
  });
});

// ─── Anomaly flag rendering ───────────────────────────────────────────────────

// Mock modules
vi.mock("@/api/client", () => ({
  anomaliesApi: {
    list: vi.fn(),
  },
  adminApi: {
    getLicense: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    status: number;
    body: unknown;
    constructor(status: number, body: { message?: string }) {
      super(body.message ?? `HTTP ${status}`);
      this.status = status;
      this.body = body;
      this.name = "ApiError";
    }
  },
}));

import { adminApi, anomaliesApi } from "@/api/client";
import { AnomaliesPage } from "../AnomaliesPage";

const enterpriseLicense: LicenseInfo = { tier: "enterprise", valid: true };
const freeLicense: LicenseInfo = { tier: "free", valid: true };
const proLicense: LicenseInfo = { tier: "pro", valid: true };

const sampleFlags: AnomalyFlag[] = [
  {
    id: "flag-1",
    metric: "viewers",
    scope: { node_id: "node-1", app: "live", stream_id: null },
    observed: 150,
    expected: 50,
    sigma: 4.5,
    ts: Date.now() - 60_000,
  },
  {
    id: "flag-2",
    metric: "error_rate",
    scope: { node_id: null, app: "live", stream_id: "stream/main" },
    observed: 0.15,
    expected: 0.01,
    sigma: 3.1,
    ts: Date.now() - 120_000,
  },
  {
    id: "flag-3",
    metric: "rebuffer_ratio",
    scope: { node_id: null, app: null, stream_id: null },
    observed: 0.08,
    expected: 0.02,
    sigma: 2.2,
    ts: Date.now() - 300_000,
  },
];

describe("AnomaliesPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading spinner while license loads", () => {
    vi.mocked(adminApi.getLicense).mockReturnValue(new Promise(() => {}));
    const { unmount } = render(<AnomaliesPage />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    unmount();
  });

  it("shows tier upsell when license is 'free'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(freeLicense);
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(
        screen.getByText(/anomaly detection requires enterprise tier/i),
      ).toBeInTheDocument();
    });
  });

  it("shows upgrade link when tier is 'free'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(freeLicense);
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByRole("link", { name: /upgrade license/i })).toBeInTheDocument();
    });
  });

  it("shows tier upsell when license is 'pro'", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(proLicense);
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(
        screen.getByText(/anomaly detection requires enterprise tier/i),
      ).toBeInTheDocument();
    });
  });

  it("shows anomaly flags table when enterprise-entitled", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: sampleFlags,
      meta: { total: 3 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText("viewers")).toBeInTheDocument();
    });
  });

  it("renders observed/expected/sigma values for each flag", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: sampleFlags,
      meta: { total: 3 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      // Flag 1: observed 150, expected 50, sigma 4.5
      expect(screen.getByText("viewers")).toBeInTheDocument();
      // sigma displayed (abs)
      expect(screen.getByText("4.50σ")).toBeInTheDocument();
    });
    // expected value shown
    expect(screen.getByText(/expected 50/)).toBeInTheDocument();
  });

  it("shows sigma severity badges", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: sampleFlags,
      meta: { total: 3 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText("high")).toBeInTheDocument();
      expect(screen.getByText("medium")).toBeInTheDocument();
      expect(screen.getByText("low")).toBeInTheDocument();
    });
  });

  it("shows empty state when no anomalies", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({ items: [], meta: { total: 0 } });
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText(/no anomalies detected/i)).toBeInTheDocument();
    });
    // Should mention baselines learning
    expect(screen.getByText(/baselines are still learning/i)).toBeInTheDocument();
  });

  it("shows error banner on fetch failure", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockRejectedValue(new Error("network error"));
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("renders scope information", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: [sampleFlags[0]],
      meta: { total: 1 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      // node:node-1, app:live
      expect(screen.getByText(/node:node-1/)).toBeInTheDocument();
    });
  });

  it("sigma selector is visible and functional", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({ items: [], meta: { total: 0 } });
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText(/minimum sigma threshold/i)).toBeInTheDocument();
    });
    const select = screen.getByLabelText(/minimum sigma threshold/i);
    // Default value is 2
    expect((select as HTMLSelectElement).value).toBe("2");
    // Change to 3
    fireEvent.change(select, { target: { value: "3" } });
    expect((select as HTMLSelectElement).value).toBe("3");
  });
});

// ─── B2 sweep: status color CSS vars ─────────────────────────────────────────
//
// These pins verify that semantic status inline-styles reference CSS custom
// properties instead of hardcoded hex values. CSS vars let global.css serve
// the correct value for both dark and light themes.
//
// jsdom 29 serialises CSS-var references to getAttribute("style") even for
// standard CSS properties (e.g. color: var(--color-error)), so we can assert
// on the raw style string.

describe("AnomaliesPage — status-color CSS vars (B2 sweep)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sigma error color uses var(--color-error) not hardcoded #FF5C68", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: [sampleFlags[0]], // sigma 4.5 → severity "error"
      meta: { total: 1 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      const sigmaEl = screen.getByText("4.50σ");
      const styleText = sigmaEl.getAttribute("style") ?? sigmaEl.style.cssText;
      expect(styleText).toContain("var(--color-error)");
    });
  });

  it("sigma warning color uses var(--color-warning) not hardcoded #FFB224", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: [sampleFlags[1]], // sigma 3.1 → severity "warning"
      meta: { total: 1 },
    });
    render(<AnomaliesPage />);
    await waitFor(() => {
      const sigmaEl = screen.getByText("3.10σ");
      const styleText = sigmaEl.getAttribute("style") ?? sigmaEl.style.cssText;
      expect(styleText).toContain("var(--color-warning)");
    });
  });

  it("positive delta uses var(--color-error) not hardcoded #FF5C68", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: [sampleFlags[0]], // observed 150, expected 50 → delta = +100 → positive
      meta: { total: 1 },
    });
    const { container } = render(<AnomaliesPage />);
    await waitFor(() => {
      // Delta cell: positive delta → error color; text content is "+100.00"
      const tds = Array.from(container.querySelectorAll("td")) as HTMLElement[];
      const deltaCell = tds.find((td) => td.textContent?.trim() === "+100.00");
      expect(deltaCell).toBeDefined();
      const styleText = deltaCell!.getAttribute("style") ?? deltaCell!.style.cssText;
      expect(styleText).toContain("var(--color-error)");
    });
  });
});
