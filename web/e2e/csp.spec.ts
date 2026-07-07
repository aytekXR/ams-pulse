/**
 * CSP spec — skipped: phase 2 (Caddy-fronted job).
 *
 * CSP headers are set by Caddy (deploy/config/Caddyfile line 71), not the Go
 * server. Playwright running against `vite preview` never sees the production
 * Content-Security-Policy header. This spec is a placeholder until a
 * Caddy-fronted CI job is wired up in phase 2.
 *
 * Phase 2 plan:
 *  - Stand up the full Caddy + pulse stack in the web-e2e CI job.
 *  - Assert `response.headers()['content-security-policy']` is present and
 *    that `page.on('console')` sees zero CSP violation messages.
 */
import { test } from "@playwright/test";

test.skip("CSP headers present and valid — phase 2 (Caddy-fronted job)", () => {
  // Phase 2 implementation:
  //   const response = await page.request.get('/');
  //   expect(response.headers()['content-security-policy']).toBeTruthy();
  //
  //   const violations: string[] = [];
  //   page.on('console', msg => {
  //     if (msg.type() === 'error' && msg.text().includes('Content Security Policy')) {
  //       violations.push(msg.text());
  //     }
  //   });
  //   await page.goto('/');
  //   expect(violations).toEqual([]);
});
