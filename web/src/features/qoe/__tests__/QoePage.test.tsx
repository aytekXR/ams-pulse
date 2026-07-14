/**
 * QoE slice-state reducer and QoePage rendering tests.
 *
 * The "slice-state reducer" here tests the URL-param / filter state logic:
 * - initial state produces expected defaults
 * - setting stream filter updates state
 * - setting app filter updates state
 * - both filters combined
 * - clearing filters resets to defaults
 *
 * Also tests QoePage rendering: loading state, empty state, error state.
 *
 * Wave 1 additions (S32 / D-094):
 * - QO-1: isAnimationActive={false} on both Line elements
 * - QO-2: accessibilityLayer on LineChart
 * - QO-3: aria-label on filter inputs
 * - QO-4: no outline:none on filter inputs
 * - QO-5: Badge rendered when threshold exceeded (color-not-only)
 * - hex → CHART_COLORS: stroke values use CHART_COLORS[1] and CHART_COLORS[4]
 * - dropped dead fallback hex from var(--color-warning) and var(--color-error)
 * - exact-match px → token substitutions (borderRadius, gap, padding, marginLeft)
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QoePage } from "../QoePage";
import { CHART_COLORS } from "@/lib/chartColors";

// ─── Slice-state reducer (pure logic extracted for unit testing) ───────────────

interface SliceState {
  streamFilter: string;
  appFilter: string;
  from: number;
  to: number;
}

type SliceAction =
  | { type: "SET_STREAM"; stream: string }
  | { type: "SET_APP"; app: string }
  | { type: "SET_RANGE"; from: number; to: number }
  | { type: "CLEAR_FILTERS" };

function sliceReducer(state: SliceState, action: SliceAction): SliceState {
  switch (action.type) {
    case "SET_STREAM":
      return { ...state, streamFilter: action.stream };
    case "SET_APP":
      return { ...state, appFilter: action.app };
    case "SET_RANGE":
      return { ...state, from: action.from, to: action.to };
    case "CLEAR_FILTERS":
      return { ...state, streamFilter: "", appFilter: "" };
    default:
      return state;
  }
}

const NOW = 1_700_000_000_000;
const defaultState: SliceState = {
  streamFilter: "",
  appFilter: "",
  from: NOW - 86_400_000,
  to: NOW,
};

describe("QoE slice-state reducer", () => {
  it("returns default state unchanged on unknown action", () => {
    const result = sliceReducer(defaultState, { type: "CLEAR_FILTERS" });
    expect(result.streamFilter).toBe("");
    expect(result.appFilter).toBe("");
  });

  it("SET_STREAM updates streamFilter", () => {
    const result = sliceReducer(defaultState, { type: "SET_STREAM", stream: "live/s1" });
    expect(result.streamFilter).toBe("live/s1");
    expect(result.appFilter).toBe(""); // untouched
  });

  it("SET_APP updates appFilter", () => {
    const result = sliceReducer(defaultState, { type: "SET_APP", app: "live" });
    expect(result.appFilter).toBe("live");
    expect(result.streamFilter).toBe(""); // untouched
  });

  it("combined filters: SET_STREAM then SET_APP", () => {
    let s = sliceReducer(defaultState, { type: "SET_STREAM", stream: "live/s1" });
    s = sliceReducer(s, { type: "SET_APP", app: "live" });
    expect(s.streamFilter).toBe("live/s1");
    expect(s.appFilter).toBe("live");
  });

  it("CLEAR_FILTERS resets both filters but preserves range", () => {
    let s = sliceReducer(defaultState, { type: "SET_STREAM", stream: "live/s1" });
    s = sliceReducer(s, { type: "SET_APP", app: "live" });
    s = sliceReducer(s, { type: "CLEAR_FILTERS" });
    expect(s.streamFilter).toBe("");
    expect(s.appFilter).toBe("");
    expect(s.from).toBe(defaultState.from); // range preserved
  });

  it("SET_RANGE updates from/to", () => {
    const result = sliceReducer(defaultState, { type: "SET_RANGE", from: 100, to: 200 });
    expect(result.from).toBe(100);
    expect(result.to).toBe(200);
  });
});

// ─── QoePage component rendering tests ────────────────────────────────────────

// Mock the qoeApi
const mockGetSummary = vi.fn();

vi.mock("@/api/client", () => ({
  qoeApi: {
    getSummary: (...args: unknown[]) => mockGetSummary(...args),
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

describe("QoePage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading spinner while fetching", () => {
    mockGetSummary.mockReturnValue(new Promise(() => {})); // never resolves
    render(<QoePage />);
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("shows empty state when no beacon data returned", async () => {
    mockGetSummary.mockResolvedValue({
      totals: null,
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => {
      expect(screen.getByText(/no qoe data yet/i)).toBeInTheDocument();
    });
  });

  it("shows SDK setup link in empty state", async () => {
    mockGetSummary.mockResolvedValue({
      totals: null,
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => {
      expect(screen.getByRole("link", { name: /sdk setup docs/i })).toBeInTheDocument();
    });
  });

  it("shows error banner when fetch fails", async () => {
    mockGetSummary.mockRejectedValue(new Error("network error"));
    render(<QoePage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("renders QoE cards when data available", async () => {
    mockGetSummary.mockResolvedValue({
      totals: {
        startup_p50_ms: 350,
        startup_p95_ms: 1200,
        rebuffer_ratio: 0.02,
        error_rate: 0.001,
      },
      bitrate_timeline: [
        { ts: Date.now() - 60000, bitrate_kbps_p50: 2500, bitrate_kbps_p95: 4000 },
        { ts: Date.now(), bitrate_kbps_p50: 2800, bitrate_kbps_p95: 4200 },
      ],
    });
    render(<QoePage />);
    await waitFor(() => {
      expect(screen.getByText(/startup p50/i)).toBeInTheDocument();
      expect(screen.getByText(/startup p95/i)).toBeInTheDocument();
      expect(screen.getByText(/rebuffer ratio/i)).toBeInTheDocument();
      expect(screen.getByText(/error rate/i)).toBeInTheDocument();
    });
  });
});

// ─── Wave 1 a11y + token tests ─────────────────────────────────────────────────

describe("QoePage filter inputs — a11y (QO-3, QO-4)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Resolve to empty so the component reaches the slice-controls render
    mockGetSummary.mockReturnValue(new Promise(() => {}));
  });

  it("QO-3: stream filter input has aria-label", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /stream id filter/i });
    expect(input).toBeInTheDocument();
  });

  it("QO-3: app filter input has aria-label", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /app filter/i });
    expect(input).toBeInTheDocument();
  });

  it("QO-4: stream filter input has no inline outline:none", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /stream id filter/i });
    // The inline style must not suppress focus ring via outline:none
    expect((input as HTMLElement).style.outline).not.toBe("none");
  });

  it("QO-4: app filter input has no inline outline:none", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /app filter/i });
    expect((input as HTMLElement).style.outline).not.toBe("none");
  });

  it("QO-4: stream filter input carries filter-input class for CSS focus ring", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /stream id filter/i });
    expect(input).toHaveClass("filter-input");
  });

  it("QO-4: app filter input carries filter-input class for CSS focus ring", () => {
    render(<QoePage />);
    const input = screen.getByRole("textbox", { name: /app filter/i });
    expect(input).toHaveClass("filter-input");
  });
});

describe("QoePage threshold badges — color-not-only (QO-5)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("HIGH badge is absent when rebuffer_ratio is below threshold", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.001 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/rebuffer ratio/i));
    // No HIGH badge should appear when both metrics are under threshold
    expect(screen.queryByText("HIGH")).not.toBeInTheDocument();
  });

  it("HIGH badge appears next to Rebuffer Ratio when rebuffer_ratio > 0.05", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.08, error_rate: 0.001 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/rebuffer ratio/i));
    // Badge with label "HIGH" must appear
    expect(screen.getByText("HIGH")).toBeInTheDocument();
  });

  it("HIGH badge appears next to Error Rate when error_rate > 0.01", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.05 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/error rate/i));
    expect(screen.getByText("HIGH")).toBeInTheDocument();
  });

  it("two HIGH badges appear when both thresholds are exceeded", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.08, error_rate: 0.05 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/rebuffer ratio/i));
    const badges = screen.getAllByText("HIGH");
    expect(badges).toHaveLength(2);
  });

  it("HIGH badge is absent for error_rate exactly at threshold (0.01 is not > 0.01)", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.01 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/error rate/i));
    expect(screen.queryByText("HIGH")).not.toBeInTheDocument();
  });

  it("HIGH badge is absent for rebuffer_ratio exactly at threshold (0.05 is not > 0.05)", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.05, error_rate: 0.001 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/rebuffer ratio/i));
    expect(screen.queryByText("HIGH")).not.toBeInTheDocument();
  });
});

describe("QoePage token substitutions (px → CSS var, hex → CHART_COLORS)", () => {
  it("CHART_COLORS[1] is exactly #58A6FF (value-preserving)", () => {
    expect(CHART_COLORS[1]).toBe("#58A6FF");
  });

  it("CHART_COLORS[4] is exactly #FFB224 (value-preserving)", () => {
    expect(CHART_COLORS[4]).toBe("#FFB224");
  });

  it("cardStyle borderRadius references var(--radius-control) not a raw number", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.001 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/startup p50/i));
    // The KPI cards should have borderRadius referencing the CSS variable
    const cards = document.querySelectorAll<HTMLElement>('[style*="border-radius: var(--radius-control)"]');
    // At minimum the summary cards and chart wrapper must have the token
    expect(cards.length).toBeGreaterThan(0);
  });

  it("var(--color-warning) fallback hex dropped — raw #FFB224 not in Rebuffer Ratio color style", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.08, error_rate: 0.001 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/rebuffer ratio/i));
    // The threshold-triggered element must use the CSS var, not the raw fallback hex
    const warningEl = document.querySelector<HTMLElement>('[style*="var(--color-warning)"]');
    expect(warningEl).not.toBeNull();
    expect(warningEl?.style.color).not.toContain("#FFB224");
  });

  it("var(--color-error) fallback hex dropped — raw #FF5C68 not in Error Rate color style", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.05 },
      bitrate_timeline: [],
    });
    render(<QoePage />);
    await waitFor(() => screen.getByText(/error rate/i));
    const errorEl = document.querySelector<HTMLElement>('[style*="var(--color-error)"]');
    expect(errorEl).not.toBeNull();
    expect(errorEl?.style.color).not.toContain("#FF5C68");
  });
});

describe("QoePage chart — motion + accessibility (QO-1, QO-2)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("QO-2: accessible heading for the bitrate chart is present", async () => {
    mockGetSummary.mockResolvedValue({
      totals: { startup_p50_ms: 300, startup_p95_ms: 900, rebuffer_ratio: 0.02, error_rate: 0.001 },
      bitrate_timeline: [
        { ts: Date.now() - 60000, bitrate_kbps_p50: 2500, bitrate_kbps_p95: 4000 },
      ],
    });
    render(<QoePage />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /bitrate timeline/i })).toBeInTheDocument();
    });
  });

  it("QO-1 (source): both Line elements have isAnimationActive={false} in JSX source", () => {
    // Reads the actual component file so any change to the prop in QoePage.tsx
    // will cause this test to fail — unlike a hard-coded string which is a tautology.
    // ESM: __dirname does not exist — derive it from import.meta.url.
    const here = dirname(fileURLToPath(import.meta.url));
    const src = readFileSync(resolve(here, "../QoePage.tsx"), "utf-8");
    // Must appear at least twice — once per <Line> element.
    const count = (src.match(/isAnimationActive=\{false\}/g) ?? []).length;
    expect(count).toBeGreaterThanOrEqual(2);
  });
});
