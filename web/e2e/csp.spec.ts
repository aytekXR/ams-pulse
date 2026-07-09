/**
 * CSP spec — Caddy-fronted job (phase 2 implementation).
 *
 * Runs against the CSP e2e compose stack (deploy/docker-compose.csp-e2e.yml)
 * where Caddy fronts pulse and sets Content-Security-Policy on every response.
 * Config: web/playwright.csp.config.ts (baseURL = http://localhost:18080).
 * CI job: csp-e2e in .github/workflows/e2e.yml.
 *
 * THREE TESTS:
 *   1. GET / response carries Content-Security-Policy header — EXACT equality to
 *      the canonical Caddyfile.ci policy string, plus parity headers present.
 *   2. Auth-gate page — zero securitypolicyviolation DOM events and zero CSP
 *      console errors (auth gate renders the SPA bundle with no API calls).
 *   3. Dashboard page (mocked API via page.route) — same zero-violation asserts.
 *      page.route() intercepts at the browser network layer before requests reach
 *      Caddy; HTML/JS/CSS assets still flow through Caddy and are CSP-checked.
 *
 * MUTATION CHECK (cannot-false-green sentinel):
 *   Remove the Content-Security-Policy directive from deploy/config/Caddyfile.ci,
 *   reload Caddy, and re-run this spec — Test 1 MUST FAIL (the exact-equality
 *   assert catches the missing header). Restore the directive → all green again.
 *
 * WHY EXACT EQUALITY (not substring):
 *   Substring matching can mask policy drift. If a directive is removed or
 *   weakened in Caddyfile.ci, only an exact-equality assert guarantees the spec
 *   catches it. Reviewers who want to change the policy MUST update this string.
 */
import { test, expect } from "@playwright/test";

// Extend the Window interface so TypeScript accepts window.__cspViolations
// in page.evaluate callbacks (the e2e files are not covered by the main
// tsconfig include, but keeping it typed avoids any Playwright ts-jest issues).
declare global {
  interface Window {
    __cspViolations?: string[];
  }
}

// ── Canonical CSP string ─────────────────────────────────────────────────────
// This MUST byte-match the Content-Security-Policy value in Caddyfile.ci.
// Changing either location without updating the other will break Test 1.
const CANONICAL_CSP =
  "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
  "img-src 'self' data:; font-src 'self'; connect-src 'self' ws://localhost:18080; " +
  "object-src 'none'; base-uri 'self'; frame-ancestors 'none'";

// ── Mock response bodies (mocked-dashboard test) ────────────────────────────
const TOKEN_KEY = "pulse_token";

const OVERVIEW_BODY = JSON.stringify({
  ts: 1_700_000_000_000,
  total_viewers: 5,
  total_publishers: 1,
  nodes: [],
  protocol_mix: { webrtc: 1, hls: 0, rtmp: 0, dash: 0, other: 0 },
  apps: [],
});

const STREAMS_BODY = JSON.stringify({
  items: [],
  meta: { total: 0, has_more: false, next_cursor: null },
});

// ── Violation collectors ─────────────────────────────────────────────────────

/**
 * Install CSP violation collectors on the page BEFORE navigation.
 * Two layers: DOM-level securitypolicyviolation events (fires before console)
 * and console-level errors (belt-and-suspenders).
 * Must be called before page.goto(); page.addInitScript runs before page JS.
 */
async function installViolationCollectors(
  page: import("@playwright/test").Page
): Promise<{ cspErrors: string[] }> {
  const cspErrors: string[] = [];

  // DOM listener — injected via addInitScript so it is active before any
  // page scripts run.  Captures async-loaded resource violations too.
  await page.addInitScript(() => {
    window.__cspViolations = [];
    document.addEventListener("securitypolicyviolation", (e) => {
      window.__cspViolations!.push(
        `blocked: ${e.blockedURI} (directive: ${e.violatedDirective})`
      );
    });
  });

  // Console listener — catches the browser's own CSP error log messages.
  page.on("console", (msg) => {
    if (
      msg.type() === "error" &&
      msg.text().includes("Content Security Policy")
    ) {
      cspErrors.push(msg.text());
    }
  });

  return { cspErrors };
}

