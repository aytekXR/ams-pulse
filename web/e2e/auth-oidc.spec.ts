/**
 * OIDC phase-2 e2e specs (S14 WO-C).
 *
 * Route-mocked — no real backend required. Modelled on auth-gate.spec.ts.
 * All route mocks intercept both /auth/oidc/status and /auth/me so the
 * AuthGate mount-time checks resolve deterministically in CI.
 *
 * Do NOT run locally when browser libs are absent; CI runs this under
 * web-e2e (continue-on-error). Chromium only.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

/** Minimal valid LiveOverview for downstream route mocks. */
const OVERVIEW_BODY = JSON.stringify({
  ts: 1_700_000_000_000,
  total_viewers: 0,
  total_publishers: 0,
  nodes: [],
  protocol_mix: { webrtc: 0, hls: 0, rtmp: 0, dash: 0, other: 0 },
  apps: [],
});

const STREAMS_BODY = JSON.stringify({
  items: [],
  meta: { total: 0, has_more: false, next_cursor: null },
});

test.describe("AuthGate — OIDC phase-2", () => {
  test("SSO button visible when OIDC is enabled", async ({ page }) => {
    // /auth/oidc/status returns enabled=true; /auth/me returns 401 (no cookie)
    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ enabled: true }),
      }),
    );
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ code: "UNAUTHORIZED", message: "not authenticated" }),
      }),
    );

    await page.goto("/");

    // The token panel should be visible with the SSO button.
    await expect(page.getByText("Enter your API token to continue")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in with SSO" })).toBeVisible();
  });

  test("SSO button absent when OIDC is disabled", async ({ page }) => {
    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ enabled: false }),
      }),
    );
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ code: "UNAUTHORIZED", message: "not authenticated" }),
      }),
    );

    await page.goto("/");

    await expect(page.getByText("Enter your API token to continue")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in with SSO" })).not.toBeVisible();
  });

  test("cookie-authed session renders dashboard without token entry", async ({ page }) => {
    // /auth/me returns 200 → cookieAuthed=true → dashboard renders
    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ enabled: true }),
      }),
    );
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "oidc-session", role: "viewer", auth_method: "cookie" }),
      }),
    );
    // Stub downstream API so the Live dashboard renders without errors.
    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: OVERVIEW_BODY }),
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: STREAMS_BODY }),
    );

    // No localStorage token — relying purely on cookie auth detected via /auth/me.
    await page.goto("/");

    // Dashboard heading should be visible; token entry panel must be absent.
    await expect(page.getByRole("heading", { name: "Live Dashboard" })).toBeVisible();
    await expect(page.getByText("Enter your API token to continue")).not.toBeVisible();
  });

  test("bearer token still works when OIDC is enabled", async ({ page }) => {
    await page.addInitScript(
      (key) => localStorage.setItem(key, "plt_e2e_valid_token"),
      TOKEN_KEY,
    );

    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ enabled: true }),
      }),
    );
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "test-token", role: "admin", auth_method: "bearer" }),
      }),
    );
    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: OVERVIEW_BODY }),
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: STREAMS_BODY }),
    );

    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Live Dashboard" })).toBeVisible();
  });
});
