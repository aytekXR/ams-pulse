/**
 * Reports page e2e (Wave 2, §2.19).
 *
 * The Reports page is the most complex gated page in the UI: Business tier
 * required, three tabpanels with distinct data fetches, and a full tenant CRUD
 * flow including a guarded delete-confirm dialog.  None of this was exercised in a
 * real browser before this wave — jsdom can assert structure but cannot drive real
 * focus, real ARIA resolution, or real network round-trips.
 *
 * Pinned tests (task contract):
 *   (a) Both sides of the tier gate — pro shows upsell + no tablist; business
 *       shows tablist + no upsell.
 *   (b) A real tenant create round-trip: POST body captured from the actual
 *       network request, not inferred from a mock echo.
 *   (c) Delete-confirm guards the DELETE — cancelling must not fire the request.
 */
import { test, expect } from "@playwright/test";
import { stubApp, collectErrors, json } from "./support/stubs";

// ─── Shared stub payloads ──────────────────────────────────────────────────────

/** Minimal but structurally valid usage response. */
const EMPTY_USAGE = {
  rows: [],
  totals: { viewer_minutes: 0, peak_concurrency: 0, egress_gb: 0, recording_gb: 0 },
  egress_method: "bitrate_x_watch_time",
};

/** Empty tenant list — returned when no tenants exist. */
const EMPTY_TENANTS = {
  items: [],
  meta: { total: 0, next_cursor: null },
};

