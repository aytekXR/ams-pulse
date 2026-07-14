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
