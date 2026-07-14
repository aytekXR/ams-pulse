/**
 * Fleet page e2e (Wave 2, §2.19).
 *
 * No Fleet spec existed before this wave. Pins the SegmentedControl extraction in a
 * real browser: keyboard navigation and the radiogroup semantics, which are exactly
 * the parts a jsdom test asserts structurally but cannot exercise for real.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

const NODES_BODY = JSON.stringify({
  items: [
    {
      node_id: "node-origin-1",
      role: "origin",
      status: "up",
      last_seen: Date.now(),
      version: "2.9.1",
      cpu_pct: 45,
      mem_pct: 62,
      net_in_mbps: 12.5,
      net_out_mbps: 88.3,
    },
    {
      node_id: "node-edge-1",
      role: "edge",
      status: "degraded",
      last_seen: Date.now(),
      version: "2.9.0",
      cpu_pct: 85,
      mem_pct: 91,
    },
  ],
  meta: { total: 2 },
});

test.describe("Fleet", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((key) => localStorage.setItem(key, "plt_e2e_fleet"), TOKEN_KEY);

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
    await page.route("**/api/v1/fleet/nodes**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: NODES_BODY })
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

  test("renders node cards; zero console errors", async ({ page }) => {
    const errors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      if (msg.text().includes("WebSocket")) return;
      errors.push(msg.text());
    });
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/fleet");
    await expect(page.getByRole("heading", { name: "Fleet" })).toBeVisible();
    await expect(page.getByText("node-origin-1")).toBeVisible();
    await expect(page.getByText("node-edge-1")).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("view switch is a radiogroup, not a tablist", async ({ page }) => {
    await page.goto("/fleet");
    await expect(page.getByRole("heading", { name: "Fleet" })).toBeVisible();

    await expect(page.getByRole("radiogroup", { name: "Fleet view" })).toBeVisible();
    // A tablist would promise tabpanels that do not exist on this page.
    await expect(page.getByRole("tablist")).toHaveCount(0);
    await expect(page.getByRole("radio", { name: "Cards" })).toHaveAttribute("aria-checked", "true");
  });

  test("keyboard: ArrowRight on the segmented control switches to the table view", async ({ page }) => {
    await page.goto("/fleet");
    await expect(page.getByText("node-origin-1")).toBeVisible();

    // Roving tabIndex means the checked radio is the group's single tab stop.
    await page.getByRole("radio", { name: "Cards" }).focus();
    await page.keyboard.press("ArrowRight");

    await expect(page.getByRole("radio", { name: "Table" })).toHaveAttribute("aria-checked", "true");
    await expect(page.getByRole("columnheader", { name: "Node ID" })).toBeVisible();
    // Selection follows focus — the newly checked radio holds focus.
    await expect(page.getByRole("radio", { name: "Table" })).toBeFocused();
  });

  test("clicking Table renders the table view", async ({ page }) => {
    await page.goto("/fleet");
    await expect(page.getByText("node-origin-1")).toBeVisible();

    await page.getByRole("radio", { name: "Table" }).click();
    await expect(page.getByRole("columnheader", { name: "Node ID" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Memory" })).toBeVisible();
  });
});