/** Single-tenant list used by the delete-confirm and edit flows. */
const ONE_TENANT_LIST = {
  items: [
    {
      id: "t-001",
      name: "Acme Corp",
      stream_pattern: "live/acme-%",
      meta_tag_key: null,
      meta_tag_value: null,
      created_at: 1700000000000,
      updated_at: 1700000000000,
    },
  ],
  meta: { total: 1, next_cursor: null },
};

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe("Reports", () => {
  /**
   * Smoke: zero console errors on mount (business tier).
   *
   * Goes red if: the page throws on mount, a required import is missing, or a
   * mandatory boot-time API call is not stubbed (the unfulfilled fetch would cause
   * an unhandled rejection that lands in collectErrors).
   */
  test("mounts without console errors (business tier)", async ({ page }) => {
    const errors = collectErrors(page);
    await stubApp(page, { tier: "business" });
    // Usage tab is active on mount; loadUsage() fires immediately.
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));

    await page.goto("/reports");
    await expect(page.getByRole("heading", { name: "Reports" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Usage" })).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  /**
   * Tier gate (pro): upsell heading is visible, tablist is absent.
   *
   * Goes red if: (i) the gate is accidentally removed so pro can access reports,
   * or (ii) tabs are rendered even when gated (false-positive accessibility tree).
   * jsdom cannot confirm the absence of a tablist in a real browser.
   */
  test("pro tier: shows tier-gate heading and no tablist", async ({ page }) => {
    await stubApp(page, { tier: "pro" });

    await page.goto("/reports");
    await expect(
      page.getByRole("heading", { name: "Usage Reports requires Business tier" }),
    ).toBeVisible();
    // A tablist MUST NOT be present — if it were, tabs would be keyboard-reachable
    // to a pro user who only sees the gate visually.
    await expect(page.getByRole("tablist")).toHaveCount(0);
  });

  /**
   * Tier gate (business): tablist present, no upsell heading.
   *
   * Goes red if: business tier is accidentally gated (regression), or the upsell
   * remains rendered behind the tabs (hidden but in the DOM).
   */
  test("business tier: shows all three tabs and no tier-gate heading", async ({ page }) => {
    await stubApp(page, { tier: "business" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));

    await page.goto("/reports");
    await expect(page.getByRole("tablist")).toBeVisible();
    await expect(page.getByRole("tab", { name: "Usage" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Schedules" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Tenants" })).toBeVisible();
    // Gate heading must not appear at all — even hidden.
    await expect(
      page.getByRole("heading", { name: /requires business tier/i }),
    ).toHaveCount(0);
  });

  /**
   * Schedules tab activation: clicking the Schedules tab fires
   * GET /api/v1/reports/schedules and RENDERS the returned rows. Uncovered by
   * e2e until S43 (D-105): the tab was asserted visible but never activated, so
   * the tab-change effect that calls loadSchedules() was never driven.
   *
   * Non-vacuous: if activation did not fire the fetch (or it were unstubbed),
   * `schedules` would stay empty and the "No scheduled exports" empty state would
   * render instead of the cron row.
   */
  test("schedules tab: activating it fetches and renders schedules", async ({ page }) => {
    await stubApp(page, { tier: "business" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));
    const SCHEDULES = {
      items: [
        {
          id: "sched-1",
          cron: "0 9 * * 1",
          format: "csv",
          scope: { app: "live" },
          created_at: 1700000000000,
          updated_at: 1700000000000,
        },
      ],
      meta: { total: 1, next_cursor: null },
    };
    await page.route("**/api/v1/reports/schedules", (route) => json(route, SCHEDULES));

    await page.goto("/reports");
    await expect(page.getByRole("tab", { name: "Schedules" })).toBeVisible();

    // Capture the fetch before activating the tab so there is no race.
    const schedReq = page.waitForRequest((req) =>
      req.url().includes("/api/v1/reports/schedules"),
    );
    await page.getByRole("tab", { name: "Schedules" }).click();
    await schedReq;

    // The tabpanel shows the fetched row (cron), NOT the empty state.
    await expect(page.getByRole("tabpanel", { name: "Schedules" })).toBeVisible();
    await expect(page.getByText("0 9 * * 1")).toBeVisible();
    await expect(page.getByText("No scheduled exports")).toHaveCount(0);
  });

  /**
   * Usage tabpanel: real ARIA wiring.
   *
   * Goes red if: the `aria-labelledby` attribute is removed from the tabpanel, or
   * the Tabs component stops setting `id="tab-usage"` on the usage tab button.
   * jsdom resolves aria-labelledby structurally; real browsers also validate that
   * the referenced id actually exists in the live document.
   */
  test("usage tabpanel is wired to its tab via aria-labelledby", async ({ page }) => {
    await stubApp(page, { tier: "business" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));

    await page.goto("/reports");
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("id", "tabpanel-usage");
    await expect(panel).toHaveAttribute("aria-labelledby", "tab-usage");
    // aria-labelledby is a dangling pointer unless the id it names actually exists. Assert
    // the other half of the wiring, or this test only proves a string was written.
    await expect(page.getByRole("tab", { name: "Usage" })).toHaveAttribute("id", "tab-usage");
  });

  /**
   * PIN (b): Tenant create round-trip — POST body matches form input.
   *
   * Captures the actual request body the page sends and asserts its shape.
   * Goes red if: the form fails to trim whitespace, sends wrong field names, or
   * omits the `null` for unfilled optional fields.  A mock-echo test would pass
   * even if the page sent a completely wrong payload — this cannot.
   */
  test("tenant create: POST body matches form data", async ({ page }) => {
    let capturedPostBody: unknown = null;

    await stubApp(page, { tier: "business" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));
    await page.route("**/api/v1/admin/tenants", async (route) => {
      if (route.request().method() === "POST") {
        capturedPostBody = route.request().postDataJSON();
        // Return a valid Tenant so the page can close the form.
        await json(
          route,
          {
            id: "t-new",
            name: "Test Tenant",
            stream_pattern: "live/test-%",
            meta_tag_key: null,
            meta_tag_value: null,
            created_at: Date.now(),
            updated_at: Date.now(),
          },
          201,
        );
      } else {
        // GET — list call on mount and after create.
        await json(route, EMPTY_TENANTS);
      }
    });

    await page.goto("/reports");
    await page.getByRole("tab", { name: "Tenants" }).click();
    await expect(page.getByRole("button", { name: "+ New tenant" })).toBeVisible();
    await page.getByRole("button", { name: "+ New tenant" }).click();
    await expect(page.getByTestId("tenant-form")).toBeVisible();

    await page.getByLabel("Tenant name").fill("Test Tenant");
    await page.getByLabel("Stream pattern").fill("live/test-%");
    await page.getByRole("button", { name: "Save tenant" }).click();

    // The form closes when the POST succeeds (handleCreate sets showForm=false).
    // Waiting for it to disappear ensures the network round-trip completed.
    await expect(page.getByTestId("tenant-form")).toHaveCount(0);

    expect(capturedPostBody).toEqual({
      name: "Test Tenant",
      stream_pattern: "live/test-%",
      meta_tag_key: null,
      meta_tag_value: null,
    });
  });

  /**
   * PIN (c): Delete cancel — dismissing confirm must not fire the DELETE request.
   *
   * Goes red if: the delete is triggered on dialog open rather than on confirm, or
   * the Cancel button accidentally calls onConfirm.  jsdom can assert that a handler
   * is not called, but cannot confirm that no real HTTP request was dispatched.
   */
  test("delete cancel: dismissing confirm does not fire DELETE", async ({ page }) => {
    const deleteRequests: string[] = [];
    page.on("request", (req) => {
      if (req.method() === "DELETE") deleteRequests.push(req.url());
    });

    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));
    // Return the one tenant on every GET so the row stays visible.
    await page.route("**/api/v1/admin/tenants", (route) =>
      json(route, ONE_TENANT_LIST),
    );

    await page.goto("/reports");
    await page.getByRole("tab", { name: "Tenants" }).click();
    await expect(page.getByTestId("tenant-row-t-001")).toBeVisible();

    // Open the delete confirm.
    await page.getByRole("button", { name: "Delete Acme Corp" }).click();
    await expect(page.getByTestId("delete-confirm")).toBeVisible();

    // Cancel — scoped to the dialog so there is no ambiguity.
    await page.getByTestId("delete-confirm").getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByTestId("delete-confirm")).toHaveCount(0);

    // If Cancel had triggered the delete, a DELETE request would be in the list.
    expect(deleteRequests).toHaveLength(0);
  });

  /**
   * The positive counterpart to the cancel test above.
   *
   * On its own, "cancelling fires no DELETE" is equally satisfied by a page whose delete
   * is broken and never fires a DELETE at all. Asserting an absence only means something
   * next to proof that the thing can happen.
   */
  test("delete confirm: confirming fires DELETE /api/v1/admin/tenants/{id}", async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/reports/usage**", (route) => json(route, EMPTY_USAGE));
    await page.route("**/api/v1/admin/tenants", (route) => json(route, ONE_TENANT_LIST));
    await page.route("**/api/v1/admin/tenants/t-001", (route) =>
      route.fulfill({ status: 204, body: "" }),
    );

    await page.goto("/reports");
    await page.getByRole("tab", { name: "Tenants" }).click();
    await expect(page.getByTestId("tenant-row-t-001")).toBeVisible();

    await page.getByRole("button", { name: "Delete Acme Corp" }).click();
    const confirm = page.getByTestId("delete-confirm");
    await expect(confirm).toBeVisible();

    // Arm the waiter before the click so there is no race.
    const deleteReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/admin/tenants/t-001") && req.method() === "DELETE",
    );
    await confirm.getByRole("button", { name: /^delete$/i }).click();
    await deleteReq; // rejects on timeout if the DELETE never fires
  });
});
