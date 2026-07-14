/**
 * Shared e2e stubs (S34).
 *
 * Every page in the app boots the same three requests before it renders anything
 * page-specific: the auth token check, the OIDC status probe, and the licence fetch
 * that <LicenseProvider> uses to decide tier gating. Before this file existed, each
 * spec re-declared all three inline — analytics.spec.ts and fleet.spec.ts already
 * carried near-identical copies. Six more pages of that would be six more chances
 * for the stubs to drift apart, and a stub that drifts silently changes what the
 * test is actually exercising.
 *
 * `stubApp()` installs the boot layer. Pass the tier the page under test needs:
 *
 *   free       → Probes gated, Anomalies gated, Reports gated
 *   pro        → Probes OPEN, Anomalies gated, Reports gated
 *   business   → Probes OPEN, Anomalies gated, Reports OPEN
 *   enterprise → everything OPEN
 *
 * (Sources: ReportsPage `isGated` = tier is neither business nor enterprise;
 * AnomaliesPage gates unless enterprise; ProbesPage gates only free.)
 *
 * Page-specific routes go in the spec that needs them — this file stays the boot
 * layer only, so a change here can't quietly rewrite an unrelated page's fixture.
 */
import type { Page, Route } from "@playwright/test";

export type Tier = "free" | "pro" | "business" | "enterprise";

const TOKEN_KEY = "pulse_token";

/** Fulfil a route with a JSON body — the shape every stub below needs. */
export function json(route: Route, body: unknown, status = 200): Promise<void> {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

/** The limits block the licence endpoint returns, widened per tier. */
function limitsFor(tier: Tier) {
  const byTier = {
    free: { max_nodes: 1, retention_days: 7, data_api: false, white_label: false },
    pro: { max_nodes: 10, retention_days: 90, data_api: true, white_label: false },
    business: { max_nodes: 50, retention_days: 365, data_api: true, white_label: true },
    enterprise: { max_nodes: null, retention_days: 365, data_api: true, white_label: true },
  } as const;
  return { ...byTier[tier], max_streams: null };
}

export interface StubAppOptions {
  /** Licence tier the app boots with. Drives every TierGate on the page. */
  tier?: Tier;
  /** Auth token written to localStorage before the first script runs. */
  token?: string;
}

/**
 * Install the boot-time routes every page depends on, and seed the auth token.
 * Call from `test.beforeEach` BEFORE `page.goto`.
 */
export async function stubApp(page: Page, options: StubAppOptions = {}): Promise<void> {
  const { tier = "enterprise", token = "plt_e2e" } = options;

  await page.addInitScript(
    ([key, value]) => localStorage.setItem(key, value),
    [TOKEN_KEY, token] as const,
  );

  await page.route("**/api/v1/admin/license", (route) =>
    json(route, {
      tier,
      valid: true,
      expires_at: null,
      offline_file: false,
      limits: limitsFor(tier),
    }),
  );

  await page.route("**/auth/me", (route) =>
    json(route, { name: "e2e", role: "admin", auth_method: "token" }),
  );

  await page.route("**/auth/oidc/status", (route) => json(route, { enabled: false }));
}

/**
 * Collect console errors and uncaught page exceptions.
 *
 * WebSocket noise is filtered: the live-overview socket has no server in the
 * preview build and its connection failure is expected, not a defect. Nothing
 * else is filtered — a spec asserting `expect(errors).toEqual([])` is asserting
 * that the page mounted cleanly, which is most of what a smoke test is for.
 */
export function collectErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    if (msg.text().includes("WebSocket")) return;
    errors.push(msg.text());
  });
  page.on("pageerror", (err) => errors.push(err.message));
  return errors;
}
