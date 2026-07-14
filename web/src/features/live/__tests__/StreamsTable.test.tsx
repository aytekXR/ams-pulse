/**
 * StreamsTable tests.
 *
 * Wave 0 pins (B2 sweep):
 *  - Virtualization: 500 rows must not create 500 DOM nodes.
 *  - Row height: default density = 40px (tokens.json tableRowHeight).
 *  - Header, empty state, footer count, stream data, bitrate formatting.
 *
 * Wave 1 (uipro) additions — ARIA grid structure (ST-1, ST-2, ST-3):
 *  - ST-1: role="grid" must be on the outermost container, not the scroll div.
 *  - ST-2: each header cell has role="columnheader"; Viewers + Bitrate have
 *          aria-sort="none" to signal they are sortable but not currently sorted.
 *  - ST-3: the stream-id cell has role="rowheader"; remaining data cells have
 *          role="gridcell".
 *  - Token: borderRadius uses var(--radius-control); padding uses var(--space-3).
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { StreamsTable } from "../StreamsTable";
import type { LiveStream } from "@/lib/api/types";
import { DensityProvider } from "@/lib/ThemeContext";
import type { ReactNode } from "react";

// DensityProvider wrapper — StreamsTable calls useDensity() for row height.
function wrapper({ children }: { children: ReactNode }) {
  return <DensityProvider>{children}</DensityProvider>;
}

// Mock @tanstack/react-virtual — same stub as Wave 0.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
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

// ── Wave 0 pins (unchanged) ──────────────────────────────────────────────────

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

    const dataRows = container.querySelectorAll('[role="row"][aria-rowindex]');
    const dataRowCount = Array.from(dataRows).filter(
      (el) => Number(el.getAttribute("aria-rowindex") ?? 0) > 1,
    ).length;

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
    const dataRows = container.querySelectorAll('[role="row"][aria-rowindex]');
    const dataRow = Array.from(dataRows).find(
      (el) => Number(el.getAttribute("aria-rowindex") ?? 0) > 1,
    );
    expect(dataRow).toBeDefined();
    expect((dataRow as HTMLElement).style.height).toBe("40px");
  });
});

// ── Wave 1: ST-1 — role="grid" on the outer container ────────────────────────

describe("StreamsTable — ARIA grid ownership (ST-1)", () => {
  it("outermost container has role='grid'", () => {
    const { container } = render(<StreamsTable streams={[]} />, { wrapper });
    // The outermost element (container.firstElementChild) must be the grid.
    const outerDiv = container.firstElementChild as HTMLElement;
    expect(outerDiv.getAttribute("role")).toBe("grid");
  });

  it("grid has aria-label='Active streams'", () => {
    const { container } = render(<StreamsTable streams={[]} />, { wrapper });
    const outerDiv = container.firstElementChild as HTMLElement;
    expect(outerDiv.getAttribute("aria-label")).toBe("Active streams");
  });

  it("grid aria-rowcount equals streams.length + 1 (header row included)", () => {
    const streams = Array.from({ length: 5 }, (_, i) => makeStream(i));
    const { container } = render(<StreamsTable streams={streams} />, { wrapper });
    const outerDiv = container.firstElementChild as HTMLElement;
    expect(outerDiv.getAttribute("aria-rowcount")).toBe("6");
  });

  it("header rowgroup is a direct child of the grid element", () => {
    const { container } = render(<StreamsTable streams={[]} />, { wrapper });
    const outerDiv = container.firstElementChild as HTMLElement;
    // First child of the grid must be role="rowgroup" (the header group).
    const firstChild = outerDiv.firstElementChild as HTMLElement;
    expect(firstChild.getAttribute("role")).toBe("rowgroup");
  });

  it("scroll viewport div does NOT carry role='grid' (grid is on the outer wrapper)", () => {
    const { container } = render(<StreamsTable streams={[]} />, { wrapper });
    // There should be exactly one element with role="grid" in the tree.
    const grids = container.querySelectorAll('[role="grid"]');
    expect(grids).toHaveLength(1);
    // And it must be the outermost container, not a nested div.
    expect(grids[0]).toBe(container.firstElementChild);
  });
});

// ── Wave 1: ST-2 — role="columnheader" + aria-sort ───────────────────────────

describe("StreamsTable — column headers (ST-2)", () => {
  it("renders exactly 7 columnheader cells", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    const columnHeaders = screen.getAllByRole("columnheader");
    expect(columnHeaders).toHaveLength(7);
  });

  it("all 7 column header labels are present", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    expect(screen.getByRole("columnheader", { name: "Stream" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "App" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Node" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "State" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Viewers" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Bitrate" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Health" })).toBeInTheDocument();
  });

  it("Viewers columnheader does NOT have aria-sort (no sort handler present)", () => {
    // aria-sort="none" must NOT appear until an onClick sort handler is wired up;
    // a false promise of interactivity misleads AT users (WAI-ARIA 1.2 §6.6).
    render(<StreamsTable streams={[]} />, { wrapper });
    const viewersHeader = screen.getByRole("columnheader", { name: "Viewers" });
    expect(viewersHeader.getAttribute("aria-sort")).toBeNull();
  });

  it("Bitrate columnheader does NOT have aria-sort (no sort handler present)", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    const bitrateHeader = screen.getByRole("columnheader", { name: "Bitrate" });
    expect(bitrateHeader.getAttribute("aria-sort")).toBeNull();
  });

  it("no column header has aria-sort until sort is implemented", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    for (const name of ["Stream", "App", "Node", "State", "Viewers", "Bitrate", "Health"]) {
      const header = screen.getByRole("columnheader", { name });
      expect(header.getAttribute("aria-sort")).toBeNull();
    }
  });
});

// ── Wave 1: ST-3 — role="rowheader" + role="gridcell" ────────────────────────

describe("StreamsTable — data cell roles (ST-3)", () => {
  it("first cell (stream_id) has role='rowheader'", () => {
    const streams = [makeStream(0)];
    render(<StreamsTable streams={streams} />, { wrapper });
    const rowHeaders = screen.getAllByRole("rowheader");
    // There is one rowheader per data row.
    expect(rowHeaders.length).toBeGreaterThanOrEqual(1);
    // The rowheader contains the stream_id.
    expect(rowHeaders[0].textContent).toContain("stream-0");
  });

  it("data rows have 6 gridcell children (one per non-identifying column)", () => {
    const streams = [makeStream(0)];
    const { container } = render(<StreamsTable streams={streams} />, { wrapper });
    // Select the data row (aria-rowindex=2 is the first data row).
    const dataRow = container.querySelector('[role="row"][aria-rowindex="2"]') as HTMLElement;
    expect(dataRow).toBeTruthy();
    const gridCells = within(dataRow).getAllByRole("gridcell");
    // 6 gridcell: app, node, state, viewers, bitrate, health (stream_id is rowheader)
    expect(gridCells).toHaveLength(6);
  });

  it("gridcell for state column contains a status badge", () => {
    const streams = [makeStream(0)];
    render(<StreamsTable streams={streams} />, { wrapper });
    // The State column renders a Badge with the publisher_state text.
    expect(screen.getByText("publishing")).toBeInTheDocument();
  });

  it("gridcell for health column contains a health badge", () => {
    const streams = [makeStream(0)];
    render(<StreamsTable streams={streams} />, { wrapper });
    expect(screen.getByText("good")).toBeInTheDocument();
  });
});

// ── Wave 1: token substitution pins ──────────────────────────────────────────

describe("StreamsTable — token substitution pins", () => {
  it("outer container borderRadius uses var(--radius-control)", () => {
    const { container } = render(<StreamsTable streams={[]} />, { wrapper });
    const outerDiv = container.firstElementChild as HTMLElement;
    const styleText = outerDiv.getAttribute("style") ?? outerDiv.style.cssText;
    expect(styleText).toContain("var(--radius-control)");
  });

  it("header cell padding uses var(--space-3) not a hardcoded 12px", () => {
    render(<StreamsTable streams={[]} />, { wrapper });
    const streamHeader = screen.getByRole("columnheader", { name: "Stream" });
    const styleText = streamHeader.getAttribute("style") ?? streamHeader.style.cssText;
    expect(styleText).toContain("var(--space-3)");
  });
});