/** Read DOM violations collected since page load. */
async function collectDOMViolations(
  page: import("@playwright/test").Page
): Promise<string[]> {
  return page.evaluate(() => window.__cspViolations ?? []);
}

// ── Tests ────────────────────────────────────────────────────────────────────

test.describe("CSP — Caddy-fronted", () => {
  /**
   * Test 1: Exact-equality header assert + parity headers.
   *
   * Uses page.request.get() (Playwright APIRequestContext — pure HTTP, no browser
   * page load) to fetch the Caddy response and inspect its headers.
   * Any deviation from CANONICAL_CSP causes this test to fail immediately.
   */
  test("GET / carries exact Content-Security-Policy header and parity headers", async ({
    page,
  }) => {
    const response = await page.request.get("/");

    const headers = response.headers();

    // ── Primary: exact CSP equality ────────────────────────────────────────
    const csp = headers["content-security-policy"];
    expect(
      csp,
      `CSP header missing from Caddy response (HTTP ${response.status()})`
    ).toBeDefined();
    expect(csp, "CSP header must exactly equal the Caddyfile.ci canonical value").toBe(
      CANONICAL_CSP
    );

    // ── Parity: other security headers ──────────────────────────────────────
    expect(
      headers["x-content-type-options"],
      "X-Content-Type-Options must be nosniff"
    ).toBe("nosniff");

    expect(
      headers["x-frame-options"],
      "X-Frame-Options must be DENY"
    ).toBe("DENY");

    expect(
      headers["referrer-policy"],
      "Referrer-Policy must be present"
    ).toBeTruthy();

    expect(
      headers["permissions-policy"],
      "Permissions-Policy must be present"
    ).toBeTruthy();
  });

  /**
   * Test 2: Auth-gate page — zero CSP violations.
   *
   * The auth gate renders the SPA bundle without any API calls (no token in
   * localStorage).  Every asset (HTML, JS, CSS) flows through Caddy and is
   * subject to the CSP policy.  Any inline script, external resource, or
   * style violation would fire the securitypolicyviolation event.
   */
  test("auth gate page — zero CSP violations", async ({ page }) => {
    const { cspErrors } = await installViolationCollectors(page);

    await page.goto("/");

    // Auth gate must render — proves the page actually loaded (not a blank/error page).
    await expect(
      page.getByText("Enter your API token to continue")
    ).toBeVisible();

    const domViolations = await collectDOMViolations(page);
    expect(
      domViolations,
      `DOM securitypolicyviolation events captured:\n${domViolations.join("\n")}`
    ).toEqual([]);
    expect(
      cspErrors,
      `Console CSP error messages captured:\n${cspErrors.join("\n")}`
    ).toEqual([]);
  });

  /**
   * Test 3: Dashboard page (mocked API) — zero CSP violations.
   *
   * Inject a fake token via addInitScript so AuthGate skips the login screen.
   * Mock the two API endpoints so the dashboard can render without a real pulse
   * backend (the CSP job does not mint a Pro license).
   * HTML/JS/CSS still flow through Caddy and are checked against the policy;
   * only the XHR/fetch calls to /api/v1/** are intercepted by page.route().
   */
  test("dashboard page (mocked API) — zero CSP violations", async ({
    page,
  }) => {
    // Inject fake auth token before page scripts run.
    await page.addInitScript(
      (key) => localStorage.setItem(key, "plt_e2e_csp_token"),
      TOKEN_KEY
    );

    // Mock API endpoints so dashboard can render without real backend auth.
    await page.route("/api/v1/live/overview", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: OVERVIEW_BODY,
      })
    );
    await page.route("/api/v1/live/streams**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: STREAMS_BODY,
      })
    );

    const { cspErrors } = await installViolationCollectors(page);

    await page.goto("/");

    // Dashboard heading must be visible — proves the dashboard actually rendered.
    await expect(
      page.getByRole("heading", { name: "Live Dashboard" })
    ).toBeVisible();

    const domViolations = await collectDOMViolations(page);
    expect(
      domViolations,
      `DOM securitypolicyviolation events captured:\n${domViolations.join("\n")}`
    ).toEqual([]);
    expect(
      cspErrors,
      `Console CSP error messages captured:\n${cspErrors.join("\n")}`
    ).toEqual([]);
  });
});
