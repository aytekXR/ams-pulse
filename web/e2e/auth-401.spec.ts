/**
 * Auth 401 mid-session spec.
 *
 * When the API client (web/src/api/client.ts) receives an HTTP 401 it dispatches
 * window.dispatchEvent(new Event("pulse:auth:401")). AuthGate listens for this
 * event and responds by clearing the stored token and showing the gate again
 * with the message "Session expired or token revoked. Please enter your token again."
 *
 * This test dispatches the event directly (the same mechanism apiFetch uses) to
 * verify the full AuthGate listener → clearToken → re-render chain.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

test.describe("Auth 401 mid-session", () => {
  test("pulse:auth:401 event clears token and shows gate", async ({ page }) => {
    // Establish a valid session
    await page.addInitScript(
      (key) => localStorage.setItem(key, "plt_e2e_midauth_token"),
      TOKEN_KEY
    );

    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          ts: 1_700_000_000_000,
          total_viewers: 5,
          total_publishers: 1,
          nodes: [],
          protocol_mix: { webrtc: 1, hls: 0, rtmp: 0, dash: 0, other: 0 },
          apps: [],
        }),
      })
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: [], meta: { total: 0, has_more: false, next_cursor: null } }),
      })
    );

    await page.goto("/");

    // Dashboard must render first (confirms AuthGate passed)
    await expect(page.getByRole("heading", { name: "Live Dashboard" })).toBeVisible();

    // Simulate a 401 mid-session by dispatching the same event that apiFetch dispatches
    await page.evaluate(() => window.dispatchEvent(new Event("pulse:auth:401")));

    // AuthGate re-renders to gate view with error message
    await expect(page.getByText("Enter your API token to continue")).toBeVisible();
    const alert = page.getByRole("alert");
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("Session expired");

    // Token must be cleared from localStorage (clearToken called by the handler)
    const storedToken = await page.evaluate((key) => localStorage.getItem(key), TOKEN_KEY);
    expect(storedToken).toBeNull();
  });
});
