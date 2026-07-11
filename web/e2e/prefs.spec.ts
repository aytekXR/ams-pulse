/**
 * Preferences (theme + density) E2E specs — B1 work order.
 *
 * (i)   pulse_theme=light in localStorage → --color-bg is the light bg value
 * (ii)  No localStorage → default dark → --color-bg is the dark bg value
 * (iii) Click the theme toggle → data-theme flips; persists after reload
 * (iv)  data-density=compact stamped → --row-height computes 32px
 * (v)   prefers-reduced-motion: reduce → --motion-fast computes 0ms
 *
 * Modelled on web/e2e/dashboard-render.spec.ts.
 */
import { test, expect } from "@playwright/test";

const TOKEN_KEY = "pulse_token";
const THEME_KEY = "pulse_theme";
const DENSITY_KEY = "pulse_density";

// Minimal API stubs so AuthGate passes and the app shell renders
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

async function stubApiAndAuth(page: import("@playwright/test").Page) {
  await page.addInitScript(
    (key) => localStorage.setItem(key, "plt_e2e_prefs_token"),
    TOKEN_KEY
  );
  await page.route("/api/v1/live/overview", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: OVERVIEW_BODY })
  );
  await page.route("/api/v1/live/streams**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: STREAMS_BODY })
  );
  // Mock AuthGate mount-time endpoints (vite preview proxies /auth with no
  // backend in CI — unmocked requests would 502).
  await page.route("/auth/me", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "unauthorized" }),
    })
  );
  await page.route("/auth/oidc/status", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ enabled: false }),
    })
  );
}

/** Read a CSS custom property value from documentElement via page.evaluate */
async function getCSSVar(page: import("@playwright/test").Page, varName: string): Promise<string> {
  return page.evaluate((v) => {
    return getComputedStyle(document.documentElement).getPropertyValue(v).trim();
  }, varName);
}

test.describe("Preferences — theme + density tokens", () => {
  test("(i) pulse_theme=light → --color-bg is the light bg (#F7F9FA)", async ({ page }) => {
    await stubApiAndAuth(page);
    // Inject light theme before the page loads
    await page.addInitScript(
      ([themeKey, themeVal]) => localStorage.setItem(themeKey, themeVal),
      [THEME_KEY, "light"]
    );
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();

    const bg = await getCSSVar(page, "--color-bg");
    // tokens.json color.light.bg = #F7F9FA
    expect(bg.toLowerCase()).toBe("#f7f9fa");
  });

  test("(ii) default (no localStorage) → dark → --color-bg is #0A0E14", async ({ page }) => {
    await stubApiAndAuth(page);
    // Pin the ambient color scheme: headless Chromium defaults to LIGHT,
    // which would legitimately resolve the theme to light. "Default is dark"
    // means "dark when the OS does not prefer light" — emulate that.
    await page.emulateMedia({ colorScheme: "dark" });
    // Explicitly ensure no theme key in localStorage
    await page.addInitScript(
      (key) => localStorage.removeItem(key),
      THEME_KEY
    );
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();

    const bg = await getCSSVar(page, "--color-bg");
    // tokens.json color.dark.bg = #0A0E14
    expect(bg.toLowerCase()).toBe("#0a0e14");
  });

  test("(iii) click theme toggle → data-theme flips + persists after reload", async ({ page }) => {
    await stubApiAndAuth(page);
    // Pin ambient scheme to dark so the start state (and toggle label
    // "Switch to light theme") is deterministic across environments.
    await page.emulateMedia({ colorScheme: "dark" });
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();

    // Default is dark; toggle should switch to light
    const toggle = page.getByRole("button", { name: /switch to light theme/i });
    await expect(toggle).toBeVisible();
    await toggle.click();

    // data-theme should now be light
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

    // Reload and confirm the theme persisted via localStorage
    await page.reload();
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });

  test("(iv) data-density=compact stamped → --row-height computes 32px", async ({ page }) => {
    await stubApiAndAuth(page);
    // Stamp compact density before load
    await page.addInitScript(
      ([densityKey, densityVal]) => localStorage.setItem(densityKey, densityVal),
      [DENSITY_KEY, "compact"]
    );
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();

    const rowHeight = await getCSSVar(page, "--row-height");
    // [data-density="compact"] → --row-height: 32px
    expect(rowHeight).toBe("32px");
  });

  test("(v) prefers-reduced-motion: reduce → --motion-fast computes 0ms", async ({ page }) => {
    await stubApiAndAuth(page);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/");
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();

    const motionFast = await getCSSVar(page, "--motion-fast");
    // @media (prefers-reduced-motion: reduce) → --motion-fast: 0ms.
    // The prod CSS minifier rewrites 0ms → 0s (shorter serialization), so
    // accept either zero form.
    expect(["0ms", "0s"]).toContain(motionFast);
  });
});
