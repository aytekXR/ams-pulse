/**
 * Ingest Health page e2e (Wave 3, §2.19).
 *
 * IngestPage (/ingest) has never been driven in a real browser. This spec pins
 * the things jsdom unit tests structurally cannot check:
 *
 *   - real mount without console errors (the failure mode most likely to reach prod)
 *   - empty state vs populated state in a real DOM tree
 *   - aria-hidden on the health-bar element (Badge provides the text equivalent)
 *   - aria-label on the Close button inside the detail panel
 *   - Recharts writes stroke as an SVG presentation attribute only during real layout
 *   - packet-loss stroke is THEME-AWARE: #FF5C68 in dark, #DC2626 in light
 *     (the whole point of the Wave 3 fix — it used to hardcode the dark red in both)
 */
import { test, expect } from "@playwright/test";
import { stubApp, json, collectErrors } from "./support/stubs";

const INGEST_ROUTE = "**/api/v1/qoe/ingest**";

const STREAM_WITH_TIMESERIES = {
  stream_id: "live/e2e-stream",
  app: "live",
  node_id: "node-1",
  health_score: 95,
  timeseries: [
    { ts: Date.now() - 60_000, bitrate_kbps: 3200, fps: 30, packet_loss_pct: 0.5, jitter_ms: 5 },
    { ts: Date.now(), bitrate_kbps: 3400, fps: 29, packet_loss_pct: 0.3, jitter_ms: 4 },
  ],
  drop_events: [],
};

