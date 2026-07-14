/**
 * Dashboard render specs.
 *
 * Verifies that the main nav and overview stat-cards render correctly from
 * mocked API responses, and that the prod bundle produces zero JS console
 * errors or uncaught page errors.
 *
 * WebSocket connection errors are excluded from the zero-error assertion:
 * vite preview does not serve a WS endpoint, so the LiveSocket's connection
 * attempt will fail and Chromium will log a network-level WebSocket error.
 * That is expected; the app falls back to REST polling.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";

const OVERVIEW_BODY = JSON.stringify({
  ts: 1_700_000_000_000,
  total_viewers: 42,
  total_publishers: 3,
  nodes: [],
  protocol_mix: { webrtc: 3, hls: 0, rtmp: 0, dash: 0, other: 0 },
  apps: [],
});

const STREAMS_BODY = JSON.stringify({
  items: [],
  meta: { total: 0, has_more: false, next_cursor: null },
});

test.describe("Dashboard render", () => {
  test.beforeEach(async ({ page }) => {
    // Inject a token so AuthGate passes
    await page.addInitScript(
      (key) => localStorage.setItem(key, "plt_e2e_dash_token"),
      TOKEN_KEY
    );

    // LicenseProvider fetches this at app root (S27 TrialBanner); unmocked it
    // ECONNREFUSEDs through the vite proxy and trips the zero-console-error gate.
    await page.route("/api/v1/admin/license", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          tier: "free",
          valid: true,
          expires_at: null,
          offline_file: false,
          limits: {
            max_nodes: 1,
            max_streams: null,
            retention_days: 7,
            data_api: false,
            white_label: false,
          },
        }),
      })
    );

    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: OVERVIEW_BODY })
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: STREAMS_BODY })
    );

    // OnboardingGuard fetches the source list on the dashboard and sends a user
    // with none to the wizard. Return one source so this test stays on the
    // dashboard (and so the request never 502s against the absent CI backend).
    await page.route("/api/v1/admin/sources", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: [{ id: "s1", name: "AMS", type: "rest_poll" }] }),
      })
    );

    // AuthGate mount-time checks: mock both /auth endpoints so the vite
    // preview /auth proxy (no backend on 8090 in CI) never answers 502.
    // /auth/me must be 200 here: Chromium logs ANY non-2xx resource load as
    // a console error, which would trip the zero-error assert below. Auth
    // semantics (401/HTML/network paths) are pinned in auth-gate.spec.ts and
    // the AuthGate unit tests — this spec only cares about dashboard render.
    await page.route("/auth/me", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "e2e", role: "admin", auth_method: "token" }),
      })
    );
    await page.route("/auth/oidc/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ enabled: false }),
      })
    );
  });

  test("nav + overview cards render; zero console errors + pageerrors", async ({ page }) => {
    const errors: string[] = [];

    // Capture JS console errors — skip WebSocket network errors which are expected
    // (vite preview has no /live/ws endpoint; the LiveSocket falls back to polling).
    page.on("console", (msg) => {
      if (msg.type() !== "error") return;
      if (msg.text().includes("WebSocket")) return;
      errors.push(msg.text());
    });
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/");

    // Wait for the dashboard heading to confirm app rendered
    await expect(page.getByRole("heading", { name: "Live Dashboard" })).toBeVisible();

    // Left-nav renders with the correct label
    const nav = page.getByRole("navigation", { name: "Main navigation" });
    await expect(nav).toBeVisible();
    await expect(nav.getByRole("link", { name: "Live" })).toBeVisible();
    await expect(nav.getByRole("link", { name: "Analytics" })).toBeVisible();
    await expect(nav.getByRole("link", { name: "Alerts" })).toBeVisible();
    await expect(nav.getByRole("link", { name: "Settings" })).toBeVisible();

    // StatCards render (label text from StatCard component)
    await expect(page.getByText("Viewers").first()).toBeVisible();
    await expect(page.getByText("Publishers").first()).toBeVisible();
    // "Streams" StatCard is always rendered
    await expect(page.getByText("Streams").first()).toBeVisible();

    // Zero app-level console errors
    expect(errors, `Unexpected console errors:\n${errors.join("\n")}`).toEqual([]);
  });
});
