/**
 * Analytics page e2e (Wave 2, §2.19).
 *
 * There was no Analytics spec before this wave. S32's standing rule: run the specs
 * of the components a wave TOUCHES, not just the default gate set — that rule exists
 * because streams-virtualization.spec (not in the default set) caught a real
 * regression the default four would have missed. Wave 2 touches Analytics, so
 * Analytics gets a spec.
 *
 * Pins the real-browser behaviour the jsdom tests cannot: the Recharts chart
 * actually paints its three series in the brandkit dataviz colours, and the tab
 * bar drives a real tabpanel.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

const AUDIENCE_BODY = JSON.stringify({
  totals: { views: 1234, uniques: 567, watch_time_s: 7200, peak_concurrency: 89 },
  timeseries: [
    { ts: 1_700_000_000_000, views: 600, uniques: 300, watch_time_s: 3600, peak_concurrency: 45 },
    { ts: 1_700_086_400_000, views: 634, uniques: 267, watch_time_s: 3600, peak_concurrency: 44 },
  ],
});

const GEO_BODY = JSON.stringify({
  rows: [{ country: "TR", views: 900, uniques: 400, watch_time_s: 5400 }],
});

const DEVICE_BODY = JSON.stringify({ rows: [] });

test.describe("Analytics", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((key) => localStorage.setItem(key, "plt_e2e_analytics"), TOKEN_KEY);

    await page.route("/api/v1/admin/license", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          tier: "pro",
          valid: true,
          expires_at: null,
          offline_file: false,
          limits: {
            max_nodes: 10,
            max_streams: null,
            retention_days: 90,
            data_api: true,
            white_label: false,
          },
        }),
      })
    );
    await page.route("**/api/v1/analytics/audience**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: AUDIENCE_BODY })
    );
    await page.route("**/api/v1/analytics/geo**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: GEO_BODY })
    );
    await page.route("**/api/v1/analytics/devices**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: DEVICE_BODY })
    );
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "e2e", role: "admin", auth_method: "token" }),
      })
    );
    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: false }) })
    );
  });

  test("renders totals, chart and tabs; zero console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      if (msg.text().includes("WebSocket")) return;
      errors.push(msg.text());
    });
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/analytics");
    await expect(page.getByRole("heading", { name: "Analytics" })).toBeVisible();

    // StatCards adopted in Wave 2 — accessible names the inline cards never had.
    await expect(page.getByRole("group", { name: "Total Views: 1,234" })).toBeVisible();
    await expect(page.getByRole("group", { name: "Peak Concurrency: 89" })).toBeVisible();

    // The audience panel is a real tabpanel wired to its tab.
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("id", "panel-audience");
    await expect(panel).toHaveAttribute("aria-labelledby", "tab-audience");

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("chart paints the three series in the brandkit dataviz colours", async ({ page }) => {
    await page.goto("/analytics");
    await expect(page.getByRole("heading", { name: "Analytics" })).toBeVisible();

    // Recharts only lays out with real dimensions — this is exactly what jsdom
    // cannot check, and why the unit test can only read the source.
    const lines = page.locator(".recharts-line-curve");
    await expect(lines).toHaveCount(3);

    const strokes = await lines.evaluateAll((els) =>
      els.map((el) => (el as SVGElement).getAttribute("stroke"))
    );
    // CHART_COLORS[1] views, [0] uniques, [4] peak — the SAME hex as before the
    // refactor. A wrong index here is a silent colour change.
    expect(strokes).toEqual(["#58A6FF", "#2CE5A7", "#FFB224"]);
  });

  test("tab switch moves the tabpanel to geo", async ({ page }) => {
    await page.goto("/analytics");
    await expect(page.getByRole("heading", { name: "Analytics" })).toBeVisible();

    await page.getByRole("tab", { name: "Geo" }).click();
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("id", "panel-geo");
    await expect(page.getByRole("columnheader", { name: "Country" })).toBeVisible();
  });
});
