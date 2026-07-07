/**
 * AuthGate e2e specs.
 *
 * There is NO /login route. AuthGate (web/src/components/AuthGate.tsx) renders
 * an inline token panel when localStorage["pulse_token"] is null, and listens
 * for the "pulse:auth:401" CustomEvent to clear the token on session expiry.
 *
 * All API traffic is route-mocked via page.route — no backend required.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

/** Minimal valid LiveOverview for route mocks */
const OVERVIEW_BODY = JSON.stringify({
  ts: 1_700_000_000_000,
  total_viewers: 7,
  total_publishers: 1,
  nodes: [],
  protocol_mix: { webrtc: 1, hls: 0, rtmp: 0, dash: 0, other: 0 },
  apps: [],
});

/** Empty stream list */
const STREAMS_BODY = JSON.stringify({
  items: [],
  meta: { total: 0, has_more: false, next_cursor: null },
});

test.describe("AuthGate", () => {
  test("unauthenticated — token panel renders", async ({ page }) => {
    // No localStorage token
    await page.goto("/");
    await expect(page.getByText("Enter your API token to continue")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("invalid token — 401 fires pulse:auth:401 — error stays on gate", async ({ page }) => {
    // All API calls return 401 — simulates an invalid token being rejected
    await page.route("/api/v1/**", (route) =>
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ message: "Unauthorized", code: "401" }),
      })
    );

    await page.goto("/");

    // Gate is visible (no token yet)
    await expect(page.getByText("Enter your API token to continue")).toBeVisible();

    // Enter a token and submit — AuthGate stores it and shows children
    await page.getByLabel("API Token").fill("plt_invalid_token_xyz");
    await page.getByRole("button", { name: "Sign in" }).click();

    // The dashboard mounts and makes API calls → 401 → pulse:auth:401 dispatched
    // → AuthGate clears token and shows error ("Session expired or token revoked…")
    const alert = page.getByRole("alert");
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("Session expired");

    // Gate is back
    await expect(page.getByText("Enter your API token to continue")).toBeVisible();
  });

  test("mocked-valid token — dashboard renders", async ({ page }) => {
    // Inject token before page scripts run
    await page.addInitScript(
      (key) => localStorage.setItem(key, "plt_e2e_valid_token"),
      TOKEN_KEY
    );

    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: OVERVIEW_BODY })
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: STREAMS_BODY })
    );

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Live Dashboard" })).toBeVisible();
  });
});
