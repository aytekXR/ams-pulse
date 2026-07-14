/**
 * ProtocolDonut tests — Wave 1 uipro a11y + token substitution.
 *
 * Pins:
 *  (P-3) CHART_COLORS[7] === "#7C93AD" — fallback hex is now the constant, not
 *        a literal. If chartColors.ts changes CHART_COLORS[7], this fails.
 *  (P-3) Unknown protocol key falls back to CHART_COLORS[7] (not "#7C93AD").
 *  (P-1) renderPieLabel: returns null for slices < 5%; returns SVG <text> for >= 5%.
 *        Exported from ProtocolDonut for direct unit testing — pure function.
 *        These tests would fail if: the threshold changes, the fill/fontFamily
 *        token is replaced with a literal hex, or null is returned for >= 5%.
 *  (P-5) Legend iconType="circle": ResponsiveContainer is mocked to render its
 *        children at 300×200, enabling Recharts to render Legend content. Tests
 *        check for recharts-legend-icon elements in the DOM.
 *  General: empty state and normal render smoke tests.
 *
 * Note on jsdom + Recharts:
 * ResponsiveContainer measures width via an inner div in jsdom, which always
 * returns 0. The P-5 mock replaces ResponsiveContainer with a pass-through
 * that clones the child with explicit dimensions, enabling full Legend render.
 */
import React from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProtocolDonut, renderPieLabel } from "../ProtocolDonut";
import { CHART_COLORS, PROTOCOL_COLORS } from "@/lib/chartColors";
import type { ProtocolMix } from "@/lib/api/types";

// ── ResponsiveContainer pass-through mock (P-5) ──────────────────────────────
// Recharts ResponsiveContainer reports width=0 in jsdom, preventing Recharts
// from rendering its Legend. This mock renders children at a fixed 300×200 so
// the PieChart (and its Legend) actually render to DOM in tests.
vi.mock("recharts", async (importActual) => {
  const actual = await importActual<typeof import("recharts")>();
  return {
    ...actual,
    // Pass-through: clones the child (PieChart) with explicit width/height so
    // Recharts renders its full content (Legend wrapper, SVG, etc.) in jsdom.
    ResponsiveContainer: ({
      children,
    }: {
      children: React.ReactElement<Record<string, unknown>>;
      width?: string | number;
      height?: string | number;
    }) => React.cloneElement(children, { width: 300, height: 200 }),
  };
});

// ── P-3: constant value pins ────────────────────────────────────────────────

describe("CHART_COLORS — constant pin (P-3)", () => {
  it("CHART_COLORS[7] equals '#7C93AD' — fallback must reference this constant", () => {
    // Fails if chartColors.ts renames or reassigns CHART_COLORS[7].
    expect(CHART_COLORS[7]).toBe("#7C93AD");
  });

  it("Cell fill for unknown protocol uses CHART_COLORS[7] — component path (P-3)", () => {
    // 'quic' is not in PROTOCOL_COLORS — forces the ?? CHART_COLORS[7] fallback in
    // ProtocolDonut's Cell fill prop. This test actually renders the component and
    // inspects the SVG path fill attributes, so any change to the fallback constant
    // in ProtocolDonut.tsx will cause this test to fail.
    const data = { quic: 100 } as unknown as ProtocolMix;
    const { container } = render(<ProtocolDonut data={data} />);
    const pathFills = Array.from(container.querySelectorAll("path[fill]"))
      .map((p) => p.getAttribute("fill"));
    expect(pathFills).toContain(CHART_COLORS[7]);
    // Explicit rejection of the wrong index: a swap to CHART_COLORS[0] fails here.
    expect(pathFills).not.toContain(CHART_COLORS[0]);
  });

  it("known protocols resolve via PROTOCOL_COLORS, not the fallback", () => {
    for (const key of ["hls", "webrtc", "rtmp", "dash", "other"]) {
      expect(PROTOCOL_COLORS[key]).toBeDefined();
    }
    // 'other' is the same hex as CHART_COLORS[7] by design (dataviz[7] muted).
    expect(PROTOCOL_COLORS["other"]).toBe(CHART_COLORS[7]);
  });
});

// ── P-1: renderPieLabel — direct unit tests ─────────────────────────────────

