/**
 * VD-04/A10 — 500-stream render measurement against the REAL compose stack.
 *
 * No page.route() mocking — this spec contacts the real pulse binary and the real
 * API (/api/v1/live/streams?limit=500). It must only be run with
 * playwright.realstack.config.ts (baseURL = pulse on :18090).
 *
 * Prerequisites:
 *   - PULSE_ADMIN_TOKEN env var set (bootstrap token from pulse logs).
 *   - PULSE_BASE_URL env var set (default http://127.0.0.1:18090).
 *   - 500 streams seeded in mock-ams via /control/bulk_publish.
 *   - 15 s propagation delay already elapsed (restpoller picks up streams).
 *
 * Budget: < 2 s  (ARCHITECTURE.md §4, row VD-04, budget 2 s).
 * Policy: record the number and log it; emit a VD-04 FINDING console.error if
 *   > 2000 ms, but the test MUST NOT fail on timing — a wall-clock gate on shared
 *   CI runners is a D-042-class flake. The acceptance decision is recorded in
 *   ARCHITECTURE.md from reproducible local runs by the verifier.
 *
 * Two test cases: run 1 and run 2 (variance visibility). Each navigates fresh to "/".
 *
 * Measurement window: wall-clock from page.goto("/") to:
 *   - role="grid" aria-label="Active streams" visible (virtualizer mounted), AND
 *   - text "500 streams" visible (footer, StreamsTable.tsx ~line 205 — from real API).
 * This captures: SPA load + AuthGate pass + API round-trip + React render + virtual
 * list paint.
 *
 * Token injection: localStorage["pulse_token"] set via page.addInitScript before
 * navigation. AuthGate (web/src/features/auth/AuthGate.tsx) reads this key and skips
 * the login screen, letting the SPA call /api/v1/live/streams with the real token.
 */
import { test, expect, type Page } from "@playwright/test";

const TOKEN_KEY = "pulse_token";
const STREAM_CNT = 500;
const BUDGET_MS = 2_000;

test.describe("VD-04/A10 — 500-stream render measurement (real stack)", () => {
  test.beforeEach(async ({ page }) => {
    const tok = process.env.PULSE_ADMIN_TOKEN;
    if (!tok) {
      throw new Error(
        "PULSE_ADMIN_TOKEN must be set — extract from pulse logs: " +
          "grep -o 'plt_[A-Za-z0-9_-]*' <(docker logs <pulse-container>)"
      );
    }
    await page.addInitScript(
      ({ k, t }: { k: string; t: string }) => localStorage.setItem(k, t),
      { k: TOKEN_KEY, t: tok }
    );
  });

  /**
   * Navigate to "/" and measure wall-clock time until both the grid and the
   * "500 streams" footer are visible. Returns elapsed milliseconds.
   */
  async function measure(page: Page): Promise<number> {
    const t0 = Date.now();
    await page.goto("/");
    const grid = page.getByRole("grid", { name: "Active streams" });
    await expect(grid).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(`${STREAM_CNT} streams`)).toBeVisible({
      timeout: 15_000,
    });
    return Date.now() - t0;
  }

  test("run 1 — measure render time", async ({ page }) => {
    const ms = await measure(page);
    console.log(`VD-04 run1 render_ms=${ms}`);
    if (ms > BUDGET_MS) {
      console.error(
        `VD-04 FINDING: run1 render_ms=${ms} > ${BUDGET_MS} ms — ` +
          "record in ARCHITECTURE.md §4 + §11; do not tune the test"
      );
    }
    test.info().annotations.push({
      type: "vd04_run1_ms",
      description: String(ms),
    });

    // Hard structural assertions (independent of timing):
    const grid = page.getByRole("grid", { name: "Active streams" });
    await expect(grid).toHaveAttribute("aria-rowcount", `${STREAM_CNT + 1}`);
    const rows = grid.getByRole("rowgroup").getByRole("row");
    const rowCount = await rows.count();
    expect(
      rowCount,
      `Expected ≤ 35 rendered rows (virtual window), got ${rowCount}`
    ).toBeLessThanOrEqual(35);
    expect(rowCount, "Expected at least 1 rendered row").toBeGreaterThan(0);
    await expect(page.getByText(`${STREAM_CNT} streams`)).toBeVisible();
  });

  test("run 2 — repeat for variance check", async ({ page }) => {
    const ms = await measure(page);
    console.log(`VD-04 run2 render_ms=${ms}`);
    if (ms > BUDGET_MS) {
      console.error(
        `VD-04 FINDING: run2 render_ms=${ms} > ${BUDGET_MS} ms — ` +
          "record in ARCHITECTURE.md §4 + §11; do not tune the test"
      );
    }
    test.info().annotations.push({
      type: "vd04_run2_ms",
      description: String(ms),
    });

    // Hard structural assertions (same as run 1 — both must pass):
    const grid = page.getByRole("grid", { name: "Active streams" });
    await expect(grid).toHaveAttribute("aria-rowcount", `${STREAM_CNT + 1}`);
    const rows = grid.getByRole("rowgroup").getByRole("row");
    const rowCount = await rows.count();
    expect(
      rowCount,
      `Expected ≤ 35 rendered rows (virtual window), got ${rowCount}`
    ).toBeLessThanOrEqual(35);
    expect(rowCount, "Expected at least 1 rendered row").toBeGreaterThan(0);
    await expect(page.getByText(`${STREAM_CNT} streams`)).toBeVisible();
  });
});
