/*
 * Marketplace demo video — ROUGH-CUT generator (D-169 / S103).
 *
 *   node qa/marketplace/capture-demo-video.mjs
 *   → writes docs/marketplace/demo/pulse-demo-roughcut.webm  (gitignored)
 *
 * A first-draft, silent screen-capture walkthrough of the six flagship screens,
 * driven against the live React UI (vite preview) with fully route-mocked API
 * data — NO backend, NO AMS, NEVER touches prod. It mirrors the storyboard in
 * docs/marketplace/demo-video-script.md so the operator can re-record the final
 * with voiceover/branding over the top, or use this as-is for internal review.
 *
 * REQUIREMENTS. Playwright's Chromium plus its OS libraries and bundled ffmpeg.
 * The browser binaries download with `npm install`, but the Chromium SYSTEM
 * LIBRARIES (libatk, libcups, libasound, libcairo, libpango, …) are a separate,
 * root-gated install and are ABSENT on the Pulse VPS. Run this where they exist:
 *   - CI (the e2e runners already have them), or
 *   - after `sudo npx playwright install-deps chromium` on a workstation, or
 *   - inside the Playwright image, e.g.:
 *       docker run --rm --network host -e PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
 *         -v "$PWD":/repo -w /repo/web \
 *         mcr.microsoft.com/playwright:v1.61.1-jammy \
 *         node /repo/qa/marketplace/capture-demo-video.mjs
 * On a host missing the libraries the browser fails to launch — that is the one
 * dependency this script cannot self-satisfy (tracked in operator-expected).
 */

import { spawn, execSync } from "child_process";
import { existsSync, mkdirSync, renameSync, readdirSync } from "fs";
import { join, dirname, resolve } from "path";
import { fileURLToPath, pathToFileURL } from "url";
import net from "net";

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

// Playwright is resolved relative to this script (web/node_modules), never an
// absolute machine path — the script must run from any clone location.
const pwMod = await import(pathToFileURL(
  join(REPO_ROOT, "web/node_modules/@playwright/test/index.js"),
).href);
const { chromium } = pwMod.default ?? pwMod;
const WEB_DIR = join(REPO_ROOT, "web");
const DIST_DIR = join(WEB_DIR, "dist");
const OUT_DIR = join(REPO_ROOT, "docs/marketplace/demo");
const VIDEO_PATH = join(OUT_DIR, "pulse-demo-roughcut.webm");
const PORT = 4319; // avoid prod (8090) and the screenshot script's port
const BASE_URL = `http://127.0.0.1:${PORT}`;
const VIEWPORT = { width: 1920, height: 1080 };

const log = (m) => console.log(`[demo-video] ${m}`);
const json = (body) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });

function isPortInUse(port) {
  return new Promise((resolve) => {
    const s = net.createConnection({ port, host: "127.0.0.1" });
    s.on("connect", () => { s.end(); resolve(true); });
    s.on("error", () => resolve(false));
  });
}
async function waitForUrl(url, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const r = await fetch(url);
      if (r.ok || r.status === 404) return true;
    } catch { /* not up yet */ }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`timed out waiting for ${url}`);
}

// ── Fixtures (shapes mirror qa/marketplace/capture-live-screenshots.mjs) ───────
const NOW = 1_784_000_000_000; // fixed epoch ms — deterministic, no Date.now()
const HOUR = 3_600_000;

const license = (tier = "enterprise") => ({
  tier, valid: true, expires_at: null, offline_file: false,
  limits: { max_nodes: null, max_streams: null, retention_days: 365, data_api: true, white_label: true },
});
const AUTH_ME = { name: "admin", role: "admin", auth_method: "token" };
const OIDC_OFF = { enabled: false };
const HEALTHZ_OK = { status: "ok", ams_env_configured: true };
const SOURCES_ONE = { items: [{ id: "src-1", name: "Origin AMS", type: "rest_poll", rest_url: "http://10.0.0.1:5080", created_at: NOW - 86_400_000 }] };
const TOKENS_BODY = { items: [] };

