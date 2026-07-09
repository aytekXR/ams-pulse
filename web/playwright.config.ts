import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for the Pulse web UI e2e suite.
 *
 * Runs against the prod bundle served by `vite preview` (port 4173).
 * All API traffic is intercepted via page.route — no backend required.
 *
 * CI: promote `web-e2e` from continue-on-error to required after 2 weeks green.
 */
export default defineConfig({
  testDir: "./e2e",
  /**
   * Specs owned by DEDICATED configs — they need a real backend and MUST NOT
   * run against vite preview (D-061: the first main-push web-e2e run picked
   * them up and failed):
   *   csp.spec.ts               → playwright.csp.config.ts (Caddy on :18080)
   *   streams-render-500.spec.ts → playwright.realstack.config.ts (real stack)
   */
  testIgnore: ["**/csp.spec.ts", "**/streams-render-500.spec.ts"],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  /* Fail the run on test.only left in source during CI */
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never" }]]
    : "list",

  use: {
    baseURL: "http://127.0.0.1:4173",
    trace: "on-first-retry",
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],

  webServer: {
    /**
     * Serve the prod build. Run `npm run build` before `npm run test:e2e`
     * (or let CI do it in sequence). vite preview defaults to port 4173.
     * --host 127.0.0.1 forces IPv4 binding so the readiness URL resolves correctly
     * on hosts where Node defaults to [::1] (IPv6-only localhost).
     */
    command: "npm run preview -- --port 4173 --host 127.0.0.1",
    url: "http://127.0.0.1:4173",
    /* In CI always start fresh; locally reuse if already running */
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
