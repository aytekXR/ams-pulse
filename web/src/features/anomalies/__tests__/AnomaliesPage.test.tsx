/**
 * Anomalies page tests (F9):
 *  - Anomaly flag rendering (observed/expected/sigma)
 *  - Tier-gated view (Enterprise required vs not)
 *  - Empty state when no anomalies
 *  - Sigma severity styling
 *  - Sensitivity selector a11y
 *  - Wave 3 — color token and spacing source-read pins
 *
 * NOTE: Earlier versions of this file contained two tautological describe
 * blocks — "AnomaliesPage tier gate logic" and "Anomaly sigma severity" —
 * which tested functions defined *in the test file itself*, never the rendered
 * component. Those were deleted per Rule 9. The same scenarios are covered
 * by the render-level tests below (enterprise/non-enterprise gate, badge labels).
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import type { AnomalyFlag, LicenseInfo } from "@/lib/api/types";

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

// ─── Wave 3: colour token source-read pins ────────────────────────────────────
//
// AnomaliesPage has no Recharts charts, so all colour/token assertions here
// target CSS custom properties rendered in inline styles that jsdom CAN read.

describe("AnomaliesPage — Wave 3 color token pins (source-read)", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const src = readFileSync(resolve(here, "../AnomaliesPage.tsx"), "utf-8");

  it("no bare hex literals remain in source (RULE 10)", () => {
    expect(src).not.toMatch(/#[0-9A-Fa-f]{6}/);
  });

  it("no var() fallbacks remain in CSS properties (RULE 4)", () => {
    expect(src).not.toMatch(/var\(--color-[^)]+,\s*#[0-9A-Fa-f]{6}/);
  });

  it("--color-muted is not used in any text color property (RULE 5)", () => {
    // --color-muted fails WCAG AA at normal text sizes; all text must use
    // --color-secondary or a semantic signal token instead.
    expect(src).not.toContain("var(--color-muted)");
  });
});

// ─── Wave 3: rendered colour CSS vars (jsdom-visible) ─────────────────────────

describe("AnomaliesPage — Wave 3 rendered colour CSS vars", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sensitivity label text uses var(--color-secondary) not --color-muted", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({ items: [], meta: { total: 0 } });
    render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText(/sensitivity:/i)).toBeInTheDocument();
    });
    const label = screen.getByText(/sensitivity:/i);
    const style = label.getAttribute("style") ?? label.style.cssText;
    expect(style).toContain("var(--color-secondary)");
    expect(style).not.toContain("var(--color-muted)");
  });

  it("zero-delta cell uses var(--color-secondary) for the neutral colour", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    const zeroDeltaFlag: AnomalyFlag = {
      id: "flag-zero",
      metric: "cpu_usage",
      scope: { node_id: null, app: null, stream_id: null },
      observed: 50,
      expected: 50, // delta = 0
      sigma: 2.1,
      ts: Date.now(),
    };
    vi.mocked(anomaliesApi.list).mockResolvedValue({ items: [zeroDeltaFlag], meta: { total: 1 } });
    const { container } = render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText("cpu_usage")).toBeInTheDocument();
    });
    // A zero delta renders "+0.00", not "0.00": the sign prefix is applied when delta >= 0.
    const tds = Array.from(container.querySelectorAll("td")) as HTMLElement[];
    const deltaCell = tds.find((td) => td.textContent?.trim() === "+0.00");
    expect(deltaCell).toBeDefined();
    const style = deltaCell!.getAttribute("style") ?? deltaCell!.style.cssText;
    expect(style).toContain("var(--color-secondary)");
  });

  it("sigma selector select uses htmlFor / id pair (keyboard a11y)", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({ items: [], meta: { total: 0 } });
    render(<AnomaliesPage />);
    await waitFor(() => {
      // getByLabelText resolves the label→control association
      const select = screen.getByLabelText(/minimum sigma threshold/i);
      expect(select.tagName).toBe("SELECT");
    });
  });

  it("table header cells use var(--space-2) var(--space-3) padding (RULE 1 exact tokens)", async () => {
    vi.mocked(adminApi.getLicense).mockResolvedValue(enterpriseLicense);
    vi.mocked(anomaliesApi.list).mockResolvedValue({
      items: [sampleFlags[0]],
      meta: { total: 1 },
    });
    const { container } = render(<AnomaliesPage />);
    await waitFor(() => {
      expect(screen.getByText("viewers")).toBeInTheDocument();
    });
    const ths = Array.from(container.querySelectorAll("th")) as HTMLElement[];
    expect(ths.length).toBeGreaterThan(0);
    const firstTh = ths[0];
    const style = firstTh.getAttribute("style") ?? firstTh.style.cssText;
    // jsdom normalises CSS var() in inline styles to the literal string
    expect(style).toMatch(/padding.*var\(--space-2\)/);
  });
});
