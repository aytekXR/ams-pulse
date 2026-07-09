import { defineConfig, devices } from "@playwright/test";

/**
 * Real-stack Playwright config — VD-04/A10 measurement.
 *
 * Target: pulse binary at PULSE_BASE_URL (default http://127.0.0.1:18090).
 * Requires the compose stack to already be running and PULSE_ADMIN_TOKEN to be
 * set. There is no webServer block — the compose stack provides the server.
 *
 * Only matches web/e2e/streams-render-500.spec.ts (real-API, no mocking).
 * All existing mock-based specs continue to run under playwright.config.ts.
 */
export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/streams-render-500.spec.ts",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never" }]]
    : "list",
  use: {
    baseURL: process.env.PULSE_BASE_URL ?? "http://127.0.0.1:18090",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
