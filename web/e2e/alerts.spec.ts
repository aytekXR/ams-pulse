/**
 * Alerts page e2e (S34 Wave).
 *
 * AlertsPage was rewritten in Wave 4 with:
 *   - a shared <Tabs> component emitting real ARIA tabs (role="tablist" /
 *     role="tab" / role="tabpanel") with full keyboard navigation;
 *   - an inline delete-confirmation step replacing window.confirm;
 *   - a de-duplicated error announcement model: the sr-only aria-live mirror
 *     that caused every validation error to be announced TWICE was removed.
 *     The inline role="alert" span IS now the only live region.
 *
 * These are exactly the things jsdom unit tests assert structurally but cannot
 * exercise in a real browser — focus management, real computed ARIA tree, and
 * the absence of a duplicate DOM node that a unit test cannot distinguish from
 * a CSS-hidden element.
 */
import { test, expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { stubApp, json, collectErrors } from "./support/stubs";

// ── fixtures ────────────────────────────────────────────────────────────────

const RULE_FIXTURE = {
  id: "rule-1",
  name: "High CPU Alert",
  metric: "cpu_pct",
  operator: "gt",
  threshold: 80,
  window_s: 300,
  severity: "warning",
  cooldown_s: 300,
  enabled: true,
  muted: false,
  created_at: 1_700_000_000_000,
  updated_at: 1_700_000_000_000,
  rule_type: "threshold",
  sigma: 4.0,
  min_samples: 30,
};

/**
 * Install the three alerts API routes the page fetches in parallel on mount.
 * Call BEFORE page.goto so the routes are registered before the first request.
 */
async function stubAlertRoutes(
  page: Page,
  opts: { rules?: unknown[]; channels?: unknown[]; history?: unknown[] } = {},
): Promise<void> {
  const { rules = [], channels = [], history = [] } = opts;
  await page.route("**/api/v1/alerts/rules**", (route) =>
    json(route, { items: rules }),
  );
  await page.route("**/api/v1/alerts/channels**", (route) =>
    json(route, { items: channels }),
  );
  await page.route("**/api/v1/alerts/history**", (route) =>
    json(route, { items: history }),
  );
}

// ── tests ────────────────────────────────────────────────────────────────────

test.describe("Alerts", () => {
  test.beforeEach(async ({ page }) => {
    // Boot layer only — API data is registered per-test so each test controls it.
    await stubApp(page);
  });

  test("mounts without console errors; rules tabpanel ARIA is wired", async ({ page }) => {
    const errors = collectErrors(page);
    await stubAlertRoutes(page); // all empty — tests the boot path, not the data
    await page.goto("/alerts");

    await expect(page.getByRole("heading", { name: "Alerts" })).toBeVisible();

    // The Rules panel is active by default. Asserting its id and aria-labelledby
    // validates the end-to-end ARIA wiring between the <Tabs> component (which
    // emits id="tab-rules" on the button) and the tabpanel in AlertsPage.
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("id", "panel-rules");
    await expect(panel).toHaveAttribute("aria-labelledby", "tab-rules");

    // Empty state renders without crashing — proves the page mounts cleanly
    // on the first real-browser render, not just in jsdom.
    await expect(page.getByText("No alert rules")).toBeVisible();

    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });

  test("keyboard: ArrowRight on Rules tab activates Channels and moves focus", async ({ page }) => {
    await stubAlertRoutes(page);
    await page.goto("/alerts");
    await expect(page.getByRole("heading", { name: "Alerts" })).toBeVisible();

    // Roving tabIndex: the active (Rules) tab is the only tab stop in the group.
    // ArrowRight must activate the next tab AND imperatively move focus — the two
    // things jsdom cannot test together (state update + DOM focus in the same tick).
    await page.getByRole("tab", { name: "Rules" }).focus();
    await page.keyboard.press("ArrowRight");

    await expect(page.getByRole("tab", { name: "Channels" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    // Focus must follow selection — the new tab holds keyboard focus.
    await expect(page.getByRole("tab", { name: "Channels" })).toBeFocused();

    // AlertsPage swaps the panel on tab change; the new panel must carry the
    // correct ids so AT can announce "Channels tab panel".
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "panel-channels");
    await expect(page.getByRole("tabpanel")).toHaveAttribute(
      "aria-labelledby",
      "tab-channels",
    );
  });

  test("keyboard: End jumps to History tab; Home wraps back to Rules", async ({ page }) => {
    await stubAlertRoutes(page);
    await page.goto("/alerts");
    await expect(page.getByRole("heading", { name: "Alerts" })).toBeVisible();

    await page.getByRole("tab", { name: "Rules" }).focus();

    // End → last tab (History, index 2)
    await page.keyboard.press("End");
    await expect(page.getByRole("tab", { name: "History" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(page.getByRole("tab", { name: "History" })).toBeFocused();
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "panel-history");

    // Home → first tab (Rules, index 0)
    await page.keyboard.press("Home");
    await expect(page.getByRole("tab", { name: "Rules" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(page.getByRole("tab", { name: "Rules" })).toBeFocused();
    await expect(page.getByRole("tabpanel")).toHaveAttribute("id", "panel-rules");
  });

  test("validation error appears exactly once — no sr-only aria-live mirror", async ({ page }) => {
    await stubAlertRoutes(page);
    await page.goto("/alerts");
    await expect(page.getByRole("heading", { name: "Alerts" })).toBeVisible();

    // Open the new rule form.
    await page.getByRole("button", { name: "+ New rule" }).click();
    await expect(page.getByRole("heading", { name: "New alert rule" })).toBeVisible();

    // Submit with both name and threshold empty — triggers the name error.
    await page.getByRole("button", { name: "Save rule" }).click();

    // The inline error span carries role="alert" so AT announces it.
    await expect(
      page.locator('[role="alert"]:has-text("Name is required")'),
    ).toBeVisible();

    // Exactly one DOM node carries this text. In the pre-fix code, an sr-only
    // aria-live div mirrored every error message, making the count 2. If that
    // mirror is re-introduced, this assertion catches it.
    await expect(
      page.locator('[role="alert"]:has-text("Name is required")'),
    ).toHaveCount(1);

    // No aria-live region ANYWHERE may repeat this message. Scoping this to `form [aria-live]`
    // (an earlier draft) would have missed a mirror re-introduced just outside the <form> tag —
    // which is exactly where a well-meaning refactor would put it. Filtering by the message text
    // instead of by position keeps the app's legitimate Toast live region out of the count while
    // still catching the duplicate wherever it lands.
    await expect(
      page.locator("[aria-live]").filter({ hasText: "Name is required" }),
    ).toHaveCount(0);
  });

  test("Delete rule shows inline confirm; Cancel dismisses without deleting", async ({ page }) => {
    // Stub rules with one real entry so the Delete button is rendered.
    await stubAlertRoutes(page, { rules: [RULE_FIXTURE] });
    await page.goto("/alerts");

    // Wait for the rule to render — confirms the stub was consumed.
    await expect(page.getByText("High CPU Alert")).toBeVisible();

    // Click the Delete button for the rule.
    await page.getByRole("button", { name: "Delete" }).click();

    // The inline confirmation replaces window.confirm. If the page still used
    // window.confirm, the native dialog would block and the testId would not appear.
    const confirm = page.getByTestId("delete-rule-confirm");
    await expect(confirm).toBeVisible();
    await expect(confirm.getByText(/cannot be undone/i)).toBeVisible();

    // Cancel must dismiss the prompt without removing the rule from the list.
    await confirm.getByRole("button", { name: "Cancel" }).click();
    await expect(confirm).not.toBeVisible();

    // The rule is still in the DOM — no premature deletion.
    await expect(page.getByText("High CPU Alert")).toBeVisible();
  });

  /**
   * The positive counterpart to the Cancel test above.
   *
   * Without this, "Cancel does not fire a DELETE" is satisfied just as well by a page
   * whose delete is broken and NEVER fires a DELETE. Asserting the absence of an effect
   * is only meaningful alongside proof that the effect can happen at all.
   */
  test("Delete rule: confirming fires DELETE /api/v1/alerts/rules/{id}", async ({ page }) => {
    await stubAlertRoutes(page, { rules: [RULE_FIXTURE] });
    await page.route("**/api/v1/alerts/rules/rule-1", (route) =>
      route.fulfill({ status: 204, body: "" }),
    );

    await page.goto("/alerts");
    await expect(page.getByText("High CPU Alert")).toBeVisible();

    await page.getByRole("button", { name: "Delete" }).click();
    await expect(page.getByTestId("delete-rule-confirm")).toBeVisible();

    // Arm the waiter before the click so there is no race.
    const deleteReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/alerts/rules/rule-1") && req.method() === "DELETE",
    );
    await page.getByRole("button", { name: "Yes, delete" }).click();
    await deleteReq; // rejects on timeout if the DELETE never fires
  });

  /**
   * Channel deletion used to call the native window.confirm(). Wave 4 gave RULES an inline
   * confirmation step and left CHANNELS on the native dialog — two confirmation models for
   * the same destructive verb. Found by this e2e pass and fixed in S34.
   *
   * This test would have been impossible to write against the old code without a Playwright
   * dialog handler, and it is exactly why the gap survived: jsdom stubs window.confirm, so
   * the unit tests never saw a dialog at all.
   */
  test("Delete channel: inline confirm (not window.confirm); Cancel fires no DELETE", async ({
    page,
  }) => {
    const deletes: string[] = [];
    page.on("request", (req) => {
      if (req.method() === "DELETE") deletes.push(req.url());
    });

    // If the page ever regresses to window.confirm, this handler proves it: Playwright
    // auto-dismisses native dialogs, so we record any that appear and assert none did.
    const nativeDialogs: string[] = [];
    page.on("dialog", (d) => {
      nativeDialogs.push(d.message());
      void d.dismiss();
    });

    await stubAlertRoutes(page, {
      channels: [{ id: "ch-1", name: "Ops Slack", type: "slack", config: {}, enabled: true }],
    });
    await page.goto("/alerts");

    await page.getByRole("tab", { name: "Channels" }).click();
    await expect(page.getByText("Ops Slack")).toBeVisible();

    await page.getByRole("button", { name: "Delete channel Ops Slack" }).click();

    // The inline confirm must appear...
    const confirmBox = page.getByTestId("delete-channel-confirm");
    await expect(confirmBox).toBeVisible();
    // ...and NO native dialog may have been raised.
    expect(nativeDialogs, `window.confirm() was called: ${nativeDialogs.join(", ")}`).toEqual([]);

    // Cancel dismisses without deleting; the channel survives.
    await confirmBox.getByRole("button", { name: "Cancel" }).click();
    await expect(confirmBox).toHaveCount(0);
    await expect(page.getByText("Ops Slack")).toBeVisible();
    expect(deletes).toEqual([]);
  });

  test("Delete channel: confirming fires DELETE /api/v1/alerts/channels/{id}", async ({ page }) => {
    await stubAlertRoutes(page, {
      channels: [{ id: "ch-1", name: "Ops Slack", type: "slack", config: {}, enabled: true }],
    });
    await page.route("**/api/v1/alerts/channels/ch-1", (route) =>
      route.fulfill({ status: 204, body: "" }),
    );

    await page.goto("/alerts");
    await page.getByRole("tab", { name: "Channels" }).click();
    await expect(page.getByText("Ops Slack")).toBeVisible();

    await page.getByRole("button", { name: "Delete channel Ops Slack" }).click();
    await expect(page.getByTestId("delete-channel-confirm")).toBeVisible();

    const deleteReq = page.waitForRequest(
      (req) =>
        req.url().includes("/api/v1/alerts/channels/ch-1") && req.method() === "DELETE",
    );
    await page.getByRole("button", { name: "Yes, delete" }).click();
    await deleteReq;
  });
});
