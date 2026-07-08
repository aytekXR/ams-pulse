/**
 * App shell smoke tests.
 *
 * App wraps BrowserRouter + ToastProvider + AppRoutes.  AppRoutes renders AuthGate
 * which checks localStorage for a token. When a token is present, Layout + the
 * active route component are rendered.
 *
 * Covers:
 * (a) App renders without crash.
 * (b) Router mounts — main navigation is present.
 * (c) Default route ("/") renders the Live Dashboard heading.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { App } from "../App";

// Keep real API functions so msw can intercept fetch calls, but replace
// LiveSocket (WebSocket) with a no-op to avoid jsdom WS issues.
vi.mock("@/api/client", async (importActual) => {
  const actual = await importActual<typeof import("@/api/client")>();
  return {
    ...actual,
    // Provide a token so AuthGate passes through to the protected shell
    getToken: () => "plt_test_token",
    setToken: vi.fn(),
    clearToken: vi.fn(),
    LiveSocket: class NoOpLiveSocket {
      get connected() { return false; }
      subscribe(_fn: unknown) { return () => {}; }
      connect() {}
      destroy() {}
    },
  };
});

// StreamsTable uses @tanstack/react-virtual which needs real DOM measurements
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: Math.min(count, 10) }, (_, i) => ({
        index: i, start: i * 44, size: 44, key: i, lane: 0, end: (i + 1) * 44,
      })),
    getTotalSize: () => count * 44,
  }),
}));

describe("App shell", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("(a) renders without crashing", () => {
    expect(() => render(<App />)).not.toThrow();
  });

  it("(b) router mounts — main navigation landmark is present", () => {
    render(<App />);
    expect(screen.getByRole("navigation", { name: /main navigation/i })).toBeInTheDocument();
  });

  it("(c) default route renders the Live Dashboard heading", async () => {
    render(<App />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /live dashboard/i })).toBeInTheDocument();
    });
  });

  it("(d) Pulse brand name appears in the sidebar nav", () => {
    render(<App />);
    expect(screen.getByText("Pulse")).toBeInTheDocument();
  });
});
