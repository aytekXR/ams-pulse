/**
 * chartColors — brandkit dataviz palette unit tests
 *
 * Verifies the palette constants export the correct brandkit values.
 * These are literal hex tests — if someone accidentally changes a hex
 * value, the test catches the regression before it ships.
 */
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { ThemeProvider } from "./ThemeContext";
import {
  CHART_COLORS,
  STATUS_COLORS,
  PROTOCOL_COLORS,
  LIGHT_STATUS_COLORS,
  useStatusColors,
} from "./chartColors";

describe("CHART_COLORS", () => {
  it("exports an 8-element readonly array", () => {
    expect(CHART_COLORS).toHaveLength(8);
  });

  it("index 0 is signal/healthy #2CE5A7", () => {
    expect(CHART_COLORS[0]).toBe("#2CE5A7");
  });

  it("index 1 is primary series #58A6FF", () => {
    expect(CHART_COLORS[1]).toBe("#58A6FF");
  });

  it("index 2 is secondary series #A78BFA", () => {
    expect(CHART_COLORS[2]).toBe("#A78BFA");
  });

  it("index 4 is warning/fifth #FFB224", () => {
    expect(CHART_COLORS[4]).toBe("#FFB224");
  });

  it("index 7 is muted series #7C93AD", () => {
    expect(CHART_COLORS[7]).toBe("#7C93AD");
  });
});

describe("STATUS_COLORS", () => {
  it("healthy is signal green #2CE5A7", () => {
    expect(STATUS_COLORS.healthy).toBe("#2CE5A7");
  });

  it("warning is amber #FFB224", () => {
    expect(STATUS_COLORS.warning).toBe("#FFB224");
  });

  it("critical is red #FF5C68", () => {
    expect(STATUS_COLORS.critical).toBe("#FF5C68");
  });

  it("neutral is muted blue-grey #8296A8", () => {
    expect(STATUS_COLORS.neutral).toBe("#8296A8");
  });
});

describe("PROTOCOL_COLORS", () => {
  it("hls is signal green (healthy)", () => {
    expect(PROTOCOL_COLORS.hls).toBe("#2CE5A7");
  });

  it("webrtc is blue dataviz[1]", () => {
    expect(PROTOCOL_COLORS.webrtc).toBe("#58A6FF");
  });

  it("rtmp is purple dataviz[2] — not critical red", () => {
    expect(PROTOCOL_COLORS.rtmp).toBe("#A78BFA");
    // Must NOT be critical red (would imply error state for healthy RTMP)
    expect(PROTOCOL_COLORS.rtmp).not.toBe("#FF5C68");
  });

  it("dash is pink dataviz[3]", () => {
    expect(PROTOCOL_COLORS.dash).toBe("#F06BB2");
  });

  it("other is muted dataviz[7]", () => {
    expect(PROTOCOL_COLORS.other).toBe("#7C93AD");
  });
});

// ─── Light palette pins — tokens.json color.light ────────────────────────────

describe("LIGHT_STATUS_COLORS", () => {
  it("healthy is accessible green #0BA678 (tokens.json color.light.healthy)", () => {
    expect(LIGHT_STATUS_COLORS.healthy).toBe("#0BA678");
  });

  it("warning is accessible amber #B45309 (tokens.json color.light.warning)", () => {
    expect(LIGHT_STATUS_COLORS.warning).toBe("#B45309");
  });

  it("critical is accessible red #DC2626 (tokens.json color.light.critical)", () => {
    expect(LIGHT_STATUS_COLORS.critical).toBe("#DC2626");
  });

  it("neutral is accessible grey #64748B (tokens.json color.light.neutral)", () => {
    expect(LIGHT_STATUS_COLORS.neutral).toBe("#64748B");
  });
});

// ─── useStatusColors hook — returns correct set per theme ────────────────────

describe("useStatusColors hook", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("data-theme");
    localStorage.clear();
  });

  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
  });

  it("returns dark STATUS_COLORS when theme is dark", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const { result } = renderHook(() => useStatusColors(), {
      wrapper: ThemeProvider,
    });
    expect(result.current.healthy).toBe("#2CE5A7");
    expect(result.current.warning).toBe("#FFB224");
    expect(result.current.critical).toBe("#FF5C68");
    expect(result.current.neutral).toBe("#8296A8");
  });

  it("returns LIGHT_STATUS_COLORS when theme is light", () => {
    document.documentElement.setAttribute("data-theme", "light");
    const { result } = renderHook(() => useStatusColors(), {
      wrapper: ThemeProvider,
    });
    expect(result.current.healthy).toBe("#0BA678");
    expect(result.current.warning).toBe("#B45309");
    expect(result.current.critical).toBe("#DC2626");
    expect(result.current.neutral).toBe("#64748B");
  });
});
