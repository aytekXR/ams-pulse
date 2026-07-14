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
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { FleetPage, cpuStatus, memStatus } from "../FleetPage";
import type { FleetNode } from "@/lib/api/types";
import { ThemeProvider } from "@/lib/ThemeContext";
import { STATUS_COLORS, LIGHT_STATUS_COLORS, CHART_COLORS } from "@/lib/chartColors";
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
    status: "degraded",
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
      // 3 total, 1 up, 2 degraded, 1 origin, 2 edge
      expect(screen.getByText("3")).toBeInTheDocument(); // total
    });
  });

  it("switches to table view when the Table segment is chosen", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("node-origin-1")).toBeInTheDocument();
    });
    // The view switch is a SegmentedControl: radiogroup / radio, not buttons.
    const tableSeg = screen.getByRole("radio", { name: "Table" });
    expect(tableSeg).toHaveAttribute("aria-checked", "false");
    fireEvent.click(tableSeg);
    await waitFor(() => {
      expect(screen.getByText("Node ID")).toBeInTheDocument();
      expect(screen.getByText("Role")).toBeInTheDocument();
      expect(screen.getByText("Status")).toBeInTheDocument();
      expect(screen.getByText("Last Seen")).toBeInTheDocument();
    });
    expect(screen.getByRole("radio", { name: "Table" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "Cards" })).toHaveAttribute("aria-checked", "false");
  });

  it("exposes the view switch as a named radiogroup", async () => {
    mockListNodes.mockResolvedValue({ items: sampleNodes, meta: {} });
    render(<FleetPage />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText("node-origin-1")).toBeInTheDocument();
    });
    expect(screen.getByRole("radiogroup", { name: "Fleet view" })).toBeInTheDocument();
    // NOT a tablist: there are no tabpanels behind it (see SegmentedControl.tsx).
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
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

// ─── LoadBar fill colour — RENDERED, not re-derived ──────────────────────────
//
// These replace twelve tests that asserted `STATUS_COLORS[cpuStatus(85)] === "#FF5C68"`.
// That composes two values the test file imported itself; it never rendered FleetPage,
// so it stayed green no matter what colour the component actually painted. One of them
// was worse than vacuous: it asserted STATUS_COLORS[memStatus(50)] === "#2CE5A7" while
// the component deliberately paints healthy memory dataviz-BLUE — pinning a value the
// component never uses.
//
// These render the page and read the fill off the DOM.