const OVERVIEW_POPULATED = {
  ts: NOW, total_viewers: 847, total_publishers: 12,
  nodes: [
    { node_id: "origin-1", role: "origin", status: "up", cpu_pct: 38, mem_pct: 61, net_out_mbps: 312 },
    { node_id: "edge-us-1", role: "edge", status: "up", cpu_pct: 22, mem_pct: 44, net_out_mbps: 188 },
    { node_id: "edge-eu-1", role: "edge", status: "up", cpu_pct: 19, mem_pct: 38, net_out_mbps: 141 },
  ],
  protocol_mix: { webrtc: 5, hls: 4, rtmp: 2, dash: 1, other: 0 },
  apps: [
    { app: "live", stream_count: 5, viewer_count: 612 },
    { app: "webrtc", stream_count: 2, viewer_count: 180 },
    { app: "vod", stream_count: 1, viewer_count: 55 },
  ],
};
const STREAMS_POPULATED = {
  items: [
    { stream_id: "live/main-broadcast", app: "live", node_id: "origin-1", viewers: 312, bitrate_kbps: 4800, fps: 30, uptime_s: 7200, publisher_ip: "203.0.113.10" },
    { stream_id: "live/gaming-channel", app: "live", node_id: "origin-1", viewers: 189, bitrate_kbps: 6000, fps: 60, uptime_s: 3600, publisher_ip: "203.0.113.11" },
    { stream_id: "live/sports-coverage", app: "live", node_id: "edge-us-1", viewers: 111, bitrate_kbps: 3200, fps: 30, uptime_s: 1800, publisher_ip: "203.0.113.12" },
    { stream_id: "live/news-live", app: "live", node_id: "origin-1", viewers: 88, bitrate_kbps: 2500, fps: 25, uptime_s: 5400, publisher_ip: "203.0.113.13" },
    { stream_id: "webrtc/conf-a", app: "webrtc", node_id: "origin-1", viewers: 44, bitrate_kbps: 1800, fps: 30, uptime_s: 900, publisher_ip: "203.0.113.15" },
  ],
  meta: { total: 5, has_more: false, next_cursor: null },
};
const QOE_INGEST = {
  streams: [
    { stream_id: "live/main-broadcast", app: "live", node_id: "origin-1", health_score: 95,
      timeseries: Array.from({ length: 6 }, (_, i) => ({ ts: NOW - (5 - i) * 60_000, bitrate_kbps: 4780 + i * 8, fps: 30, packet_loss_pct: 0.1, jitter_ms: 3 })),
      drop_events: [] },
  ],
};
const ALERT_RULES = {
  items: [
    { id: "r1", name: "CPU Critical", metric: "node_cpu", operator: "gt", threshold: 90, window_s: 300, severity: "critical", cooldown_s: 600, enabled: true, muted: false, created_at: NOW - 86_400_000, rule_type: "threshold" },
    { id: "r2", name: "Viewer Floor", metric: "viewer_count_floor", operator: "lt", threshold: 10, window_s: 120, severity: "warning", cooldown_s: 300, enabled: true, muted: false, created_at: NOW - 86_400_000, rule_type: "threshold" },
    { id: "r3", name: "Rebuffer Ratio", metric: "rebuffer_ratio", operator: "gt", threshold: 0.05, window_s: 60, severity: "warning", cooldown_s: 300, enabled: true, muted: false, created_at: NOW - 86_400_000, rule_type: "threshold" },
    { id: "r4", name: "Bitrate Anomaly", metric: "ingest_bitrate_kbps", operator: "lt", threshold: 1000, window_s: 3600, severity: "info", cooldown_s: 900, enabled: true, muted: false, created_at: NOW - 86_400_000, rule_type: "anomaly" },
  ],
};
const ALERT_CHANNELS = { items: [
  { id: "ch1", name: "Ops Slack", type: "slack", config: {}, enabled: true },
  { id: "ch2", name: "PagerDuty", type: "webhook", config: {}, enabled: true },
] };
const ALERT_HISTORY = { items: [
  { id: "h1", rule_id: "r1", rule_name: "CPU Critical", severity: "critical", fired_at: NOW - 1_800_000, resolved_at: NOW - 1_200_000, node_id: "origin-1" },
  { id: "h2", rule_id: "r3", rule_name: "Rebuffer Ratio", severity: "warning", fired_at: NOW - 900_000, resolved_at: null, node_id: "edge-us-1" },
] };
const QOE_SUMMARY = {
  totals: { startup_p50_ms: 380, startup_p95_ms: 920, rebuffer_ratio: 0.012, error_rate: 0.008 },
  bitrate_timeline: Array.from({ length: 12 }, (_, i) => ({ ts: NOW - (11 - i) * HOUR, bitrate_kbps_p50: 3800 + Math.round(Math.sin(i * 0.6) * 400), bitrate_kbps_p95: 5400 + Math.round(Math.cos(i * 0.4) * 600) })),
};
const ANALYTICS_AUDIENCE = {
  totals: { views: 28_450, uniques: 9_812, watch_time_s: 892_800, peak_concurrency: 847 },
  timeseries: Array.from({ length: 14 }, (_, i) => ({ ts: NOW - (13 - i) * 24 * HOUR, views: 1800 + Math.round(Math.sin(i * 0.7) * 400) + i * 80, uniques: 620 + Math.round(Math.cos(i * 0.5) * 150) + i * 28, watch_time_s: 63_000 + i * 3_000, peak_concurrency: 500 + Math.round(Math.sin(i * 0.9) * 120) + i * 20 })),
};
const ANALYTICS_GEO = { rows: [
  { country: "US", views: 11_200, uniques: 3_900, watch_time_s: 360_000 },
  { country: "TR", views: 5_400, uniques: 1_800, watch_time_s: 172_800 },
  { country: "DE", views: 3_900, uniques: 1_350, watch_time_s: 124_200 },
] };
const ANALYTICS_DEVICES = { rows: [
  { device: "desktop", views: 14_200, watch_time_s: 450_000 },
  { device: "mobile", views: 9_800, watch_time_s: 312_000 },
] };
const REPORTS_USAGE = {
  rows: [
    { app: "live", period: "2026-07", viewer_minutes: 482_000, peak_concurrency: 847, egress_gb: 2_840, recording_gb: 380 },
    { app: "webrtc", period: "2026-07", viewer_minutes: 148_000, peak_concurrency: 212, egress_gb: 880, recording_gb: 0 },
  ],
  totals: { viewer_minutes: 630_000, peak_concurrency: 847, egress_gb: 3_720, recording_gb: 380 },
  egress_method: "bitrate_x_watch_time",
};
const REPORTS_SCHEDULES = { items: [], meta: { total: 0, next_cursor: null } };

