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

// ─── B2 sweep: health-bar uses CSS vars ──────────────────────────────────────
//
// The health-score progress bar previously used hardcoded hex colors. It must
// now reference CSS custom properties so the correct value is resolved for each
// theme by global.css.
//
// Uses a stream with health_score < 50 to trigger the critical branch (var(--color-error)).

describe("IngestPage — health-bar CSS var (B2 sweep)", () => {
  it("critical health bar uses var(--color-error) not hardcoded #FF5C68", async () => {
    const criticalStream = {
      ...populatedResponse.streams[0],
      health_score: 30, // < 50 → critical branch
    };
    mockGetIngestHealth.mockResolvedValue({ streams: [criticalStream] });
    const { container } = render(<IngestPage />);

    await waitFor(() => {
      expect(screen.getByText(/live\/test-stream/)).toBeInTheDocument();
    });

    // Find the health bar div (has a width% inline style and background color)
    const bars = Array.from(
      container.querySelectorAll("div[style]"),
    ) as HTMLElement[];
    const healthBar = bars.find((el) => {
      const style = el.getAttribute("style") ?? el.style.cssText;
      return style.includes("width: 30%") || style.includes("width:30%");
    });
    expect(healthBar).toBeDefined();
    const styleText = healthBar!.getAttribute("style") ?? healthBar!.style.cssText;
    expect(styleText).toContain("var(--color-error)");
  });

  it("good health bar uses var(--color-success) not hardcoded #2CE5A7", async () => {
    mockGetIngestHealth.mockResolvedValue(populatedResponse); // health_score 95
    const { container } = render(<IngestPage />);

    await waitFor(() => {
      expect(screen.getByText(/live\/test-stream/)).toBeInTheDocument();
    });

    const bars = Array.from(
      container.querySelectorAll("div[style]"),
    ) as HTMLElement[];
    const healthBar = bars.find((el) => {
      const style = el.getAttribute("style") ?? el.style.cssText;
      return style.includes("width: 95%") || style.includes("width:95%");
    });
    expect(healthBar).toBeDefined();
    const styleText = healthBar!.getAttribute("style") ?? healthBar!.style.cssText;
    expect(styleText).toContain("var(--color-success)");
  });
});
