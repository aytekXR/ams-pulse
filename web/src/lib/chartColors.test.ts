/**
 * chartColors — brandkit dataviz palette unit tests
 *
 * Verifies the palette constants export the correct brandkit values.
 * These are literal hex tests — if someone accidentally changes a hex
 * value, the test catches the regression before it ships.
 */
import { describe, it, expect } from "vitest";
import { CHART_COLORS, STATUS_COLORS, PROTOCOL_COLORS } from "./chartColors";

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