// ── Page mocks ─────────────────────────────────────────────────────────────
async function installMocks(page) {
  await page.addInitScript(([k, v]) => localStorage.setItem(k, v), ["pulse_token", "plt_demo"]);
  // S105: pin the brand-default dark theme explicitly — Playwright's default
  // emulated prefers-color-scheme is light, and with no stored choice the app
  // falls through to matchMedia (the first rough cut silently rendered light).
  await page.addInitScript(([k, v]) => localStorage.setItem(k, v), ["pulse_theme", "dark"]);
  const R = (pat, body) => page.route(pat, (r) => r.fulfill(json(body)));
  await R("**/auth/me", AUTH_ME);
  await R("**/auth/oidc/status", OIDC_OFF);
  await R("**/api/v1/admin/license", license("enterprise"));
  await R("**/healthz", HEALTHZ_OK);
  await R("**/api/v1/admin/sources", SOURCES_ONE);
  await R("**/api/v1/admin/tokens", TOKENS_BODY);
  await R("**/api/v1/live/overview", OVERVIEW_POPULATED);
  await R("**/api/v1/live/streams**", STREAMS_POPULATED);
  await R("**/api/v1/qoe/ingest**", QOE_INGEST);
  await R("**/api/v1/qoe/summary**", QOE_SUMMARY);
  await R("**/api/v1/alerts/rules**", ALERT_RULES);
  await R("**/api/v1/alerts/channels**", ALERT_CHANNELS);
  await R("**/api/v1/alerts/history**", ALERT_HISTORY);
  await R("**/api/v1/analytics/audience**", ANALYTICS_AUDIENCE);
  await R("**/api/v1/analytics/geo**", ANALYTICS_GEO);
  await R("**/api/v1/analytics/devices**", ANALYTICS_DEVICES);
  await R("**/api/v1/reports/usage**", REPORTS_USAGE);
  await R("**/api/v1/reports/schedules**", REPORTS_SCHEDULES);
  await R("**/api/v1/anomalies**", { items: [] });
  await R("**/api/v1/fleet/nodes**", { items: OVERVIEW_POPULATED.nodes });
}

