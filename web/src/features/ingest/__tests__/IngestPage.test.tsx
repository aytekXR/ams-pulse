/**
 * IngestPage rendering tests (VD-26).
 *
 * Covers:
 * (a) Smoke render — page mounts without crashing.
 * (b) Empty state — when /qoe/ingest returns {streams: []}, empty-state message renders
 *     instead of a blank/crashed chart.
 * (c) Populated — when a stream returns health_score and timeseries with bitrate_kbps,
 *     the health label and bitrate value are visible.
 * (d) Wave 3 — chart colour tokens, drop-events a11y, close-button a11y.
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { ThemeProvider } from "@/lib/ThemeContext";
import { IngestPage } from "../IngestPage";
import type { IngestHealthResponse } from "@/lib/api/types";

// StreamDetail calls useStatusColors() → useTheme() → requires ThemeProvider.
// Wrap only the tests that open the detail panel (clicking Details).
function withTheme({ children }: { children: ReactNode }) {
  return <ThemeProvider>{children}</ThemeProvider>;
}

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
    // StreamDetail calls useStatusColors() so wrap with ThemeProvider
    render(<IngestPage />, { wrapper: withTheme });
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

// ─── Wave 3: chart colour tokens (source-read — Recharts props not in DOM) ────
//
// Recharts writes stroke/fill as SVG presentation attributes during real
// browser layout; jsdom does not lay out SVG so these values are never
// available in the rendered DOM. Instead we read the component source and
// assert the token expression rather than a bare hex literal.

describe("IngestPage — Wave 3 chart colour tokens (source-read)", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const src = readFileSync(resolve(here, "../IngestPage.tsx"), "utf-8");

  it("Bitrate line uses CHART_COLORS[1] (dataviz token, not a bare hex)", () => {
    expect(src).toContain('stroke={CHART_COLORS[1]}');
    expect(src).not.toContain('stroke="#58A6FF"');
  });

  it("FPS line uses CHART_COLORS[0] (dataviz token, not a bare hex)", () => {
    expect(src).toContain('stroke={CHART_COLORS[0]}');
    expect(src).not.toContain('stroke="#2CE5A7"');
  });

  it("Jitter line uses CHART_COLORS[4] (dataviz token, not a bare hex)", () => {
    expect(src).toContain('stroke={CHART_COLORS[4]}');
    expect(src).not.toContain('stroke="#FFB224"');
  });

  it("Packet Loss line routes through statusColors.critical (not a bare hex)", () => {
    // statusColors.critical is theme-correct: dark error in dark theme,
    // light error in light theme. A bare hex is always dark-only.
    expect(src).toContain('stroke={statusColors.critical}');
    expect(src).not.toContain('stroke="#FF5C68"');
  });

  it("drop ReferenceLine stroke uses statusColors.critical (RULE 3 — SVG attr)", () => {
    // ReferenceLine stroke must be a JS value, not a var() string.
    expect(src).toMatch(/ReferenceLine[^>]*stroke=\{statusColors\.critical\}/s);
  });

  it("threshold ReferenceLine stroke uses statusColors.warning (RULE 3 — SVG attr)", () => {
    expect(src).toMatch(/ReferenceLine[^>]*stroke=\{statusColors\.warning\}/s);
  });

  it("no var() fallbacks remain in CSS properties (RULE 4)", () => {
    // All var(--x, #hex) fallbacks must have been dropped.
    expect(src).not.toMatch(/var\(--color-[^)]+,\s*#[0-9A-Fa-f]{6}/);
  });

  it("no bare hex literals remain in source (RULE 10)", () => {
    expect(src).not.toMatch(/#[0-9A-Fa-f]{6}/);
  });
});

// ─── Wave 3: a11y — drop events panel and close button ────────────────────────

const dropStream: IngestHealthResponse = {
  streams: [
    {
      stream_id: "live/drop-stream",
      app: "live",
      node_id: "node-2",
      health_score: 45,
      timeseries: [{ ts: Date.now() - 60_000, bitrate_kbps: 1200, fps: 25 }],
      drop_events: [{ ts: Date.now() - 30_000, reason: "buffer overflow" }],
    },
  ],
};

describe("IngestPage — Wave 3 a11y additions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("drop events panel has aria-live='polite' so it is announced on update", async () => {
    mockGetIngestHealth.mockResolvedValue(dropStream);
    const { container } = render(<IngestPage />, { wrapper: withTheme });

    // Open the detail panel first
    await waitFor(() => {
      expect(screen.getByText("live/drop-stream")).toBeInTheDocument();
    });
    screen.getByRole("button", { name: /details/i }).click();

    await waitFor(() => {
      const panel = container.querySelector('[aria-live="polite"]');
      expect(panel).not.toBeNull();
    });
  });

  it("drop events panel has aria-label='Drop events'", async () => {
    mockGetIngestHealth.mockResolvedValue(dropStream);
    const { container } = render(<IngestPage />, { wrapper: withTheme });

    await waitFor(() => {
      expect(screen.getByText("live/drop-stream")).toBeInTheDocument();
    });
    screen.getByRole("button", { name: /details/i }).click();

    await waitFor(() => {
      const panel = container.querySelector('[aria-label="Drop events"]');
      expect(panel).not.toBeNull();
    });
  });

  it("Close button has an accessible aria-label", async () => {
    mockGetIngestHealth.mockResolvedValue(dropStream);
    render(<IngestPage />, { wrapper: withTheme });

    await waitFor(() => {
      expect(screen.getByText("live/drop-stream")).toBeInTheDocument();
    });
    screen.getByRole("button", { name: /details/i }).click();

    await waitFor(() => {
      // The close button must be findable by its accessible name, not just its text
      expect(screen.getByRole("button", { name: /close stream detail/i })).toBeInTheDocument();
    });
  });

  it("health progress bar is aria-hidden (Badge provides the text equivalent)", async () => {
    mockGetIngestHealth.mockResolvedValue(populatedResponse);
    const { container } = render(<IngestPage />);

    await waitFor(() => {
      expect(screen.getByText("live/test-stream")).toBeInTheDocument();
    });

    // Anchored to the health bar itself. A `querySelectorAll('[aria-hidden="true"]').length > 0`
    // assertion was written here first and could not fail: the always-present status dot is also
    // aria-hidden, so it kept the count above zero even if the health bar lost the attribute
    // entirely — the exact regression the test claims to guard.
    const bar = container.querySelector('[data-testid="health-bar-bg"]');
    expect(bar).not.toBeNull();
    expect(bar!.getAttribute("aria-hidden")).toBe("true");
  });
});
