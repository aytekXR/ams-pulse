/**
 * Anomalies page e2e (Wave 2, §2.19).
 *
 * AnomaliesPage has never been driven in a real browser. Pins the tier-gate
 * fork, the anomalies table structure, and the sigma threshold selector.
 * jsdom can assert that these components exist in the React tree; these tests
 * assert that they actually render in a real browser with the correct ARIA tree,
 * that the network re-fetch fires when the selector changes, and that the page
 * mounts without console errors.
 */
import { test, expect } from "@playwright/test";
import { stubApp, json, collectErrors } from "./support/stubs";

// ─── Fixture data ──────────────────────────────────────────────────────────────

const SAMPLE_FLAGS_BODY = {
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
    {
      id: "flag-2",
      metric: "error_rate",
      scope: { node_id: null, app: "live", stream_id: "stream/main" },
      observed: 0.15,
      expected: 0.01,
      sigma: 3.1,
      ts: Date.now() - 120_000,
    },
  ],
  meta: { total: 2 },
};

const EMPTY_FLAGS_BODY = { items: [], meta: { total: 0 } };

// ─── Tests ─────────────────────────────────────────────────────────────────────

test.describe("Anomalies", () => {
  test("mounts without console errors (enterprise, empty list)", async ({ page }) => {
    const errors = collectErrors(page);

    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/anomalies**", (route) => json(route, EMPTY_FLAGS_BODY));

    await page.goto("/anomalies");
    await expect(page.getByRole("heading", { name: "Anomaly Detection", level: 1 })).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("tier gate: pro tier shows TierGate upsell, table absent", async ({ page }) => {
    // No anomalies route needed — license check short-circuits before any fetch.
    await stubApp(page, { tier: "pro" });

    await page.goto("/anomalies");

    // The h1 is rendered in the gated path too (same heading, but gate renders beneath it).
    await expect(page.getByRole("heading", { name: "Anomaly Detection", level: 1 })).toBeVisible();
    // TierGate renders its own h2 with the feature-locked copy.
    await expect(
      page.getByRole("heading", {
        name: "Anomaly Detection requires Enterprise tier",
        level: 2,
      })
    ).toBeVisible();
    // The CTA link must be reachable by keyboard and AT.
    await expect(page.getByRole("link", { name: "Upgrade License" })).toBeVisible();
    // The table must be absent — the gate blocks it.
    await expect(page.getByRole("table", { name: "Anomaly flags table" })).toHaveCount(0);
  });

  test("enterprise tier: table renders, TierGate absent", async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/anomalies**", (route) => json(route, SAMPLE_FLAGS_BODY));

    await page.goto("/anomalies");

    // The anomaly flags table must be present and labelled.
    await expect(page.getByRole("table", { name: "Anomaly flags table" })).toBeVisible();
    // A metric from the stub must appear as a table cell.
    await expect(page.getByRole("cell", { name: "viewers" })).toBeVisible();
    // TierGate heading must not exist when enterprise.
    await expect(
      page.getByRole("heading", { name: "Anomaly Detection requires Enterprise tier" })
    ).toHaveCount(0);
  });

  test("sigma selector: changing value re-fetches with the new min_sigma AND renders the result", async ({
    page,
  }) => {
    await stubApp(page, { tier: "enterprise" });

    // Serve DIFFERENT data per sigma so the re-fetch is observable in the DOM.
    // Asserting only that the request fired would pass even if the page threw the
    // response away — the render assertion below is what closes that gap.
    await page.route("**/api/v1/anomalies**", (route) =>
      json(route, route.request().url().includes("min_sigma=3") ? SAMPLE_FLAGS_BODY : EMPTY_FLAGS_BODY),
    );

    await page.goto("/anomalies");
    await expect(page.getByRole("heading", { name: "Anomaly Detection", level: 1 })).toBeVisible();

    // At the default sigma (2) the stub returns nothing, so the empty state shows.
    await expect(page.getByText(/no anomalies detected/i)).toBeVisible();

    const select = page.getByLabel("Minimum sigma threshold");
    await expect(select).toHaveValue("2");

    // Arm the request waiter BEFORE the interaction that triggers the fetch.
    const sigma3Req = page.waitForRequest(
      (req) =>
        req.url().includes("/api/v1/anomalies") && req.url().includes("min_sigma=3"),
    );
    await select.selectOption("3");
    await sigma3Req; // rejects on timeout if the re-fetch never fires

    // ...and the response must actually reach the screen.
    await expect(page.getByRole("table", { name: "Anomaly flags table" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "viewers" })).toBeVisible();
  });

  test("empty state: enterprise with no anomalies shows copy, not the table", async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/anomalies**", (route) => json(route, EMPTY_FLAGS_BODY));

    await page.goto("/anomalies");

    await expect(page.getByText(/no anomalies detected/i)).toBeVisible();
    // "baselines still learning" is the clarifying sub-copy — assert it is present too.
    await expect(page.getByText(/baselines are still learning/i)).toBeVisible();
    // The table must NOT exist when the list is empty.
    await expect(page.getByRole("table", { name: "Anomaly flags table" })).toHaveCount(0);
  });
});
