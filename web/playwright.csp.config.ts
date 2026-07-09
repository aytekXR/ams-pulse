import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for the CSP e2e job.
 *
 * Targets the Caddy-fronted pulse stack (deploy/docker-compose.csp-e2e.yml).
 * No webServer block — the compose stack is already up before this config runs.
 *
 * Only web/e2e/csp.spec.ts is matched (testMatch restricts to that file).
 * All other specs remain covered by playwright.config.ts (vite preview, no Caddy).
 *
 * baseURL: CADDY_BASE_URL env var (set to http://127.0.0.1:18080 in CI for IPv4
 * determinism) or http://localhost:18080 as the default for local runs.
 *
 * CI job: csp-e2e in .github/workflows/e2e.yml
 */
export default defineConfig({
  testDir: "./e2e",
  testMatch: ["**/csp.spec.ts"],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never" }]]
    : "list",

  use: {
    baseURL: process.env.CADDY_BASE_URL ?? "http://localhost:18080",
    trace: "on-first-retry",
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
