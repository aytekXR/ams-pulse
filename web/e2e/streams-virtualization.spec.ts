/**
 * Streams virtualization specs.
 *
 * StreamsTable (web/src/features/live/StreamsTable.tsx) uses @tanstack/react-virtual
 * (ROW_HEIGHT 44, maxHeight 520, overscan 10). With 500 streams the virtualizer
 * renders only the visible window (~12 rows) + overscan (10 each side) ≈ 22–32 DOM rows.
 *
 * Assertions grounded in the actual component:
 *  - role="grid" aria-label="Active streams" aria-rowcount={streams.length + 1}
 *  - role="rowgroup" > role="row"  (virtual rows, count bounded ≤ 35)
 *  - footer text: "{N} streams"  (src/features/live/StreamsTable.tsx line 205)
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

/** Generate N streams with predictable IDs (1-indexed, zero-padded to 4 digits) */
function makeStreams(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    stream_id: `stream-${String(i + 1).padStart(4, "0")}`,
    app: "live",
    node_id: "node-1",
    viewers: i,
    publisher_state: "publishing",
    health_score: 100,
    bitrate_kbps: 2000,
  }));
}

test.describe("Streams virtualization", () => {
  test(
    "500 streams: bounded DOM rows, aria-rowcount 501, scroll to bottom, footer text",
    async ({ page }) => {
      const streams = makeStreams(500);

      await page.addInitScript(
        (key) => localStorage.setItem(key, "plt_e2e_virt_token"),
        TOKEN_KEY
      );

      await page.route("/api/v1/live/overview", (route) =>
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            ts: 1_700_000_000_000,
            total_viewers: 10_000,
            total_publishers: 500,
            nodes: [],
            protocol_mix: { webrtc: 500, hls: 0, rtmp: 0, dash: 0, other: 0 },
            apps: [],
          }),
        })
      );
      await page.route("/api/v1/live/streams**", (route) =>
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            items: streams,
            meta: { total: 500, has_more: false, next_cursor: null },
          }),
        })
      );

      await page.goto("/");

      // Wait for the virtualized grid to appear
      const grid = page.getByRole("grid", { name: "Active streams" });
      await expect(grid).toBeVisible();

      // aria-rowcount = 500 data rows + 1 header row = 501
      await expect(grid).toHaveAttribute("aria-rowcount", "501");

      // Only a bounded window of rows is in the DOM (visible ~12 + overscan ≤ 35 total)
      const renderedRows = grid.getByRole("rowgroup").getByRole("row");
      const renderedCount = await renderedRows.count();
      expect(
        renderedCount,
        `Expected ≤ 35 rendered rows (virtual window), got ${renderedCount}`
      ).toBeLessThanOrEqual(35);
      expect(renderedCount, "Expected at least 1 rendered row").toBeGreaterThan(0);

      // Footer shows the total count (StreamsTable.tsx line 205)
      await expect(page.getByText("500 streams")).toBeVisible();

      // Scroll to the bottom of the virtual container — virtualizer re-renders last rows
      await grid.evaluate((el) => {
        el.scrollTop = el.scrollHeight;
      });

      // Last stream becomes visible after virtualizer re-renders
      await expect(grid.getByText("stream-0500")).toBeVisible();
    }
  );
});