test.describe("Ingest Health", () => {
  test.beforeEach(async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
  });

  test("mount smoke: h1 visible and zero console errors", async ({ page }) => {
    const errors = collectErrors(page);
    await page.route(INGEST_ROUTE, (route) => json(route, { streams: [] }));

    await page.goto("/ingest");
    await expect(page.getByRole("heading", { name: "Ingest Health" })).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("empty state: 'No active publishers' message; publisher table absent", async ({ page }) => {
    await page.route(INGEST_ROUTE, (route) => json(route, { streams: [] }));

    await page.goto("/ingest");
    // The empty state must appear — removing it would flip this test
    await expect(page.getByText(/no active publishers/i)).toBeVisible();
    // The table must NOT appear — rendering a table with no rows would flip this
    await expect(page.getByRole("columnheader", { name: "Stream" })).toHaveCount(0);
  });

  test("health bar is aria-hidden (Badge carries the accessible label)", async ({ page }) => {
    await page.route(INGEST_ROUTE, (route) =>
      json(route, { streams: [STREAM_WITH_TIMESERIES] })
    );

    await page.goto("/ingest");
    await expect(page.getByText("live/e2e-stream")).toBeVisible();

    // data-testid="health-bar-bg" is on the outer bar container.
    // aria-hidden="true" is required: the Badge next to it says "Healthy" / "Degraded" / "Poor"
    // so the visual bar is decorative. Removing the attribute would flip this test.
    const bar = page.locator('[data-testid="health-bar-bg"]').first();
    await expect(bar).toHaveAttribute("aria-hidden", "true");
  });

  test("detail panel: Close button is accessible and dismisses the panel", async ({ page }) => {
    await page.route(INGEST_ROUTE, (route) =>
      json(route, { streams: [STREAM_WITH_TIMESERIES] })
    );

    await page.goto("/ingest");
    await expect(page.getByText("live/e2e-stream")).toBeVisible();

    // Open the detail panel
    await page.getByRole("button", { name: /details/i }).click();

    // The close button must be findable by its accessible name, not just its text.
    // Removing or changing the aria-label would flip this.
    const closeBtn = page.getByRole("button", { name: /close stream detail/i });
    await expect(closeBtn).toBeVisible();

    // Clicking Close must dismiss the detail panel — the button disappears with it
    await closeBtn.click();
    await expect(closeBtn).not.toBeVisible();
  });

  test("chart strokes — Bitrate #58A6FF, FPS #2CE5A7, PacketLoss #FF5C68, Jitter #FFB224 (dark)", async ({ page }) => {
    // Pin dark the SAME way the light test pins light — via the localStorage key that
    // resolveTheme() reads first. An earlier draft used emulateMedia({colorScheme:'dark'}),
    // which looked like it was pinning the theme but was doing nothing: resolveTheme() only
    // consults matchMedia for '(prefers-color-scheme: light)' and otherwise falls through to
    // a hardcoded 'dark'. The test passed because dark is the default, not because it asked
    // for dark. Setting the key exercises the real branch and keeps the two theme tests
    // symmetric.
    await page.addInitScript(
      ([k, v]) => localStorage.setItem(k, v),
      ["pulse_theme", "dark"] as const,
    );
    await page.route(INGEST_ROUTE, (route) =>
      json(route, { streams: [STREAM_WITH_TIMESERIES] })
    );

    await page.goto("/ingest");
    await expect(page.getByText("live/e2e-stream")).toBeVisible();

    // Open the detail panel — charts only render inside StreamDetail
    await page.getByRole("button", { name: /details/i }).click();

    // Recharts writes the stroke as an SVG presentation attribute only during
    // real browser layout. jsdom never provides this — it is exactly why the
    // unit tests read the source instead.
    const lines = page.locator(".recharts-line-curve");
    await expect(lines.first()).toBeVisible();
    await expect(lines).toHaveCount(4); // Bitrate, FPS (chart 1) + PacketLoss, Jitter (chart 2)

    const strokes = await lines.evaluateAll((els) =>
      els.map((el) => (el as SVGElement).getAttribute("stroke"))
    );

    // Chart 1 — Bitrate & FPS
    //   Bitrate: CHART_COLORS[1] = #58A6FF
    //   FPS:     CHART_COLORS[0] = #2CE5A7
    // Chart 2 — Packet Loss & Jitter
    //   PacketLoss: statusColors.critical (dark) = #FF5C68  ← theme-aware
    //   Jitter:     CHART_COLORS[4] = #FFB224
    expect(strokes).toEqual(["#58A6FF", "#2CE5A7", "#FF5C68", "#FFB224"]);
  });

  test("packet-loss stroke is theme-aware: #DC2626 in light theme (not the dark-hardcoded #FF5C68)", async ({ page }) => {
    // Seed light theme before the page loads so ThemeContext reads it from localStorage
    await page.addInitScript(
      ([k, v]) => localStorage.setItem(k, v),
      ["pulse_theme", "light"] as const,
    );
    await page.route(INGEST_ROUTE, (route) =>
      json(route, { streams: [STREAM_WITH_TIMESERIES] })
    );

    await page.goto("/ingest");
    await expect(page.getByText("live/e2e-stream")).toBeVisible();

    await page.getByRole("button", { name: /details/i }).click();

    const lines = page.locator(".recharts-line-curve");
    await expect(lines.first()).toBeVisible();
    await expect(lines).toHaveCount(4);

    const strokes = await lines.evaluateAll((els) =>
      els.map((el) => (el as SVGElement).getAttribute("stroke"))
    );

    // PacketLoss routes through useStatusColors().critical.
    // In light theme that is LIGHT_STATUS_COLORS.critical = #DC2626.
    // The bug this pins: before the Wave 3 fix the stroke was hardcoded to
    // STATUS_COLORS.critical (#FF5C68) in BOTH themes, so this would return
    // #FF5C68 and the assertion below would fail.
    expect(strokes[2]).toBe("#DC2626");
    // Static palette strokes are unchanged by theme
    expect(strokes[0]).toBe("#58A6FF"); // Bitrate CHART_COLORS[1]
    expect(strokes[1]).toBe("#2CE5A7"); // FPS CHART_COLORS[0]
    expect(strokes[3]).toBe("#FFB224"); // Jitter CHART_COLORS[4]
  });
});
