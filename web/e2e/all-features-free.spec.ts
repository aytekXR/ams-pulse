/**
 * D-194 (v0.5.0): Pulse is free — every feature, for everyone, commercial use
 * included. A keyless v0.5.0 server reports tier "free" with
 * `all_features_free: true`, and every page that used to put an upgrade wall in
 * front of Free must open in a real browser.
 *
 * Each page's data route is stubbed with real content, so the assertion is "the
 * feature's own UI is visible AND no upgrade prompt is" — absence of a prompt
 * alone would also pass on a blank page.
 */
import { test, expect } from "@playwright/test";
import { stubApp, collectErrors, json } from "./support/stubs";

const PROBES_BODY = {
  items: [
    {
      id: "probe-1",
      name: "Main HLS stream",
      url: "https://example.com/live/main.m3u8",
      protocol: "hls",
      interval_s: 60,
      timeout_s: 10,
      enabled: true,
      created_at: Date.now() - 86_400_000,
    },
  ],
  meta: { total: 1 },
};

const FLAGS_BODY = {
  items: [
    {
      id: "flag-1",
      metric: "viewers",
      scope: { node_id: "node-1", app: "live", stream_id: null },
      observed: 150,
      expected: 50,
      sigma: 4.5,
      ts: Date.now() - 60_000,
    },
  ],
  meta: { total: 1 },
};

const EMPTY_USAGE = {
  rows: [],
  totals: { viewer_minutes: 0, peak_concurrency: 0, egress_gb: 0, recording_gb: 0 },
  egress_method: "bitrate_x_watch_time",
};

test.describe("D-194: every feature is free on a keyless server", () => {
  test.beforeEach(async ({ page }) => {
    await stubApp(page, { tier: "free", allFeaturesFree: true });
  });

  test("synthetic probes open with no upgrade prompt", async ({ page }) => {
    const errors = collectErrors(page);
    await page.route(/\/api\/v1\/probes\?/, (route) => json(route, PROBES_BODY));
    await page.goto("/probes");
    await expect(page.getByRole("table", { name: "Synthetic probes list" })).toBeVisible();
    await expect(page.getByRole("link", { name: /upgrade license/i })).toHaveCount(0);
    expect(errors).toEqual([]);
  });

  test("anomaly detection opens with no upgrade prompt", async ({ page }) => {
    const errors = collectErrors(page);
    await page.route("**/api/v1/anomalies**", (route) => json(route, FLAGS_BODY));
    await page.goto("/anomalies");
    await expect(page.getByRole("table", { name: "Anomaly flags table" })).toBeVisible();
    await expect(page.getByRole("link", { name: /upgrade license/i })).toHaveCount(0);
    expect(errors).toEqual([]);
  });

  test("usage reports open with every tab and no upgrade prompt", async ({ page }) => {
    const errors = collectErrors(page);
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));
    await page.goto("/reports");
    await expect(page.getByRole("tab", { name: "Usage" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Schedules" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Tenants" })).toBeVisible();
    await expect(page.getByRole("heading", { name: /requires business tier/i })).toHaveCount(0);
    expect(errors).toEqual([]);
  });

  test("the License tab says every feature is free and offers no key form", async ({ page }) => {
    await page.route("**/api/v1/admin/sources", (route) => json(route, { items: [] }));
    await page.route("**/api/v1/admin/tokens", (route) => json(route, { items: [] }));
    await page.goto("/settings");
    await page.getByRole("tab", { name: "License" }).click();
    await expect(page.getByText(/every feature is included/i)).toBeVisible();
    await expect(page.getByPlaceholder(/PULSE-XXXX/i)).toHaveCount(0);
    await expect(page.getByText(/contact sales/i)).toHaveCount(0);
    await expect(page.getByText("null", { exact: true })).toHaveCount(0);
  });
});
