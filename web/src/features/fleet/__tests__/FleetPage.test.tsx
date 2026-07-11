/**
 * Fleet node-state rendering tests.
 *
 * Tests:
 * - Loading state
 * - Empty state when no nodes
 * - Node cards render with role/status badges
 * - Node table view renders all columns
 * - Health color logic — cpuStatus pure threshold + both palettes (dark & light)
 * - Aggregate header counts
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { FleetPage, cpuStatus } from "../FleetPage";
import type { FleetNode } from "@/lib/api/types";
import { ThemeProvider } from "@/lib/ThemeContext";
import { STATUS_COLORS, LIGHT_STATUS_COLORS } from "@/lib/chartColors";
import type { ReactNode } from "react";

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

// Wrap renders with ThemeProvider — FleetPage calls useStatusColors().
function wrapper({ children }: { children: ReactNode }) {
  return <ThemeProvider>{children}</ThemeProvider>;
}

describe("FleetPage rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.documentElement.setAttribute("data-theme", "dark");
  });

  it("shows loading spinner while fetching", () => {
    mockListNodes.mockReturnValue(new Promise(() => {}));
    render(<FleetPage />, { wrapper });
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("shows empty state when no nodes discovered", async () => {
    mockListNodes.mockResolvedValue({ items: [], meta: {} });
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText(/no fleet nodes discovered/i)).toBeInTheDocument();
    });
  });

  it("shows error banner on fetch failure", async () => {
    mockListNodes.mockRejectedValue(new Error("network error"));
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
  });

  it("renders node cards with role badges", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />, { wrapper });
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
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      // 3 total, 1 up, 1 degraded, 1 down, 1 origin, 2 edge
      expect(screen.getByText("3")).toBeInTheDocument(); // total
    });
  });

  it("switches to table view when table button clicked", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />, { wrapper });
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
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText(/2\.9\.1/)).toBeInTheDocument();
    });
  });
});

// ─── CPU threshold — pure logic + both palettes ───────────────────────────────
//
// The cpuStatus() export from FleetPage returns the status tier string.
// STATUS_COLORS and LIGHT_STATUS_COLORS map that string to the theme hex.
// These pins are updated atomically with the FleetPage.tsx LoadBar ternaries.

describe("cpuStatus — pure threshold logic", () => {
  it("pct > 80 is critical", () => {
    expect(cpuStatus(85)).toBe("critical");
    expect(cpuStatus(100)).toBe("critical");
    expect(cpuStatus(81)).toBe("critical");
  });

  it("pct > 60 and <= 80 is warning", () => {
    expect(cpuStatus(65)).toBe("warning");
    expect(cpuStatus(80)).toBe("warning");
    expect(cpuStatus(61)).toBe("warning");
  });

  it("pct <= 60 is healthy", () => {
    expect(cpuStatus(60)).toBe("healthy");
    expect(cpuStatus(0)).toBe("healthy");
    expect(cpuStatus(45)).toBe("healthy");
  });
});

describe("cpuStatus → dark palette hex (STATUS_COLORS)", () => {
  it("critical maps to dark critical #FF5C68", () => {
    expect(STATUS_COLORS[cpuStatus(85)]).toBe("#FF5C68");
    expect(STATUS_COLORS[cpuStatus(100)]).toBe("#FF5C68");
  });

  it("warning maps to dark warning #FFB224", () => {
    expect(STATUS_COLORS[cpuStatus(65)]).toBe("#FFB224");
    expect(STATUS_COLORS[cpuStatus(80)]).toBe("#FFB224");
  });

  it("healthy maps to dark healthy #2CE5A7", () => {
    expect(STATUS_COLORS[cpuStatus(45)]).toBe("#2CE5A7");
    expect(STATUS_COLORS[cpuStatus(0)]).toBe("#2CE5A7");
  });
});

describe("cpuStatus → light palette hex (LIGHT_STATUS_COLORS)", () => {
  it("critical maps to light critical #DC2626", () => {
    expect(LIGHT_STATUS_COLORS[cpuStatus(85)]).toBe("#DC2626");
  });

  it("warning maps to light warning #B45309", () => {
    expect(LIGHT_STATUS_COLORS[cpuStatus(65)]).toBe("#B45309");
  });

  it("healthy maps to light healthy #0BA678", () => {
    expect(LIGHT_STATUS_COLORS[cpuStatus(45)]).toBe("#0BA678");
  });
});
