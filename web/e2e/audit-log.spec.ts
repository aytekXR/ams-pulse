/**
 * Audit Log page e2e (D-102 Phase 2).
 *
 * Drives the new /audit-log page in a real browser: table render, empty state,
 * and the cursor "Load more" pagination that appends the next page. jsdom unit
 * tests assert the React tree; these assert the real ARIA tree + that the
 * cursor request actually fires and its result reaches the screen.
 */
import { test, expect } from "@playwright/test";
import { stubApp, json, collectErrors } from "./support/stubs";

const PAGE1 = {
  items: [
    {
      id: "a1",
      ts: Date.now() - 60_000,
      actor_name: "admin-token",
      actor_token_id: "tok-abc12345",
      actor_user_id: "",
      action: "alert_rule.create",
      object_type: "alert_rule",
      object_id: "rule-1",
      remote_addr: "10.0.0.5",
      detail: { name: "cpu" },
    },
  ],
  meta: { next_cursor: "cur-1" },
};

const PAGE2 = {
  items: [
    {
      id: "a2",
      ts: Date.now() - 120_000,
      actor_name: "",
      actor_token_id: "tok-def67890",
      action: "user.delete",
      object_type: "user",
      object_id: "user-9",
      remote_addr: "10.0.0.6",
    },
  ],
  meta: { next_cursor: null },
};

const EMPTY = { items: [], meta: { next_cursor: null } };

test.describe("Audit Log", () => {
  test("mounts without console errors (empty list)", async ({ page }) => {
    const errors = collectErrors(page);

    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/admin/audit-log**", (route) => json(route, EMPTY));

    await page.goto("/audit-log");
    await expect(page.getByRole("heading", { name: "Audit Log", level: 1 })).toBeVisible();
    await expect(page.getByText(/no audit entries yet/i)).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("renders the entries table with the action and object", async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/admin/audit-log**", (route) => json(route, PAGE2));

    await page.goto("/audit-log");

    await expect(page.getByRole("table", { name: /audit log/i })).toBeVisible();
    await expect(page.getByRole("cell", { name: "user.delete" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "user-9" })).toBeVisible();
  });

  test("Load more: fires the cursor request and appends the next page", async ({ page }) => {
    await stubApp(page, { tier: "enterprise" });
    // Serve page 2 only when the cursor is present, page 1 otherwise.
    await page.route("**/api/v1/admin/audit-log**", (route) =>
      json(route, route.request().url().includes("cursor=") ? PAGE2 : PAGE1),
    );

    await page.goto("/audit-log");
    await expect(page.getByRole("cell", { name: "alert_rule.create" })).toBeVisible();

    const loadMore = page.getByRole("button", { name: /load more/i });
    await expect(loadMore).toBeVisible();

    const cursorReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/admin/audit-log") && req.url().includes("cursor="),
    );
    await loadMore.click();
    await cursorReq; // rejects on timeout if the cursor fetch never fires

    // Both pages now on screen; the button is gone (next_cursor is null).
    await expect(page.getByRole("cell", { name: "user.delete" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "alert_rule.create" })).toBeVisible();
    await expect(page.getByRole("button", { name: /load more/i })).toHaveCount(0);
  });
});