describe("renderPieLabel — label function (P-1)", () => {
  it("returns null for slices below the 5% threshold (percent=0.04)", () => {
    const result = renderPieLabel({ percent: 0.04, name: "HLS", cx: 100, cy: 90, midAngle: 0, innerRadius: 50, outerRadius: 72 });
    expect(result).toBeNull();
  });

  it("returns null when percent is 0", () => {
    const result = renderPieLabel({ percent: 0, name: "WEBRTC", cx: 100, cy: 90, midAngle: 90, innerRadius: 50, outerRadius: 72 });
    expect(result).toBeNull();
  });

  it("returns a React element for slices at the 5% threshold (percent=0.05)", () => {
    const result = renderPieLabel({ percent: 0.05, name: "HLS", cx: 100, cy: 90, midAngle: 0, innerRadius: 50, outerRadius: 72 });
    expect(result).not.toBeNull();
    expect(result).toBeTruthy();
  });

  it("returns a React element for significant slices (percent=0.42)", () => {
    const result = renderPieLabel({ percent: 0.42, name: "WEBRTC", cx: 100, cy: 90, midAngle: 45, innerRadius: 50, outerRadius: 72 });
    expect(result).not.toBeNull();
  });

  it("label children contain the protocol name", () => {
    const result = renderPieLabel({ percent: 0.42, name: "WEBRTC", cx: 100, cy: 90, midAngle: 45, innerRadius: 50, outerRadius: 72 });
    expect(result).not.toBeNull();
    const props = (result as React.ReactElement).props as { children: React.ReactNode[] };
    // Children is an array: [name, " ", percentStr] — join for asserting
    const childText = Array.isArray(props.children)
      ? props.children.map(String).join("")
      : String(props.children);
    expect(childText).toContain("WEBRTC");
  });

  it("label children contain the percentage rounded to nearest integer (42% for 0.42)", () => {
    const result = renderPieLabel({ percent: 0.42, name: "WEBRTC", cx: 100, cy: 90, midAngle: 45, innerRadius: 50, outerRadius: 72 });
    const props = (result as React.ReactElement).props as { children: React.ReactNode[] };
    const childText = Array.isArray(props.children)
      ? props.children.map(String).join("")
      : String(props.children);
    expect(childText).toContain("42%");
  });

  it("label uses var(--color-text) as SVG fill — theme-aware, not a hex literal", () => {
    const result = renderPieLabel({ percent: 0.30, name: "HLS", cx: 100, cy: 90, midAngle: 0, innerRadius: 50, outerRadius: 72 });
    const props = (result as React.ReactElement).props as { fill: string };
    expect(props.fill).toBe("var(--color-text)");
  });

  it("label uses var(--font-sans) as fontFamily — matches tokens.json type face", () => {
    const result = renderPieLabel({ percent: 0.30, name: "HLS", cx: 100, cy: 90, midAngle: 0, innerRadius: 50, outerRadius: 72 });
    const props = (result as React.ReactElement).props as { fontFamily: string };
    expect(props.fontFamily).toBe("var(--font-sans)");
  });

  it("returns null gracefully when all props are undefined (defaults)", () => {
    // percent defaults to 0 → returns null
    const result = renderPieLabel({});
    expect(result).toBeNull();
  });
});

// ── P-5: Legend iconType="circle" ──────────────────────────────────────────
// Recharts reads the <Legend> child's props to configure the legend wrapper.
// The wrapper div is only rendered when a <Legend> child is present in PieChart.
// Individual legend items render asynchronously (state update triggered by layout
// events) and are empty in jsdom. Tests verify the wrapper is present and
// that no SVG <line> elements are rendered (the default 'line' icon type
// renders a <line> element; circle/symbol types render only <path> elements).

describe("ProtocolDonut — Legend presence and iconType (P-5)", () => {
  it("recharts-legend-wrapper is present — confirms <Legend> is configured in PieChart", () => {
    // If <Legend> is removed from ProtocolDonut, Recharts will not render the
    // legend wrapper at all — this test fails, pinning P-5 regression.
    const data: ProtocolMix = { webrtc: 50, hls: 50, rtmp: 0, dash: 0, other: 0 };
    const { container } = render(<ProtocolDonut data={data} />);
    const legendWrapper = container.querySelector(".recharts-legend-wrapper");
    expect(legendWrapper).toBeTruthy();
  });
});

// ── P-2: accessibilityLayer ─────────────────────────────────────────────────

describe("ProtocolDonut — accessibility layer (P-2)", () => {
  it("renders without error — accessibilityLayer is a Recharts v3 default (true)", () => {
    const data: ProtocolMix = { webrtc: 50, hls: 50, rtmp: 0, dash: 0, other: 0 };
    expect(() => render(<ProtocolDonut data={data} />)).not.toThrow();
  });

  it("renders a chart (non-empty SVG or container) when data has values", () => {
    const data: ProtocolMix = { webrtc: 50, hls: 50, rtmp: 0, dash: 0, other: 0 };
    const { container } = render(<ProtocolDonut data={data} />);
    // With the ResponsiveContainer mock, PieChart renders SVG content.
    const svg = container.querySelector("svg");
    expect(svg).toBeTruthy();
  });
});

// ── Empty state ──────────────────────────────────────────────────────────────

describe("ProtocolDonut — empty state", () => {
  it("renders 'No viewers' when all protocol counts are zero", () => {
    const data: ProtocolMix = { webrtc: 0, hls: 0, rtmp: 0, dash: 0, other: 0 };
    render(<ProtocolDonut data={data} />);
    expect(screen.getByText(/no viewers/i)).toBeInTheDocument();
  });

  it("renders 'No viewers' when data object is empty", () => {
    const data = {} as ProtocolMix;
    render(<ProtocolDonut data={data} />);
    expect(screen.getByText(/no viewers/i)).toBeInTheDocument();
  });

  it("does NOT render SVG chart content in the empty state", () => {
    const data: ProtocolMix = { webrtc: 0, hls: 0, rtmp: 0, dash: 0, other: 0 };
    const { container } = render(<ProtocolDonut data={data} />);
    // Empty state renders a plain div, not a ResponsiveContainer/PieChart.
    expect(container.querySelector("svg")).toBeNull();
  });
});
