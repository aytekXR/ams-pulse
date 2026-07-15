/**
 * ProbesPage e2e (Wave 2, §2.19).
 *
 * ProbesPage gates the OTHER way from every other gated page: Free is
 * blocked, Pro+ is open. This spec is the first time the page has been
 * driven in a real browser. Pins:
 *   (a) both sides of the tier gate (free = TierGate, pro = probe list)
 *   (b) delete dialog — appears as role="dialog", Cancel closes without
 *       firing DELETE, confirming fires the DELETE request
 *   (c) form validation — invalid URL (bypasses native HTML5 constraint
 *       validation, exercises the React validateProbeForm path) surfaces
 *       exactly one role="alert" with the URL error message
 *
 * NOTE on native HTML5 validation vs React validation:
 *   The form fields carry `required` / `min` attributes, so Chromium fires
 *   native constraint validation on submit-button-click before the submit
 *   event fires. Tests that rely on React's validateProbeForm must use inputs
 *   that pass native validation but fail React's — the invalid-URL case is
 *   ideal: `type="text"` (no native URL check) + non-empty (passes `required`)
 *   → native passes, React fails. The empty-name / interval-<30 cases would
 *   hit native validation first and NEVER reach the React handler in a real
 *   browser; those paths are covered by the unit tests that submit the form
 *   element directly (bypassing native), so they are not duplicated here.
 */
import { test, expect } from "@playwright/test";
import { stubApp, collectErrors, json } from "./support/stubs";

// ─── Shared fixtures ──────────────────────────────────────────────────────────

const now = Date.now();

const PROBE_1 = {
  id: "probe-1",
  name: "Main HLS stream",
  url: "https://example.com/live/main.m3u8",
  protocol: "hls",
  interval_s: 60,
  timeout_s: 10,
  enabled: true,
  created_at: now - 86_400_000,
  last_result: {
    id: "result-1",
    probe_id: "probe-1",
    ts: now - 60_000,
    success: true,
    ttfb_ms: 150,
    bitrate_kbps: 2500,
  },
};

const PROBES_BODY = { items: [PROBE_1], meta: { total: 1 } };

// ─── Tests ────────────────────────────────────────────────────────────────────

