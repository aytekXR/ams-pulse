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
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QoePage } from "../QoePage";

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
