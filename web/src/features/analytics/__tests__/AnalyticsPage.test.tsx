/**
 * AnalyticsPage rendering tests.
 *
 * Covers:
 * (a) Smoke render — page mounts without crashing and shows the page title.
 * (b) Loading state — spinner present while fetch is in flight.
 * (c) Empty state — no data → "No data for this range" message shown.
 * (d) Populated state — audience totals and tab labels are visible.
 * (e) Tab switching — Geo and Device tabs render their empty states.
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { AnalyticsPage } from "../AnalyticsPage";
import type { AudienceResponse, GeoResponse, DeviceResponse } from "@/lib/api/types";

const mockGetAudience = vi.fn();
const mockGetGeo = vi.fn();
const mockGetDevices = vi.fn();
const mockExportCsv = vi.fn();

vi.mock("@/api/client", () => ({
  analyticsApi: {
    getAudience: (...args: unknown[]) => mockGetAudience(...args),
    getGeo: (...args: unknown[]) => mockGetGeo(...args),
    getDevices: (...args: unknown[]) => mockGetDevices(...args),
    exportCsv: (...args: unknown[]) => mockExportCsv(...args),
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

const emptyAudience: AudienceResponse = {
  totals: { views: 0, uniques: 0, watch_time_s: 0, peak_concurrency: 0 },
  timeseries: [],
};

const populatedAudience: AudienceResponse = {
  totals: { views: 1234, uniques: 567, watch_time_s: 7200, peak_concurrency: 89 },
  timeseries: [
    { ts: Date.now() - 86400_000, views: 600, uniques: 300, watch_time_s: 3600, peak_concurrency: 45 },
    { ts: Date.now(), views: 634, uniques: 267, watch_time_s: 3600, peak_concurrency: 44 },
  ],
};

const emptyGeo: GeoResponse = { rows: [] };
const emptyDevice: DeviceResponse = { rows: [] };

describe("AnalyticsPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: all calls resolve with empty data
    mockGetAudience.mockResolvedValue(emptyAudience);
    mockGetGeo.mockResolvedValue(emptyGeo);
    mockGetDevices.mockResolvedValue(emptyDevice);
  });

  it("(a) smoke — mounts without crash and shows Analytics heading", () => {
    render(<AnalyticsPage />);
    expect(screen.getByRole("heading", { name: /analytics/i })).toBeInTheDocument();
  });

  it("(a) Export CSV button is rendered", () => {
    render(<AnalyticsPage />);
    expect(screen.getByRole("button", { name: /export csv/i })).toBeInTheDocument();
  });

  it("(b) audience/geo/device tab buttons are rendered", () => {
    render(<AnalyticsPage />);
    expect(screen.getByRole("tab", { name: /^audience$/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /^geo$/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /^device$/i })).toBeInTheDocument();
  });

  it("(c) empty state — shows no-data message when timeseries is empty", async () => {
    render(<AnalyticsPage />);
    await waitFor(() => {
      expect(screen.getByText(/no data for this range/i)).toBeInTheDocument();
    });
  });

  it("(d) populated — shows total views from totals block", async () => {
    mockGetAudience.mockResolvedValue(populatedAudience);
    render(<AnalyticsPage />);
    await waitFor(() => {
      expect(screen.getByText("1,234")).toBeInTheDocument();
    });
  });

  it("(d) populated — shows Unique Viewers label", async () => {
    mockGetAudience.mockResolvedValue(populatedAudience);
    render(<AnalyticsPage />);
    await waitFor(() => {
      expect(screen.getByText(/unique viewers/i)).toBeInTheDocument();
    });
  });

  it("(e) geo tab — shows no geo data empty state", async () => {
    render(<AnalyticsPage />);
    await waitFor(() => { expect(screen.queryByText(/loading/i)).not.toBeInTheDocument(); });
    fireEvent.click(screen.getByRole("tab", { name: /^geo$/i }));
    await waitFor(() => {
      expect(screen.getByText(/no geo data/i)).toBeInTheDocument();
    });
  });

  it("(e) device tab — shows no device data empty state", async () => {
    render(<AnalyticsPage />);
    await waitFor(() => { expect(screen.queryByText(/loading/i)).not.toBeInTheDocument(); });
    fireEvent.click(screen.getByRole("tab", { name: /^device$/i }));
    await waitFor(() => {
      expect(screen.getByText(/no device data/i)).toBeInTheDocument();
    });
  });

  it("shows error banner when fetch fails", async () => {
    mockGetAudience.mockRejectedValue(new Error("network error"));
    render(<AnalyticsPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });
});

// ─── Wave 2 (§2.19) ──────────────────────────────────────────────────────────

describe("Wave 2 — StatCard adoption (rendered)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAudience.mockResolvedValue(populatedAudience);
    mockGetGeo.mockResolvedValue(emptyGeo);
    mockGetDevices.mockResolvedValue(emptyDevice);
  });

  it("W2-1: totals render as shared StatCards WITH accessible names (the inline cards had none)", async () => {
    render(<AnalyticsPage />);
    await waitFor(() => {
      // role="group" + aria-label comes from <StatCard>; the replaced inline
      // <div> markup exposed nothing to assistive tech.
      expect(screen.getByRole("group", { name: "Total Views: 1,234" })).toBeInTheDocument();
    });
    expect(screen.getByRole("group", { name: "Unique Viewers: 567" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Peak Concurrency: 89" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Watch Time: 2h" })).toBeInTheDocument();
  });

  it("W2-2: the compact StatCard keeps the analytics geometry (14px 16px, 24px value)", async () => {
    render(<AnalyticsPage />);
    const card = await screen.findByRole("group", { name: "Total Views: 1,234" });
    // Pixel-neutrality of the swap: the default StatCard would be var(--card-padding)
    // (24px) with a var(--metric-size) (40px) value — visibly different.
    expect(card.style.padding).toBe("14px 16px");
    const value = card.querySelector("[data-metric]") as HTMLElement;
    expect(value).not.toBeNull();
    expect(value.style.fontSize).toBe("24px");
  });
});

describe("Wave 2 — tab panels (rendered)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAudience.mockResolvedValue(populatedAudience);
    mockGetGeo.mockResolvedValue(emptyGeo);
    mockGetDevices.mockResolvedValue(emptyDevice);
  });

  it("W2-3: the audience panel is a tabpanel labelled by its tab", async () => {
    render(<AnalyticsPage />);
    const panel = await screen.findByRole("tabpanel");
    expect(panel).toHaveAttribute("id", "panel-audience");
    expect(panel).toHaveAttribute("aria-labelledby", "tab-audience");
    // The referenced tab must actually exist — an aria-labelledby pointing at
    // nothing is a broken reference, not a label. Tabs.tsx emits id="tab-{id}".
    expect(document.getElementById("tab-audience")).not.toBeNull();
  });

  it("W2-3: switching tab moves the tabpanel and its label with it", async () => {
    render(<AnalyticsPage />);
    await screen.findByRole("tabpanel");
    fireEvent.click(screen.getByRole("tab", { name: "Geo" }));
    await waitFor(() => {
      const panel = screen.getByRole("tabpanel");
      expect(panel).toHaveAttribute("id", "panel-geo");
      expect(panel).toHaveAttribute("aria-labelledby", "tab-geo");
    });
    expect(document.getElementById("tab-geo")).not.toBeNull();
  });
});

describe("Wave 2 — brandkit compliance (source-level)", () => {
  // Reads the real component file: any reintroduced hex or muted token fails.
  // Not a tautology — the assertion is about the component's source, not about
  // an expression computed here.
  const here = dirname(fileURLToPath(import.meta.url));
  const analytics = readFileSync(resolve(here, "../AnalyticsPage.tsx"), "utf-8");
  const picker = readFileSync(resolve(here, "../DateRangePicker.tsx"), "utf-8");

  it("W2-4: chart strokes use CHART_COLORS indices, and no bare hex remains", () => {
    expect(analytics).toContain("stroke={CHART_COLORS[1]}"); // views  — #58A6FF
    expect(analytics).toContain("stroke={CHART_COLORS[0]}"); // uniques — #2CE5A7
    expect(analytics).toContain("stroke={CHART_COLORS[4]}"); // peak    — #FFB224
    expect(analytics).not.toMatch(/#[0-9A-Fa-f]{6}/);
    expect(picker).not.toMatch(/#[0-9A-Fa-f]{6}/);
  });

  it("W2-5: LineChart carries accessibilityLayer", () => {
    expect(analytics).toContain("accessibilityLayer");
  });

  it("W2-6: --color-muted is gone (it fails AA for normal text in both themes)", () => {
    expect(analytics).not.toContain("--color-muted");
    expect(picker).not.toContain("--color-muted");
  });
});

describe("Wave 2 — DateRangePicker a11y (rendered)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAudience.mockResolvedValue(populatedAudience);
    mockGetGeo.mockResolvedValue(emptyGeo);
    mockGetDevices.mockResolvedValue(emptyDevice);
  });

  it("W2-7: the custom-range inputs have accessible names", async () => {
    render(<AnalyticsPage />);
    fireEvent.click(await screen.findByRole("button", { name: "Custom" }));
    expect(screen.getByLabelText("Custom range start")).toBeInTheDocument();
    expect(screen.getByLabelText("Custom range end")).toBeInTheDocument();
  });

  it("W2-7: preset buttons report their selected state", async () => {
    render(<AnalyticsPage />);
    const preset = await screen.findByRole("button", { name: "Last 24h" });
    expect(preset).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Last 7d" })).toHaveAttribute("aria-pressed", "false");
  });
});