test.describe("ProbesPage", () => {
  // ── (a) Tier gate — free side ────────────────────────────────────────────

  test("free tier: shows TierGate upsell, probe list is absent", async ({ page }) => {
    await stubApp(page, { tier: "free" });
    // The probes route IS stubbed even though the gated page never calls it. That is
    // deliberate: without it, deleting the gate entirely would leave the fetch unstubbed,
    // no probes would load, no table would render — and "table has count 0" would still
    // pass. The assertion would be measuring the absent stub, not the gate. With data
    // available to render, the table's absence can only be the gate's doing.
    await page.route(/\/api\/v1\/probes\?/, (route) => json(route, PROBES_BODY));
    await page.goto("/probes");

    // The TierGate heading must be present.
    await expect(
      page.getByRole("heading", { name: /synthetic probes requires pro tier/i }),
    ).toBeVisible();

    // The probe data table must NOT exist — gate is not cosmetic.
    await expect(page.getByRole("table", { name: "Synthetic probes list" })).toHaveCount(0);
  });

  // ── (a) Tier gate — pro side ─────────────────────────────────────────────
  // ── (b) Smoke test ───────────────────────────────────────────────────────

  test.describe("pro tier (open)", () => {
    test.beforeEach(async ({ page }) => {
      await stubApp(page, { tier: "pro" });
      // ProbesPage calls probesApi.list({ limit: 100 }) → GET /api/v1/probes?limit=100
      // after the license resolves as non-free.
      await page.route(/\/api\/v1\/probes\?/, (route) => json(route, PROBES_BODY));
    });

    test("mounts with zero console errors; notice and probe list are visible", async ({ page }) => {
      const errors = collectErrors(page);
      await page.goto("/probes");

      // Pro tier must show the page heading, not the upsell.
      await expect(page.getByRole("heading", { name: "Synthetic Probes" })).toBeVisible();

      // Synthetic notice (role="note") — PRD F10 acceptance: results are
      // always labeled as synthetic and never silently mixed with organic data.
      await expect(page.getByRole("note", { name: "Synthetic probes notice" })).toBeVisible();

      // Probe list table rendered with an accessible label.
      await expect(page.getByRole("table", { name: "Synthetic probes list" })).toBeVisible();
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
    });

    // ── (b) Delete dialog — Cancel closes without DELETE ─────────────────

    test("delete dialog: Cancel closes dialog without firing DELETE", async ({ page }) => {
      let deleteHit = false;
      await page.route(/\/api\/v1\/probes\/probe-1/, (route) => {
        if (route.request().method() === "DELETE") {
          deleteHit = true;
          return route.fulfill({ status: 204, body: "" });
        }
        return route.continue();
      });

      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      // Open the delete confirmation.
      await page.getByRole("button", { name: "Delete probe Main HLS stream" }).click();
      const dialog = page.getByRole("dialog", { name: "Confirm probe deletion" });
      await expect(dialog).toBeVisible();

      // Cancel must dismiss the dialog.
      await page.getByRole("button", { name: /^cancel$/i }).click();
      await expect(dialog).toHaveCount(0);

      // Dismissing must NOT have triggered the DELETE request.
      expect(deleteHit, "DELETE was fired after Cancel — should not happen").toBe(false);
    });

    // ── (b) Delete dialog — confirm fires DELETE ──────────────────────────

    test("delete dialog: confirming fires DELETE /api/v1/probes/{id}", async ({ page }) => {
      await page.route(/\/api\/v1\/probes\/probe-1/, (route) => {
        if (route.request().method() === "DELETE") {
          return route.fulfill({ status: 204, body: "" });
        }
        return route.continue();
      });

      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      await page.getByRole("button", { name: "Delete probe Main HLS stream" }).click();
      await expect(page.getByRole("dialog", { name: "Confirm probe deletion" })).toBeVisible();

      // Set up the capture BEFORE clicking so there is no race.
      const deleteReq = page.waitForRequest(
        (req) =>
          req.url().includes("/api/v1/probes/probe-1") && req.method() === "DELETE",
      );

      // The confirm button inside the dialog is labeled "Delete" (not the per-row
      // aria-label, which includes the probe name).
      await page.getByRole("button", { name: /^delete$/i }).click();

      // waitForRequest rejects if the request is never fired within the timeout — THAT is
      // the assertion. (An `expect(fired.method()).toBe("DELETE")` here would be circular:
      // waitForRequest only resolves on a DELETE, so it is true by construction.)
      await deleteReq;

      // The row must actually leave the table — proof the response was handled, not just sent.
      await expect(page.getByText("Main HLS stream")).toHaveCount(0);
    });

    // ── (b) Delete dialog — a role="dialog" must behave like one ─────────
    //
    // Both of these went RED against the code as shipped and drove the S34 fix in
    // ProbesPage.tsx. A role="dialog" promises AT that focus moves into it and that
    // Escape gets you out; the component made neither promise good, and jsdom could
    // not see it because focus and key dispatch are browser behaviour.

    test("delete dialog: opening moves focus into the dialog", async ({ page }) => {
      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      const trigger = page.getByRole("button", { name: "Delete probe Main HLS stream" });
      await trigger.click();

      const dialog = page.getByRole("dialog", { name: "Confirm probe deletion" });
      await expect(dialog).toBeVisible();

      // Before the fix, focus stayed on the row's Delete button and a screen-reader
      // user was never told the dialog had appeared.
      await expect(dialog).toBeFocused();
    });

    test("delete dialog: Escape closes it, fires no DELETE, and returns focus to the trigger", async ({
      page,
    }) => {
      let deleteHit = false;
      await page.route(/\/api\/v1\/probes\/probe-1/, (route) => {
        if (route.request().method() === "DELETE") {
          deleteHit = true;
          return route.fulfill({ status: 204, body: "" });
        }
        return route.continue();
      });

      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      const trigger = page.getByRole("button", { name: "Delete probe Main HLS stream" });
      await trigger.click();

      const dialog = page.getByRole("dialog", { name: "Confirm probe deletion" });
      await expect(dialog).toBeVisible();

      await page.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);

      // Escape is a CANCEL, not a confirm — it must never delete.
      expect(deleteHit, "Escape fired the DELETE — it must only cancel").toBe(false);

      // Focus must come back to what opened the dialog, or a keyboard user is
      // stranded at the top of the document.
      await expect(trigger).toBeFocused();
    });

    // ── (c) Create — happy-path fires POST and appends the new probe ──────
    //
    // The (d) test drives only the invalid-URL React-validation path. This pins
    // the SUCCESS path end-to-end: a valid submit fires POST /api/v1/probes and
    // the returned probe is appended to the list and the form closes. Uncovered
    // by e2e until S43 (D-105) — only validation and delete were driven before.
    test("create happy-path: valid submit POSTs and appends the new probe", async ({ page }) => {
      const CREATED = {
        id: "probe-new",
        name: "New Origin Probe",
        url: "https://example.com/live/new.m3u8",
        protocol: "hls",
        interval_s: 60,
        timeout_s: 10,
        enabled: true,
        created_at: now,
        last_result: null,
      };
      // Query-less /probes matches ONLY the POST; the beforeEach GET stub uses
      // /probes?limit=100, so the two routes never collide.
      await page.route(/\/api\/v1\/probes$/, (route) => {
        if (route.request().method() === "POST") return json(route, CREATED, 201);
        return route.fallback();
      });

      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();
      // The new probe must not be present before the create.
      await expect(page.getByText("New Origin Probe")).toHaveCount(0);

      await page.getByRole("button", { name: "+ New Probe" }).click();
      await expect(page.getByRole("form", { name: "Create probe form" })).toBeVisible();

      await page.getByLabel("Name").fill("New Origin Probe");
      await page.getByLabel("Stream URL").fill("https://example.com/live/new.m3u8");

      // Capture the POST before clicking so there is no race.
      const postReq = page.waitForRequest(
        (req) => req.url().includes("/api/v1/probes") && req.method() === "POST",
      );
      await page.getByRole("button", { name: "Create Probe" }).click();
      await postReq;

      // Proof the response was HANDLED, not just sent: the returned probe is in
      // the list and the create form has closed.
      await expect(page.getByText("New Origin Probe")).toBeVisible();
      await expect(page.getByRole("form", { name: "Create probe form" })).toHaveCount(0);
    });

    // ── (d) Form validation — one role=alert per bad field ───────────────
    //
    // Uses the invalid-URL case: name="Test Probe" (passes native `required`),
    // url="not-a-url" (passes native `required` because non-empty; input is
    // type="text" so no native URL check), interval=60 (passes native `min=30`).
    // Native validation lets the submit event through; React's validateProbeForm
    // then rejects the URL and renders exactly one role="alert".

    test("form validation: invalid URL surfaces exactly one role=alert", async ({ page }) => {
      await page.goto("/probes");
      await expect(page.getByText("Main HLS stream")).toBeVisible();

      await page.getByRole("button", { name: "+ New Probe" }).click();
      await expect(page.getByRole("form", { name: "Create probe form" })).toBeVisible();

      await page.getByLabel("Name").fill("Test Probe");
      // "not-a-url" is non-empty (passes `required`) but fails `new URL()`.
      await page.getByLabel("Stream URL").fill("not-a-url");
      // Interval stays at the default 60 s — passes `min=30` natively.

      await page.getByRole("button", { name: "Create Probe" }).click();

      // Exactly one alert must appear — not zero (no error shown) and not two
      // (duplicate rendering of the same error).
      const alerts = page.getByRole("alert");
      await expect(alerts).toHaveCount(1);
      await expect(alerts.first()).toContainText(/valid url/i);
    });
  });
});
