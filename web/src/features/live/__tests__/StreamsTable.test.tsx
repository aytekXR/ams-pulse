/**
 * StreamsTable virtualization tests — assert row virtualization, not 500 DOM nodes.
 * Budget: 500 rows must not create 500 DOM row elements.
 *
 * Density-aware row height (B2 sweep): default density = 40px (tokens.json tableRowHeight).
 * The 44px legacy constant was a phase-1 divergence — fixed here.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { StreamsTable } from "../StreamsTable";
import type { LiveStream } from "@/lib/api/types";
import { DensityProvider } from "@/lib/ThemeContext";
import type { ReactNode } from "react";

// DensityProvider wrapper — StreamsTable calls useDensity() for row height.
// Default density from jsdom (no data-density attr) → "default" → rowHeight = 40.
function wrapper({ children }: { children: ReactNode }) {
  return <DensityProvider>{children}</DensityProvider>;
}

// Mock @tanstack/react-virtual to use a simplified version for tests.
// size=40 mirrors the default density row height (was 44 — phase-1 divergence fixed).
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      // Only virtualize the first 20 rows regardless of total count
      Array.from({ length: Math.min(count, 20) }, (_, i) => ({
        index: i,
        start: i * 40,
        size: 40,
        key: i,
        lane: 0,
        end: (i + 1) * 40,
      })),
    getTotalSize: () => count * 40,
  }),
}));

function makeStream(i: number): LiveStream {
  return {
    stream_id: `stream-${i}`,
    app: "live",
    node_id: `node-${i % 4}`,
    publisher_state: "publishing",
    viewers: i * 10,
    bitrate_kbps: 2000 + i,
    health_score: 95,
  };
}

describe("StreamsTable", () => {
  it("renders the header row", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    expect(screen.getByText("Stream")).toBeInTheDocument();
    expect(screen.getByText("Viewers")).toBeInTheDocument();
    expect(screen.getByText("Health")).toBeInTheDocument();
  });

  it("shows empty state when no streams", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    expect(screen.getByText(/no active streams/i)).toBeInTheDocument();
  });

  it("shows stream count footer", () => {
    const streams = Array.from({ length: 5 }, (_, i) => makeStream(i));
    render(<StreamsTable streams={streams} />, { wrapper });
    expect(screen.getByText(/5 streams/i)).toBeInTheDocument();
  });

  it("virtualizes 500 rows — does NOT render 500 DOM row elements", () => {
    const streams = Array.from({ length: 500 }, (_, i) => makeStream(i));
    const { container } = render(<StreamsTable streams={streams} />, { wrapper });

    // The virtualized list should show ≤20 rows (our mock), not 500
    const dataRows = container.querySelectorAll('[role="row"][aria-rowindex]');
    // Count only data rows (rowindex > 1)
    const dataRowCount = Array.from(dataRows).filter(
      (el) => Number(el.getAttribute("aria-rowindex") ?? 0) > 1,
    ).length;

    // Must not have 500 rendered rows
    expect(dataRowCount).toBeLessThan(100);
    expect(dataRowCount).toBeLessThanOrEqual(20);
  });

  it("displays stream data correctly", () => {
    const streams = [makeStream(0)];
    render(<StreamsTable streams={streams} />, { wrapper });
    expect(screen.getByText("stream-0")).toBeInTheDocument();
    expect(screen.getByText("publishing")).toBeInTheDocument();
  });

  it("formats bitrate as Mbps when >= 1000", () => {
    const streams: LiveStream[] = [{
      stream_id: "s1",
      app: "live",
      node_id: "n1",
      publisher_state: "publishing",
      viewers: 10,
      bitrate_kbps: 2500,
      health_score: 95,
    }];
    render(<StreamsTable streams={streams} />, { wrapper });
    expect(screen.getByText("2.5 Mbps")).toBeInTheDocument();
  });

  it("formats bitrate in Kbps when < 1000", () => {
    const streams: LiveStream[] = [{
      stream_id: "s2",
      app: "live",
      node_id: "n1",
      publisher_state: "publishing",
      viewers: 1,
      bitrate_kbps: 500,
      health_score: 95,
    }];
    render(<StreamsTable streams={streams} />, { wrapper });
    expect(screen.getByText("500 Kbps")).toBeInTheDocument();
  });

  it("row height uses default density (40px, not 44px — phase-1 fix)", () => {
    const streams = [makeStream(0)];
    const { container } = render(<StreamsTable streams={streams} />, { wrapper });
    // The data row div has an inline height style set to the density rowHeight.
    const dataRows = container.querySelectorAll('[role="row"][aria-rowindex]');
    const dataRow = Array.from(dataRows).find(
      (el) => Number(el.getAttribute("aria-rowindex") ?? 0) > 1,
    );
    expect(dataRow).toBeDefined();
    // Default density rowHeight = 40 (tokens.json tableRowHeight).
    expect((dataRow as HTMLElement).style.height).toBe("40px");
  });
});
