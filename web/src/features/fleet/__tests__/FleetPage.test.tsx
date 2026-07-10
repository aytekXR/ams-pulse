/**
 * Fleet node-state rendering tests.
 *
 * Tests:
 * - Loading state
 * - Empty state when no nodes
 * - Node cards render with role/status badges
 * - Node table view renders all columns
 * - Health color logic (cpu thresholds)
 * - Aggregate header counts
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { FleetPage } from "../FleetPage";
import type { FleetNode } from "@/lib/api/types";

// Mock the fleet API
const mockListNodes = vi.fn();

vi.mock("@/api/client", () => ({
  fleetApi: {
    listNodes: (...args: unknown[]) => mockListNodes(...args),
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

const sampleNodes: FleetNode[] = [
  {
    node_id: "node-origin-1",
    role: "origin",
    status: "up",
    last_seen: Date.now() - 5000,
    version: "2.9.1",
    cpu_pct: 45,
    mem_pct: 62,
    net_in_mbps: 12.5,
    net_out_mbps: 88.3,
  },
  {
    node_id: "node-edge-1",
    role: "edge",
    status: "degraded",
    last_seen: Date.now() - 30000,
    version: "2.9.0",
    cpu_pct: 85,
    mem_pct: 91,
  },
  {
    node_id: "node-edge-2",
    role: "edge",
    status: "down",
    last_seen: Date.now() - 300000,
    version: "2.8.5",
  },
];

describe("FleetPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading spinner while fetching", () => {
    mockListNodes.mockReturnValue(new Promise(() => {}));
    render(<FleetPage />);
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("shows empty state when no nodes discovered", async () => {
    mockListNodes.mockResolvedValue({ items: [], meta: {} });
    render(<FleetPage />);
    await waitFor(() => {
      expect(screen.getByText(/no fleet nodes discovered/i)).toBeInTheDocument();
    });
  });

  it("shows error banner on fetch failure", async () => {
    mockListNodes.mockRejectedValue(new Error("network error"));
    render(<FleetPage />);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("renders node cards with role badges", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />);
    await waitFor(() => {
      // node-origin-1 should show origin badge
      expect(screen.getByText("node-origin-1")).toBeInTheDocument();
      // Role badges (multiple instances OK — cards + badges)
      const originBadges = screen.getAllByText("origin");
      expect(originBadges.length).toBeGreaterThanOrEqual(1);
      // Status badges
      const upBadges = screen.getAllByText("up");
      expect(upBadges.length).toBeGreaterThanOrEqual(1);
    });
  });

  it("renders aggregate header with correct counts", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />);
    await waitFor(() => {
      // 3 total, 1 up, 1 degraded, 1 down, 1 origin, 2 edge
      expect(screen.getByText("3")).toBeInTheDocument(); // total
    });
  });

  it("switches to table view when table button clicked", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />);
    await waitFor(() => {
      expect(screen.getByText("node-origin-1")).toBeInTheDocument();
    });
    // Click table view toggle
    const tableBtn = screen.getByRole("button", { name: /table/i });
    fireEvent.click(tableBtn);
    // Table headers should appear
    await waitFor(() => {
      expect(screen.getByText("Node ID")).toBeInTheDocument();
      expect(screen.getByText("Role")).toBeInTheDocument();
      expect(screen.getByText("Status")).toBeInTheDocument();
      expect(screen.getByText("Last Seen")).toBeInTheDocument();
    });
  });

  it("renders version numbers", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />);
    await waitFor(() => {
      expect(screen.getByText(/2\.9\.1/)).toBeInTheDocument();
    });
  });
});

// ─── Health color logic (pure unit test) ─────────────────────────────────────

// ─── cpuColor mirrors the inline ternary in FleetPage.tsx LoadBar calls ────────
// Updated atomically with FleetPage.tsx to brandkit STATUS_COLORS (D-071 trap fix).
function cpuColor(pct: number): string {
  if (pct > 80) return "#FF5C68"; // critical
  if (pct > 60) return "#FFB224"; // warning
  return "#2CE5A7"; // healthy
}

describe("Fleet node health color logic", () => {
  it("returns critical red for cpu > 80", () => {
    expect(cpuColor(85)).toBe("#FF5C68");
    expect(cpuColor(100)).toBe("#FF5C68");
  });

  it("returns warning amber for cpu > 60 and <= 80", () => {
    expect(cpuColor(65)).toBe("#FFB224");
    expect(cpuColor(80)).toBe("#FFB224");
  });

  it("returns healthy green for cpu <= 60", () => {
    expect(cpuColor(60)).toBe("#2CE5A7");
    expect(cpuColor(0)).toBe("#2CE5A7");
    expect(cpuColor(45)).toBe("#2CE5A7");
  });
});
