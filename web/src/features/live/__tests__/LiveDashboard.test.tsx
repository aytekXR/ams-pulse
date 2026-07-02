/**
 * LiveDashboard MSW-based integration tests.
 *
 * msw intercepts GET /api/v1/live/overview and GET /api/v1/live/streams,
 * returning realistic stub data (see src/test/mocks/server.ts).  Tests
 * assert on REAL DOM output — not on the mocks themselves — so they fail
 * if the component renders nothing or ignores the API response.
 *
 * LiveSocket is replaced with a no-op via vi.importActual so that the real
 * liveApi functions are preserved (allowing msw to intercept them) while
 * no WebSocket is ever created, avoiding msw's WS interceptor.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { LiveDashboard } from "../LiveDashboard";

// Keep all real API functions (liveApi, alertsApi, etc.) so msw can intercept
// their fetch calls, but replace LiveSocket with a no-op to prevent any
// WebSocket creation in jsdom.
vi.mock("@/api/client", async (importActual) => {
  const actual = await importActual<typeof import("@/api/client")>();
  return {
    ...actual,
    LiveSocket: class NoOpLiveSocket {
      get connected() { return false; }
      subscribe(_fn: unknown) { return () => {}; }
      connect() {}
      destroy() {}
    },
  };
});

// @tanstack/react-virtual requires real DOM measurements unavailable in jsdom.
// The stub renders up to 10 rows deterministically.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: Math.min(count, 10) }, (_, i) => ({
        index: i,
        start: i * 44,
        size: 44,
        key: i,
        lane: 0,
        end: (i + 1) * 44,
      })),
    getTotalSize: () => count * 44,
  }),
}));

// ── Tests ───────────────────────────────────────────────────────────────────

describe("LiveDashboard (msw)", () => {
  it("renders the Live Dashboard heading immediately", () => {
    render(<LiveDashboard />);
    expect(
      screen.getByRole("heading", { name: /live dashboard/i })
    ).toBeInTheDocument();
  });

  it("renders a Refresh button", () => {
    render(<LiveDashboard />);
    expect(
      screen.getByRole("button", { name: /refresh/i })
    ).toBeInTheDocument();
  });

  it("shows a stream row from the msw-intercepted /live/streams response", async () => {
    render(<LiveDashboard />);
    // msw returns items[0].stream_id = "test-stream-1"
    await waitFor(() => {
      expect(screen.getByText("test-stream-1")).toBeInTheDocument();
    });
  });

  it("shows total viewer count from the msw-intercepted /live/overview response", async () => {
    render(<LiveDashboard />);
    // msw returns total_viewers: 42; the Viewers StatCard renders it as "42".
    // "42" also appears in the per-app viewers table, so use getAllByText to
    // avoid the "Found multiple elements" ambiguity error.
    await waitFor(() => {
      const matches = screen.getAllByText("42");
      expect(matches.length).toBeGreaterThan(0);
    });
  });

  it("renders the 'Protocol mix' section heading after data loads", async () => {
    render(<LiveDashboard />);
    await waitFor(() => {
      expect(screen.getByText(/protocol mix/i)).toBeInTheDocument();
    });
  });

  it("renders the 'By application' section after data loads", async () => {
    render(<LiveDashboard />);
    await waitFor(() => {
      expect(screen.getByText(/by application/i)).toBeInTheDocument();
    });
  });

  it("shows the 'live' app in the applications table from overview data", async () => {
    render(<LiveDashboard />);
    // msw overview: apps: [{ app: "live", ... }]
    // Multiple elements may show "live" (apps table + stream rows), so use getAllByText.
    await waitFor(() => {
      const matches = screen.getAllByText("live");
      expect(matches.length).toBeGreaterThan(0);
    });
  });

  it("shows the Active streams heading with the count", async () => {
    render(<LiveDashboard />);
    // The <h2> heading renders "Active streams (N)".
    // The Publishers StatCard also has sub="active streams" so /active streams/i
    // matches multiple elements.  Use getByRole to target only the <h2>.
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: /active streams/i })
      ).toBeInTheDocument();
    });
  });

  it("shows the stream count footer from StreamsTable (1 stream)", async () => {
    render(<LiveDashboard />);
    // StreamsTable footer: "1 stream" (streams.length === 1)
    await waitFor(() => {
      expect(screen.getByText(/\b1 stream\b/)).toBeInTheDocument();
    });
  });
});