async function visit(page, path, holdMs) {
  log(`  scene: ${path || "/"}`);
  await page.goto(`${BASE_URL}/${path}`);
  await page.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => {});
  await page.waitForTimeout(holdMs);
}

async function main() {
  mkdirSync(OUT_DIR, { recursive: true });
  if (!existsSync(join(DIST_DIR, "index.html"))) {
    log("web/dist missing — building…");
    execSync("npm run build", { cwd: WEB_DIR, stdio: "inherit" });
  }
  let preview = null;
  if (!(await isPortInUse(PORT))) {
    log(`starting vite preview on ${BASE_URL}…`);
    preview = spawn("npm", ["run", "preview", "--", "--port", String(PORT), "--host", "127.0.0.1"],
      { cwd: WEB_DIR, stdio: ["ignore", "ignore", "inherit"] });
    await waitForUrl(`${BASE_URL}/`, 30_000);
  }
  const browser = await chromium.launch({ headless: true, args: ["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"] });
  try {
    const ctx = await browser.newContext({ viewport: VIEWPORT, deviceScaleFactor: 1, colorScheme: "dark", recordVideo: { dir: OUT_DIR, size: VIEWPORT } });
    const page = await ctx.newPage();
    await installMocks(page);

    // Walkthrough (~90s) mirroring demo-video-script.md scenes.
    await visit(page, "", 5000);            // 1. Live dashboard
    await visit(page, "ingest", 5000);      // 2. Ingest health
    await visit(page, "alerts", 6000);      // 3. Alerting (rules + firing history)
    await visit(page, "qoe", 6000);         // 4. Viewer QoE (beacon)
    await visit(page, "analytics", 7000);   // 5. 13-month analytics
    await visit(page, "reports", 5000);     // 6. Usage / billing reports
    await visit(page, "", 3000);            // close on the dashboard

    const video = page.video();
    await ctx.close(); // flushes the recording
    if (video) {
      const raw = await video.path();
      renameSync(raw, VIDEO_PATH);
      log(`saved ${VIDEO_PATH}`);
    } else {
      const webm = readdirSync(OUT_DIR).find((f) => f.endsWith(".webm"));
      if (webm) { renameSync(join(OUT_DIR, webm), VIDEO_PATH); log(`saved ${VIDEO_PATH}`); }
    }
  } finally {
    await browser.close();
    if (preview) preview.kill("SIGTERM");
  }
  log("done — rough cut ready for the operator to re-record with voiceover.");
}

main().catch((err) => { console.error("[demo-video] FATAL:", err); process.exit(1); });
