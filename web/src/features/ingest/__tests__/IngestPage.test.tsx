/**
 * IngestPage rendering tests (VD-26).
 *
 * Covers:
 * (a) Smoke render — page mounts without crashing.
 * (b) Empty state — when /qoe/ingest returns {streams: []}, empty-state message renders
 *     instead of a blank/crashed chart.
 * (c) Populated — when a stream returns health_score and timeseries with bitrate_kbps,
 *     the health label and bitrate value are visible.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { IngestPage } from "../IngestPage";
import type { IngestHealthResponse } from "@/lib/api/types";

// Mock the qoeApi — IngestPage calls qoeApi.getIngestHealth
const mockGetIngestHealth = vi.fn();

vi.mock("@/api/client", () => ({
  qoeApi: {
    getIngestHealth: (...args: unknown[]) => mockGetIngestHealth(...args),
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

// ─── Shared fixtures ──────────────────────────────────────────────────────────

const emptyResponse: IngestHealthResponse = { streams: [] };

const populatedResponse: IngestHealthResponse = {
  streams: [
    {
      stream_id: "live/test-stream",
      app: "live",
      node_id: "node-1",
      health_score: 95,
      timeseries: [
        { ts: Date.now() - 60_000, bitrate_kbps: 3200, fps: 30 },
        { ts: Date.now(), bitrate_kbps: 3400, fps: 30 },
      ],
      drop_events: [],
    },
  ],
};

// ─── Tests ────────────────────────────────────────────────────────────────────

describe("IngestPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("(a) smoke — mounts without crash and shows the page title", async () => {
    // Resolve immediately so we don't hang
    mockGetIngestHealth.mockResolvedValue(emptyResponse);
    render(<IngestPage />);
    // The h1 title "Ingest Health" must always be present
    expect(screen.getByRole("heading", { name: /ingest health/i })).toBeInTheDocument();
  });

  it("(b) empty state — renders empty-state message when streams array is empty", async () => {
    mockGetIngestHealth.mockResolvedValue(emptyResponse);
    render(<IngestPage />);
    await waitFor(() => {
      expect(screen.getByText(/no active publishers/i)).toBeInTheDocument();
    });
  });

  it("(b) empty state — does not render the publishers table when streams is empty", async () => {
    mockGetIngestHealth.mockResolvedValue(emptyResponse);
    render(<IngestPage />);
    await waitFor(() => {
      // Empty state is visible
      expect(screen.getByText(/no active publishers/i)).toBeInTheDocument();
    });
    // No "Stream" column header from the table
    expect(screen.queryByText(/^stream$/i)).not.toBeInTheDocument();
  });

  it("(c) populated — renders the stream id when data is available", async () => {
    mockGetIngestHealth.mockResolvedValue(populatedResponse);
    render(<IngestPage />);
    await waitFor(() => {
      expect(screen.getByText("live/test-stream")).toBeInTheDocument();
    });
  });

  it("(c) populated — renders a health label badge for the stream", async () => {
    mockGetIngestHealth.mockResolvedValue(populatedResponse);
    render(<IngestPage />);
    // health_score 95 → "Healthy" badge
    await waitFor(() => {
      expect(screen.getByText(/healthy/i)).toBeInTheDocument();
    });
  });

  it("(c) populated — renders a bitrate value from timeseries in the detail chart", async () => {
    mockGetIngestHealth.mockResolvedValue(populatedResponse);
    render(<IngestPage />);
    // The Bitrate & FPS chart is only rendered inside StreamDetail (detail panel).
    // At list level the stream row is always visible; click Details to expand.
    await waitFor(() => {
      expect(screen.getByText("live/test-stream")).toBeInTheDocument();
    });
    // Details button opens the StreamDetail component which renders the Bitrate & FPS section
    const detailsBtn = screen.getByRole("button", { name: /details/i });
    detailsBtn.click();
    await waitFor(() => {
      expect(screen.getByText(/bitrate/i)).toBeInTheDocument();
    });
  });

  it("shows error banner when fetch fails", async () => {
    mockGetIngestHealth.mockRejectedValue(new Error("network error"));
    render(<IngestPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });
});
