/**
 * StreamsTable virtualization tests — assert row virtualization, not 500 DOM nodes.
 * Budget: 500 rows must not create 500 DOM row elements.
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { StreamsTable } from "../StreamsTable";
import type { LiveStream } from "@/lib/api/types";

// Mock @tanstack/react-virtual to use a simplified version for tests
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      // Only virtualize the first 20 rows regardless of total count
      Array.from({ length: Math.min(count, 20) }, (_, i) => ({
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
    render(<StreamsTable streams={[]} />);
    expect(screen.getByText("Stream")).toBeInTheDocument();
    expect(screen.getByText("Viewers")).toBeInTheDocument();
    expect(screen.getByText("Health")).toBeInTheDocument();
  });

  it("shows empty state when no streams", () => {
    render(<StreamsTable streams={[]} />);
    expect(screen.getByText(/no active streams/i)).toBeInTheDocument();
  });

  it("shows stream count footer", () => {
    const streams = Array.from({ length: 5 }, (_, i) => makeStream(i));
    render(<StreamsTable streams={streams} />);
    expect(screen.getByText(/5 streams/i)).toBeInTheDocument();
  });

  it("virtualizes 500 rows — does NOT render 500 DOM row elements", () => {
    const streams = Array.from({ length: 500 }, (_, i) => makeStream(i));
    const { container } = render(<StreamsTable streams={streams} />);

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
    render(<StreamsTable streams={streams} />);
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
    render(<StreamsTable streams={streams} />);
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
    render(<StreamsTable streams={streams} />);
    expect(screen.getByText("500 Kbps")).toBeInTheDocument();
  });
});