/** jsdom may serialise an inline colour as hex or rgb(); normalise both sides. */
function normalizeColor(v: string): string {
  const m = v.trim().match(/^#([0-9a-fA-F]{6})$/);
  if (!m) return v.trim();
  const n = parseInt(m[1], 16);
  return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`;
}

/** Fills in DOM order within a NodeCard: [0] = CPU, [1] = Memory. */
async function renderFillsFor(node: Partial<FleetNode>): Promise<HTMLElement[]> {
  mockListNodes.mockResolvedValue({
    items: [{ node_id: "n1", role: "origin", status: "up", last_seen: Date.now(), ...node }],
    meta: {},
  });
  render(<FleetPage />, { wrapper });
  await waitFor(() => expect(screen.getByText("n1")).toBeInTheDocument());
  return screen.getAllByTestId("loadbar-fill");
}

function expectFill(el: HTMLElement, expected: string) {
  expect(normalizeColor(el.style.background)).toBe(normalizeColor(expected));
}

describe("CPU LoadBar paints the theme-correct status colour (rendered)", () => {
  it("dark: cpu 85 renders critical, 65 warning, 45 healthy", async () => {
    document.documentElement.setAttribute("data-theme", "dark");
    expectFill((await renderFillsFor({ cpu_pct: 85 }))[0], STATUS_COLORS.critical);
    cleanup();
    expectFill((await renderFillsFor({ cpu_pct: 65 }))[0], STATUS_COLORS.warning);
    cleanup();
    expectFill((await renderFillsFor({ cpu_pct: 45 }))[0], STATUS_COLORS.healthy);
  });

  it("light: cpu 85 renders the LIGHT critical hex, not the dark one", async () => {
    document.documentElement.setAttribute("data-theme", "light");
    const fills = await renderFillsFor({ cpu_pct: 85 });
    expectFill(fills[0], LIGHT_STATUS_COLORS.critical);
    // Guards the theme switch itself: the dark hex must NOT leak into light theme.
    expect(normalizeColor(fills[0].style.background)).not.toBe(normalizeColor(STATUS_COLORS.critical));
  });
});

// ─── memStatus — pure threshold logic ────────────────────────────────────────
//
// Maps mem % to a status tier. Thresholds differ from cpuStatus:
//   > 85 → critical, > 70 → warning, else → healthy.
// "healthy" memory intentionally renders as dataviz blue, not status green
// (documented in FleetPage.tsx comment at the memStatus definition).

describe("memStatus — pure threshold logic", () => {
  it("pct > 85 is critical", () => {
    expect(memStatus(86)).toBe("critical");
    expect(memStatus(100)).toBe("critical");
    expect(memStatus(85.1)).toBe("critical");
  });

  it("boundary: pct = 85 is warning (not critical)", () => {
    expect(memStatus(85)).toBe("warning");
  });

  it("pct > 70 and <= 85 is warning", () => {
    expect(memStatus(71)).toBe("warning");
    expect(memStatus(80)).toBe("warning");
    expect(memStatus(85)).toBe("warning");
  });

  it("boundary: pct = 70 is healthy (not warning)", () => {
    expect(memStatus(70)).toBe("healthy");
  });

  it("pct <= 70 is healthy", () => {
    expect(memStatus(70)).toBe("healthy");
    expect(memStatus(50)).toBe("healthy");
    expect(memStatus(0)).toBe("healthy");
  });
});

describe("Memory LoadBar paints the theme-correct colour (rendered)", () => {
  it("dark: mem 90 renders critical, 75 warning", async () => {
    document.documentElement.setAttribute("data-theme", "dark");
    expectFill((await renderFillsFor({ cpu_pct: 10, mem_pct: 90 }))[1], STATUS_COLORS.critical);
    cleanup();
    expectFill((await renderFillsFor({ cpu_pct: 10, mem_pct: 75 }))[1], STATUS_COLORS.warning);
  });

  /**
   * THE LOAD-BEARING PIN.
   *
   * Healthy memory is deliberately dataviz-BLUE (CHART_COLORS[1]), not status-green:
   * normal memory is a secondary metric, not a health signal. Nothing pinned that at
   * render level before — a "fix" changing it to statusColors.healthy would have left
   * the whole suite green. Both halves are asserted: it IS blue, and it is NOT green.
   */
  it("dark: healthy memory is dataviz blue, NOT status green", async () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const fills = await renderFillsFor({ cpu_pct: 10, mem_pct: 50 });
    expectFill(fills[1], CHART_COLORS[1]);
    expect(normalizeColor(fills[1].style.background)).not.toBe(normalizeColor(STATUS_COLORS.healthy));
    // CPU at the same healthy tier IS green — proving the two bars differ by design.
    expectFill(fills[0], STATUS_COLORS.healthy);
  });

  it("light: healthy memory stays the same dataviz blue (it is not theme-dependent)", async () => {
    document.documentElement.setAttribute("data-theme", "light");
    const fills = await renderFillsFor({ cpu_pct: 10, mem_pct: 50 });
    expectFill(fills[1], CHART_COLORS[1]);
    expect(normalizeColor(fills[1].style.background)).not.toBe(normalizeColor(LIGHT_STATUS_COLORS.healthy));
  });

  it("light: mem 90 renders the LIGHT critical hex", async () => {
    document.documentElement.setAttribute("data-theme", "light");
    expectFill((await renderFillsFor({ cpu_pct: 10, mem_pct: 90 }))[1], LIGHT_STATUS_COLORS.critical);
  });
});

describe("Wave 2 — brandkit compliance (source-level)", () => {
  // Reads the real component file, so a reintroduced hex/muted token fails here.
  const here = dirname(fileURLToPath(import.meta.url));
  const fleet = readFileSync(resolve(here, "../FleetPage.tsx"), "utf-8");

  it("W2-F1: no bare hex remains — the dataviz blue is named CHART_COLORS[1]", () => {
    expect(fleet).toContain("CHART_COLORS[1]");
    expect(fleet).not.toMatch(/#[0-9A-Fa-f]{6}/);
  });

  it("W2-F2: --color-muted is gone (fails AA for 11–12px text in both themes)", () => {
    expect(fleet).not.toContain("--color-muted");
  });

  it("W2-F3: the dead+stale var() colour fallbacks are gone", () => {
    // `var(--color-warning, #FFB224)` was unreachable (the var is defined in both
    // themes) AND wrong (light theme is #B45309). Same defect Wave 1 removed from QoE.
    expect(fleet).not.toMatch(/var\(--color-\w+,\s*#/);
  });
});

describe("LoadBar exposes its tier to assistive tech (colour-not-only)", () => {
  it("renders the status tier as screen-reader text, not colour alone", async () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const fills = await renderFillsFor({ cpu_pct: 85, mem_pct: 50 });
    expect(fills[0]).toHaveAttribute("data-status", "critical");
    expect(fills[1]).toHaveAttribute("data-status", "healthy");
    // The tier reaches the accessibility tree as text.
    expect(screen.getAllByText("critical").length).toBeGreaterThanOrEqual(1);
  });
});
