/**
 * Settings page e2e (Wave 2, §2.19).
 *
 * SettingsPage is the highest-value spec of the six wave-1 pages because it
 * exercises the keyboard-trap that existed in the earlier hand-rolled tab bar:
 * role="tab" + a roving tabIndex but NO arrow-key handler — meaning five of the
 * six tabs were literally unreachable by keyboard. The page now uses the shared
 * <Tabs wrap> component, which provides Arrow/Home/End navigation.
 *
 * This spec pins that navigation in a real browser, where jsdom cannot exercise:
 *   - Real focus management (tabIndex=-1 vs 0 in the live DOM)
 *   - Real keyboard events dispatched through the browser's focus model
 *   - Real ARIA tree (role="tablist", role="tab", aria-selected, aria-labelledby)
 *   - Real panel visibility toggling driven by tab selection
 *
 * Network: SettingsPage.loadAll() fires three parallel requests on mount:
 *   GET /api/v1/admin/sources — AMS source list (handled by beforeEach stub)
 *   GET /api/v1/admin/tokens  — API + ingest token list (handled by beforeEach stub)
 *   GET /api/v1/admin/license — licence info (handled by stubApp)
 * GET /admin/users is NOT called by the current implementation; the Users tab
 * renders a hardcoded placeholder and needs no network round-trip.
 */
import { test, expect } from "@playwright/test";
import { stubApp, json, collectErrors } from "./support/stubs";

const SOURCES_BODY = { items: [] };
const TOKENS_BODY = { items: [] };

test.describe("Settings", () => {
  test.beforeEach(async ({ page }) => {
    // stubApp installs /auth/me, /auth/oidc/status, and /api/v1/admin/license.
    // SettingsPage.loadAll() also calls getLicense() — the same route — so the
    // single stub serves both the LicenseProvider boot call and the page call.
    await stubApp(page, { tier: "enterprise" });
    await page.route("**/api/v1/admin/sources", (route) => json(route, SOURCES_BODY));
    await page.route("**/api/v1/admin/tokens", (route) => json(route, TOKENS_BODY));
  });

  // ── 1. Smoke ───────────────────────────────────────────────────────────────

  test("mounts without console errors; heading and all six tabs visible", async ({ page }) => {
    const errors = collectErrors(page);

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

    // All six tabs must appear in the accessibility tree; if any were missing the
    // getByRole call would throw and the test would go red.
    await expect(page.getByRole("tab", { name: "Sources" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "API Tokens" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Ingest Tokens" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Integrations" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "License" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Users" })).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  // ── 2. Keyboard walk — the test that would have caught the old trap ─────────

  test("keyboard: ArrowRight walks all six tabs in order, wraps, End/Home jump", async ({ page }) => {
    await page.goto("/settings");

    // The tabpanel only appears once loadAll() resolves; wait for it so that
    // every subsequent panel-id check is against fully rendered content.
    await expect(page.getByRole("tabpanel")).toBeVisible();

    const tabNames = [
      "Sources",
      "API Tokens",
      "Ingest Tokens",
      "Integrations",
      "License",
      "Users",
    ] as const;
    const panelIds = [
      "settings-panel-sources",
      "settings-panel-tokens",
      "settings-panel-ingest",
      "settings-panel-integrations",
      "settings-panel-license",
      "settings-panel-users",
    ] as const;

    // Focus the first tab — roving-tabIndex gives it tabIndex=0, so it is the
    // entry point for keyboard users.
    await page.getByRole("tab", { name: "Sources" }).focus();
    await expect(page.getByRole("tab", { name: "Sources" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "Sources" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-sources");

    // Walk tabs 1-5 via ArrowRight.
    // Each assertion below goes red if ANY of these fail:
    //   - ArrowRight does not move focus (keyboard trap)
    //   - aria-selected is not updated on the newly focused tab
    //   - the panel does not switch to match the selected tab
    for (let i = 1; i < tabNames.length; i++) {
      await page.keyboard.press("ArrowRight");
      await expect(page.getByRole("tab", { name: tabNames[i] })).toBeFocused();
      await expect(page.getByRole("tab", { name: tabNames[i] })).toHaveAttribute("aria-selected", "true");
      await expect(page.getByRole("tabpanel")).toHaveAttribute("id", panelIds[i]);
    }

    // ArrowRight from the last tab must wrap back to the first.
    await page.keyboard.press("ArrowRight");
    await expect(page.getByRole("tab", { name: "Sources" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "Sources" })).toHaveAttribute("aria-selected", "true");

    // End: jump directly to the last tab.
    await page.keyboard.press("End");
    await expect(page.getByRole("tab", { name: "Users" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "Users" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-users");

    // Home: jump back to the first tab.
    await page.keyboard.press("Home");
    await expect(page.getByRole("tab", { name: "Sources" })).toBeFocused();
    await expect(page.getByRole("tab", { name: "Sources" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-sources");
  });

  // ── 3. Panel ARIA wiring ────────────────────────────────────────────────────

  test("tabpanel id and aria-labelledby update when a different tab is clicked", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("tabpanel")).toBeVisible();

    // Initial state: sources panel wired to its tab button.
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-sources");
    await expect(page.getByRole("tabpanel")).toHaveAttribute("aria-labelledby", "tab-sources");

    // Click Integrations: the panel must flip to integrations and the aria-labelledby
    // must reference the Integrations tab button id.
    await page.getByRole("tab", { name: "Integrations" }).click();
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-integrations");
    await expect(page.getByRole("tabpanel")).toHaveAttribute("aria-labelledby", "tab-integrations");
    // The element referenced by aria-labelledby must actually exist in the DOM.
    await expect(page.locator("#tab-integrations")).toBeVisible();
  });

  // ── 4. License tab content ──────────────────────────────────────────────────

  test("license tab: shows current tier from stub and activate form is disabled without a key", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("tabpanel")).toBeVisible();

    await page.getByRole("tab", { name: "License" }).click();
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-license");

    // The license card shows the tier returned by the stub. Asserting only the static
    // "Current license" label would pass even if the tier Badge were deleted — so assert
    // the stubbed tier value itself reached the DOM.
    await expect(page.getByText("Current license")).toBeVisible();
    await expect(
      page.getByRole("tabpanel").getByText("enterprise", { exact: true }),
    ).toBeVisible();

    // Activate form: submit is disabled until a non-empty key is typed.
    // Goes red if the disabled guard is removed from the button.
    const activateBtn = page.getByRole("button", { name: "Activate" });
    await expect(activateBtn).toBeDisabled();

    // Typing a key enables the button.
    await page.getByPlaceholder("PULSE-XXXX-XXXX-XXXX").fill("PULSE-TEST-1234");
    await expect(activateBtn).toBeEnabled();
  });

  // ── 5. Integrations tab content ─────────────────────────────────────────────

  test("integrations tab: Prometheus heading and S3 save button visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("tabpanel")).toBeVisible();

    await page.getByRole("tab", { name: "Integrations" }).click();
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "settings-panel-integrations");

    // Prometheus section heading — goes red if the section is removed or renamed.
    await expect(page.getByRole("heading", { name: "Prometheus Metrics" })).toBeVisible();

    // S3 section heading and form submit — goes red if the S3 section is removed.
    await expect(page.getByRole("heading", { name: "S3 Export Destination" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Save S3 config" })).toBeVisible();
  });
});
