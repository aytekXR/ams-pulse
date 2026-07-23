/**
 * Marketplace + User-Guide screenshot capture script.
 *
 * Captures the live React UI (vite preview, no backend) with route-mocked API
 * calls — the same technique used by web/e2e/*.spec.ts.
 *
 * Usage:
 *   node qa/marketplace/capture-live-screenshots.mjs
 *
 * Output:
 *   docs/marketplace/screenshots/  (gitignored — safe to write PNGs there)
 *
 * Self-contained: starts vite preview, captures, then stops it.
 */

import pkg from "/home/aytek/repo/ams-pulse/web/node_modules/@playwright/test/index.js";
const { chromium } = pkg;
import { spawn, execSync } from "child_process";
import { existsSync, mkdirSync } from "fs";
import { join, dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { createServer } from "net";

// ── Paths ────────────────────────────────────────────────────────────────────

const __dirname = dirname(fileURLToPath(import.meta.url));

// ── Bootstrap: ensure libatk / libcups etc. are available for the headless ──
// shell. On Ubuntu 24.04 these may not be installed system-wide. We extract
// the .deb packages to a local dir and prepend it to LD_LIBRARY_PATH so that
// Playwright's chromium_headless_shell can load them.  The extraction runs
// only when the target directory is missing.
{
  const LOCAL_LIBS = "/tmp/pw-locallibs";
  const MARKER = `${LOCAL_LIBS}/.extracted`;
  const ATK_SO = `${LOCAL_LIBS}/usr/lib/x86_64-linux-gnu/libatk-1.0.so.0`;
  const { existsSync: exists, mkdirSync } = await import("fs");
  const { join } = await import("path");
  const { execSync } = await import("child_process");

  if (!exists(MARKER) || !exists(ATK_SO)) {
    try {
      console.log("[capture] Extracting GTK/ATK libs for headless Chromium...");
      mkdirSync(LOCAL_LIBS, { recursive: true });
      const DL_DIR = "/tmp/pw-aptdebs";
      mkdirSync(DL_DIR, { recursive: true });
      execSync(
        `cd "${DL_DIR}" && apt-get download ` +
        `libatk1.0-0t64 libatk-bridge2.0-0t64 libatspi2.0-0t64 ` +
        `libcups2t64 libasound2t64 libgbm1 libdrm2 ` +
        `libcairo2 libpango-1.0-0 libpangocairo-1.0-0 ` +
        `libxcomposite1 libxdamage1 libxfixes3 libxkbcommon0 libxrandr2 2>&1`,
        { stdio: "inherit" },
      );
      execSync(
        `for deb in "${DL_DIR}"/*.deb; do dpkg-deb -x "$deb" "${LOCAL_LIBS}/"; done`,
        { shell: "/bin/bash", stdio: "inherit" },
      );
      execSync(`touch "${MARKER}"`);
    } catch (e) {
      console.warn("[capture] lib extraction failed (will try anyway):", e.message);
    }
  }

  // Prepend to LD_LIBRARY_PATH so the chromium sub-process finds the libs
  const libPath = `${LOCAL_LIBS}/usr/lib/x86_64-linux-gnu`;
  process.env.LD_LIBRARY_PATH = process.env.LD_LIBRARY_PATH
    ? `${libPath}:${process.env.LD_LIBRARY_PATH}`
    : libPath;
}
const REPO_ROOT = resolve(__dirname, "../..");
const WEB_DIR = join(REPO_ROOT, "web");
const DIST_DIR = join(WEB_DIR, "dist");
const OUT_DIR = join(REPO_ROOT, "docs/marketplace/screenshots");

const PORT = 4173;
const BASE_URL = `http://127.0.0.1:${PORT}`;

// ── Helpers ──────────────────────────────────────────────────────────────────

function log(msg) {
  console.log(`[capture] ${msg}`);
}

function isPortInUse(port) {
  return new Promise((resolve) => {
    const s = createServer();
    s.once("error", () => resolve(true));
    s.once("listening", () => { s.close(); resolve(false); });
    s.listen(port, "127.0.0.1");
  });
}

async function waitForUrl(url, timeoutMs = 30_000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(2000) });
      if (res.ok || res.status < 500) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`Timed out waiting for ${url}`);
}

// ── Mock payloads ─────────────────────────────────────────────────────────────

const NOW = Date.now();
const HOUR = 3_600_000;

/** License payload factory — used to gate features per page */
function license(tier = "enterprise") {
  const limits = {
    free:       { max_nodes: 1,    retention_days: 7,   data_api: false, white_label: false },
    pro:        { max_nodes: 10,   retention_days: 90,  data_api: true,  white_label: false },
    business:   { max_nodes: 50,   retention_days: 365, data_api: true,  white_label: true  },
    enterprise: { max_nodes: null, retention_days: 365, data_api: true,  white_label: true  },
  }[tier];
  return { tier, valid: true, expires_at: null, offline_file: false, limits: { ...limits, max_streams: null } };
}

const AUTH_ME     = { name: "admin", role: "admin", auth_method: "token" };
const OIDC_OFF    = { enabled: false };
const HEALTHZ_OK  = { status: "ok", ams_env_configured: true };
const HEALTHZ_NEW = { status: "ok", ams_env_configured: false };

const SOURCES_ONE = {
  items: [{ id: "src-1", name: "Origin AMS", type: "rest_poll", rest_url: "http://10.0.0.1:5080", created_at: NOW - 86_400_000 }],
};
const TOKENS_BODY = { items: [] };

// Dashboard — 8 streams, non-zero viewers/publishers/CPU
const OVERVIEW_POPULATED = {
  ts: NOW,
  total_viewers: 847,
  total_publishers: 12,
  nodes: [
    { node_id: "origin-1", role: "origin", status: "up", cpu_pct: 38, mem_pct: 61, net_out_mbps: 312 },
    { node_id: "edge-us-1", role: "edge",   status: "up", cpu_pct: 22, mem_pct: 44, net_out_mbps: 188 },
    { node_id: "edge-eu-1", role: "edge",   status: "up", cpu_pct: 19, mem_pct: 38, net_out_mbps: 141 },
  ],
  protocol_mix: { webrtc: 5, hls: 4, rtmp: 2, dash: 1, other: 0 },
  apps: [
    { app: "live",     stream_count: 5, viewer_count: 612 },
    { app: "webrtc",   stream_count: 2, viewer_count: 180 },
    { app: "vod",      stream_count: 1, viewer_count:  55 },
  ],
};

const STREAMS_POPULATED = {
  items: [
    { stream_id: "live/main-broadcast",  app: "live",   node_id: "origin-1", viewers: 312, bitrate_kbps: 4800, fps: 30, uptime_s: 7200, publisher_ip: "203.0.113.10" },
    { stream_id: "live/gaming-channel",  app: "live",   node_id: "origin-1", viewers: 189, bitrate_kbps: 6000, fps: 60, uptime_s: 3600, publisher_ip: "203.0.113.11" },
    { stream_id: "live/sports-coverage", app: "live",   node_id: "edge-us-1", viewers: 111, bitrate_kbps: 3200, fps: 30, uptime_s: 1800, publisher_ip: "203.0.113.12" },
    { stream_id: "live/news-live",       app: "live",   node_id: "origin-1", viewers:  88, bitrate_kbps: 2500, fps: 25, uptime_s: 5400, publisher_ip: "203.0.113.13" },
    { stream_id: "live/concert-stream",  app: "live",   node_id: "edge-eu-1", viewers:  72, bitrate_kbps: 5600, fps: 30, uptime_s: 2700, publisher_ip: "203.0.113.14" },
    { stream_id: "webrtc/conf-a",        app: "webrtc", node_id: "origin-1", viewers:  44, bitrate_kbps: 1800, fps: 30, uptime_s:  900, publisher_ip: "203.0.113.15" },
    { stream_id: "webrtc/conf-b",        app: "webrtc", node_id: "origin-1", viewers:  18, bitrate_kbps: 1200, fps: 24, uptime_s:  600, publisher_ip: "203.0.113.16" },
    { stream_id: "vod/replay-1",         app: "vod",    node_id: "edge-us-1", viewers:  13, bitrate_kbps: 3800, fps: 30, uptime_s: 9000, publisher_ip: "203.0.113.17" },
  ],
  meta: { total: 8, has_more: false, next_cursor: null },
};

// Ingest — one stream with timeseries, health=95
const INGEST_DATA = {
  streams: [
    {
      stream_id: "live/main-broadcast",
      app: "live",
      node_id: "origin-1",
      health_score: 95,
      timeseries: [
        { ts: NOW - 300_000, bitrate_kbps: 4750, fps: 30, packet_loss_pct: 0.1, jitter_ms: 3 },
        { ts: NOW - 240_000, bitrate_kbps: 4800, fps: 30, packet_loss_pct: 0.0, jitter_ms: 2 },
        { ts: NOW - 180_000, bitrate_kbps: 4780, fps: 29, packet_loss_pct: 0.2, jitter_ms: 4 },
        { ts: NOW - 120_000, bitrate_kbps: 4820, fps: 30, packet_loss_pct: 0.1, jitter_ms: 3 },
        { ts: NOW -  60_000, bitrate_kbps: 4800, fps: 30, packet_loss_pct: 0.0, jitter_ms: 2 },
      ],
      drop_events: [],
    },
    {
      stream_id: "live/gaming-channel",
      app: "live",
      node_id: "origin-1",
      health_score: 82,
      timeseries: [
        { ts: NOW - 300_000, bitrate_kbps: 5900, fps: 59, packet_loss_pct: 0.5, jitter_ms: 8 },
        { ts: NOW -  60_000, bitrate_kbps: 6000, fps: 60, packet_loss_pct: 0.3, jitter_ms: 5 },
      ],
      drop_events: [{ ts: NOW - 200_000, duration_ms: 120 }],
    },
    {
      stream_id: "live/sports-coverage",
      app: "live",
      node_id: "edge-us-1",
      health_score: 78,
      timeseries: [
        { ts: NOW - 120_000, bitrate_kbps: 3100, fps: 29, packet_loss_pct: 1.2, jitter_ms: 12 },
        { ts: NOW -  60_000, bitrate_kbps: 3200, fps: 30, packet_loss_pct: 0.8, jitter_ms: 9 },
      ],
      drop_events: [],
    },
  ],
};

// Alerts — 4 rules, 1 firing, with history
const ALERT_RULES = {
  items: [
    { id: "r1", name: "CPU Critical",          metric: "cpu_pct",       operator: "gt", threshold: 90,   window_s: 300, severity: "critical", cooldown_s: 600,  enabled: true,  muted: false, created_at: NOW - 86_400_000, rule_type: "threshold", sigma: 4.0, min_samples: 30 },
    { id: "r2", name: "High Viewer Drop",       metric: "viewer_count",  operator: "lt", threshold: 10,   window_s: 120, severity: "warning",  cooldown_s: 300,  enabled: true,  muted: false, created_at: NOW - 86_400_000, rule_type: "threshold", sigma: 4.0, min_samples: 30 },
    { id: "r3", name: "Packet Loss Spike",      metric: "packet_loss",   operator: "gt", threshold: 2,    window_s: 60,  severity: "warning",  cooldown_s: 300,  enabled: true,  muted: false, created_at: NOW - 86_400_000, rule_type: "threshold", sigma: 4.0, min_samples: 30 },
    { id: "r4", name: "Bitrate Anomaly",        metric: "bitrate_kbps",  operator: "lt", threshold: 1000, window_s: 180, severity: "info",     cooldown_s: 900,  enabled: true,  muted: false, created_at: NOW - 86_400_000, rule_type: "anomaly",   sigma: 3.0, min_samples: 20 },
  ],
};
const ALERT_CHANNELS = {
  items: [
    { id: "ch1", name: "Ops Slack",    type: "slack",   config: { webhook_url: "https://hooks.slack.com/x" }, enabled: true },
    { id: "ch2", name: "PagerDuty",    type: "webhook", config: { url: "https://pd.example.com/x" },          enabled: true },
  ],
};
const ALERT_HISTORY = {
  items: [
    { id: "h1", rule_id: "r1", rule_name: "CPU Critical",    severity: "critical", fired_at: NOW - 1800_000, resolved_at: NOW - 1200_000, node_id: "origin-1" },
    { id: "h2", rule_id: "r3", rule_name: "Packet Loss Spike", severity: "warning", fired_at: NOW -  900_000, resolved_at: null,           node_id: "edge-us-1" },
  ],
};

// Analytics — audience tab with line charts + totals
const ANALYTICS_AUDIENCE = {
  totals: { views: 28_450, uniques: 9_812, watch_time_s: 892_800, peak_concurrency: 847 },
  timeseries: Array.from({ length: 14 }, (_, i) => ({
    ts: NOW - (13 - i) * 24 * HOUR,
    views:            1800 + Math.round(Math.sin(i * 0.7) * 400) + i * 80,
    uniques:           620 + Math.round(Math.cos(i * 0.5) * 150) + i * 28,
    watch_time_s:    63_000 + i * 3_000,
    peak_concurrency:  500 + Math.round(Math.sin(i * 0.9) * 120) + i * 20,
  })),
};
const ANALYTICS_GEO = {
  rows: [
    { country: "US", views: 11_200, uniques: 3_900, watch_time_s: 360_000 },
    { country: "TR", views:  5_400, uniques: 1_800, watch_time_s: 172_800 },
    { country: "DE", views:  3_900, uniques: 1_350, watch_time_s: 124_200 },
    { country: "GB", views:  2_800, uniques:   980, watch_time_s:  90_000 },
    { country: "FR", views:  2_150, uniques:   740, watch_time_s:  69_000 },
  ],
};
const ANALYTICS_DEVICES = {
  rows: [
    { device: "desktop", views: 14_200, watch_time_s: 450_000 },
    { device: "mobile",  views:  9_800, watch_time_s: 312_000 },
    { device: "tablet",  views:  4_450, watch_time_s: 130_800 },
  ],
};

// Reports — business tier, usage table populated
const REPORTS_USAGE = {
  rows: [
    { app: "live",   period: "2026-07", viewer_minutes: 482_000, peak_concurrency: 847, egress_gb: 2_840, recording_gb: 380 },
    { app: "webrtc", period: "2026-07", viewer_minutes: 148_000, peak_concurrency: 212, egress_gb:   880, recording_gb:   0 },
    { app: "vod",    period: "2026-07", viewer_minutes:  62_000, peak_concurrency:  55, egress_gb:   410, recording_gb: 120 },
  ],
  totals: { viewer_minutes: 692_000, peak_concurrency: 847, egress_gb: 4_130, recording_gb: 500 },
  egress_method: "bitrate_x_watch_time",
};
const REPORTS_SCHEDULES = { items: [], meta: { total: 0, next_cursor: null } };
const REPORTS_TENANTS   = { items: [], meta: { total: 0, next_cursor: null } };

// Fleet — 3 nodes, card view
const FLEET_NODES = {
  items: [
    { node_id: "origin-1",  role: "origin", status: "up",       last_seen: NOW, version: "2.10.1", cpu_pct: 38, mem_pct: 61, net_in_mbps: 92.4, net_out_mbps: 312.8 },
    { node_id: "edge-us-1", role: "edge",   status: "up",       last_seen: NOW, version: "2.10.0", cpu_pct: 22, mem_pct: 44, net_in_mbps: 31.2, net_out_mbps: 188.3 },
    { node_id: "edge-eu-1", role: "edge",   status: "degraded", last_seen: NOW, version: "2.9.3",  cpu_pct: 79, mem_pct: 88, net_in_mbps: 18.6, net_out_mbps: 141.0 },
  ],
  meta: { total: 3 },
};

// Anomalies — enterprise, 3 rows
const ANOMALIES_DATA = {
  items: [
    { id: "f1", metric: "viewers",     scope: { node_id: "origin-1",  app: "live",   stream_id: null         }, observed: 847,  expected: 320,  sigma: 5.1, ts: NOW -  60_000 },
    { id: "f2", metric: "error_rate",  scope: { node_id: null,         app: "live",   stream_id: "live/main"  }, observed: 0.18, expected: 0.02, sigma: 4.2, ts: NOW - 120_000 },
    { id: "f3", metric: "bitrate_kbps",scope: { node_id: "edge-us-1", app: "live",   stream_id: "live/sports"}, observed: 890,  expected: 3200, sigma: 3.8, ts: NOW - 300_000 },
  ],
  meta: { total: 3 },
};

// Probes — pro+ tier, 3 probes
const PROBES_DATA = {
  items: [
    {
      id: "p1", name: "Main HLS Probe",   url: "https://cdn.example.com/live/main.m3u8",   protocol: "hls", interval_s: 60, timeout_s: 10, enabled: true, created_at: NOW - 86_400_000,
      last_result: { id: "pr1", probe_id: "p1", ts: NOW - 45_000, success: true,  ttfb_ms: 142, bitrate_kbps: 4800 },
    },
    {
      id: "p2", name: "WebRTC Origin",    url: "wss://origin.example.com/ws",              protocol: "webrtc", interval_s: 30, timeout_s: 5,  enabled: true, created_at: NOW - 43_200_000,
      last_result: { id: "pr2", probe_id: "p2", ts: NOW - 20_000, success: true,  ttfb_ms:  98, bitrate_kbps: 1800 },
    },
    {
      id: "p3", name: "Edge EU DASH",     url: "https://edge-eu.example.com/live/main.mpd",protocol: "dash",  interval_s: 60, timeout_s: 10, enabled: true, created_at: NOW - 21_600_000,
      last_result: { id: "pr3", probe_id: "p3", ts: NOW - 55_000, success: false, ttfb_ms: null,bitrate_kbps: null, error: "timeout" },
    },
  ],
  meta: { total: 3 },
};

// Audit log — with rows
const AUDIT_LOG_DATA = {
  items: [
    { id: "a1", ts: NOW -  60_000, actor_name: "admin",          actor_token_id: "tok-abc123", action: "alert_rule.create",  object_type: "alert_rule",  object_id: "r1",    remote_addr: "10.0.0.5",  detail: { name: "CPU Critical" } },
    { id: "a2", ts: NOW - 120_000, actor_name: "ops-token",      actor_token_id: "tok-def456", action: "source.update",      object_type: "source",      object_id: "src-1", remote_addr: "10.0.0.6",  detail: {} },
    { id: "a3", ts: NOW - 300_000, actor_name: "admin",          actor_token_id: "tok-abc123", action: "license.activate",   object_type: "license",     object_id: "",      remote_addr: "10.0.0.5",  detail: { tier: "enterprise" } },
    { id: "a4", ts: NOW - 600_000, actor_name: "api-token",      actor_token_id: "tok-ghi789", action: "token.create",       object_type: "token",       object_id: "tok-X", remote_addr: "10.0.0.7",  detail: {} },
    { id: "a5", ts: NOW - 900_000, actor_name: "admin",          actor_token_id: "tok-abc123", action: "alert_rule.delete",  object_type: "alert_rule",  object_id: "r0",    remote_addr: "10.0.0.5",  detail: {} },
  ],
  meta: { next_cursor: null },
};

// QoE summary
const QOE_SUMMARY = {
  totals: { startup_p50_ms: 380, startup_p95_ms: 920, rebuffer_ratio: 0.012, error_rate: 0.008 },
  bitrate_timeline: Array.from({ length: 12 }, (_, i) => ({
    ts: NOW - (11 - i) * HOUR,
    bitrate_kbps_p50: 3800 + Math.round(Math.sin(i * 0.6) * 400),
    bitrate_kbps_p95: 5400 + Math.round(Math.cos(i * 0.4) * 600),
  })),
};

// ── Install boot-layer mocks on a page ───────────────────────────────────────

async function stubBoot(page, { tier = "enterprise", token = "plt_marketplace_test", healthzConfigured = true } = {}) {
  // Seed auth token before page scripts run
  await page.addInitScript(
    ([k, v]) => { localStorage.setItem(k, v); },
    ["pulse_token", token],
  );

  await page.route("**/api/v1/admin/license", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(license(tier)) })
  );
  await page.route("**/auth/me", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AUTH_ME) })
  );
  await page.route("**/auth/oidc/status", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(OIDC_OFF) })
  );
  await page.route("**/healthz", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(healthzConfigured ? HEALTHZ_OK : HEALTHZ_NEW) })
  );
  await page.route("**/api/v1/admin/sources", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(SOURCES_ONE) })
  );
  await page.route("**/api/v1/admin/tokens", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(TOKENS_BODY) })
  );
}

function json(body, status = 200) {
  return { status, contentType: "application/json", body: JSON.stringify(body) };
}

// ── Screenshot helper ─────────────────────────────────────────────────────────

async function shot(page, outPath, { waitFor = null, timeout = 15_000 } = {}) {
  // Always wait for network-idle first
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  if (waitFor) {
    // waitFor can be a CSS selector or a function
    if (typeof waitFor === "string") {
      await page.waitForSelector(waitFor, { timeout: 10_000 }).catch(() => {});
    } else if (typeof waitFor === "function") {
      await waitFor(page).catch(() => {});
    }
  }
  // Extra settle time for charts/animations
  await page.waitForTimeout(800);
  await page.screenshot({ path: outPath, fullPage: false });
  const st = (await import("fs")).statSync(outPath);
  log(`  saved ${outPath.split("/").pop()} (${(st.size / 1024).toFixed(1)} KB)`);
}

// ── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  // 1. Ensure output directory
  mkdirSync(OUT_DIR, { recursive: true });
  log(`Output: ${OUT_DIR}`);

  // 2. Build if dist is missing
  if (!existsSync(join(DIST_DIR, "index.html"))) {
    log("web/dist not found — building...");
    execSync("npm run build", { cwd: WEB_DIR, stdio: "inherit" });
  } else {
    log("web/dist found — skipping build");
  }

  // 3. Start vite preview if not already running
  let previewProc = null;
  const alreadyUp = await isPortInUse(PORT);
  if (alreadyUp) {
    log(`Port ${PORT} already in use — reusing running server`);
  } else {
    log(`Starting vite preview on ${BASE_URL}...`);
    previewProc = spawn(
      "npm",
      ["run", "preview", "--", "--port", String(PORT), "--host", "127.0.0.1"],
      { cwd: WEB_DIR, stdio: ["ignore", "pipe", "pipe"], detached: false },
    );
    previewProc.stdout.on("data", (d) => process.stdout.write(`[vite] ${d}`));
    previewProc.stderr.on("data", (d) => process.stderr.write(`[vite] ${d}`));
    await waitForUrl(`${BASE_URL}/`, 30_000);
    log("vite preview ready");
  }

  // 4. Launch browser
  const browser = await chromium.launch({ headless: true });

  try {
    const viewport = { width: 1920, height: 1080 };
    const deviceScaleFactor = 1;

    // ── Helper: new page with dark theme (default) ────────────────────────
    async function newPage(tier = "enterprise", themeOverride = null) {
      const ctx = await browser.newContext({ viewport, deviceScaleFactor });
      const page = await ctx.newPage();
      if (themeOverride) {
        await page.addInitScript(
          ([k, v]) => localStorage.setItem(k, v),
          ["pulse_theme", themeOverride],
        );
      }
      await stubBoot(page, { tier });
      return { page, ctx };
    }

    // ══════════════════════════════════════════════════════════════════════
    //  LISTING SET
    // ══════════════════════════════════════════════════════════════════════

    // ss1-dashboard.png — route /, populated with ~8 streams, non-zero stats
    {
      log("Capturing ss1-dashboard.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/live/overview", (r) => r.fulfill(json(OVERVIEW_POPULATED)));
      await page.route("**/api/v1/live/streams**",  (r) => r.fulfill(json(STREAMS_POPULATED)));
      await page.goto(`${BASE_URL}/`);
      await shot(page, join(OUT_DIR, "ss1-dashboard.png"), {
        waitFor: 'h1, [data-testid="stat-card"], .recharts-wrapper',
      });
      await ctx.close();
    }

    // ss2-ingest-health.png — /ingest with detail panel open
    {
      log("Capturing ss2-ingest-health.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/qoe/ingest**", (r) => r.fulfill(json(INGEST_DATA)));
      await page.goto(`${BASE_URL}/ingest`);
      await page.waitForLoadState("networkidle").catch(() => {});
      // Open first stream's detail panel
      await page.waitForSelector('button:has-text("Details")', { timeout: 8_000 }).catch(() => {});
      const btn = page.locator('button:has-text("Details")').first();
      if (await btn.count() > 0) await btn.click();
      await shot(page, join(OUT_DIR, "ss2-ingest-health.png"), {
        waitFor: 'h1, .recharts-wrapper',
      });
      await ctx.close();
    }

    // ss3-alerting.png — /alerts Rules tab with 3-4 rules + History firing badge
    {
      log("Capturing ss3-alerting.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/alerts/rules**",   (r) => r.fulfill(json(ALERT_RULES)));
      await page.route("**/api/v1/alerts/channels**",(r) => r.fulfill(json(ALERT_CHANNELS)));
      await page.route("**/api/v1/alerts/history**", (r) => r.fulfill(json(ALERT_HISTORY)));
      await page.goto(`${BASE_URL}/alerts`);
      await shot(page, join(OUT_DIR, "ss3-alerting.png"), {
        waitFor: '[role="tabpanel"]',
      });
      await ctx.close();
    }

    // ss4-analytics.png — /analytics Audience tab with line charts + totals
    {
      log("Capturing ss4-analytics.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/analytics/audience**",(r) => r.fulfill(json(ANALYTICS_AUDIENCE)));
      await page.route("**/api/v1/analytics/geo**",     (r) => r.fulfill(json(ANALYTICS_GEO)));
      await page.route("**/api/v1/analytics/devices**", (r) => r.fulfill(json(ANALYTICS_DEVICES)));
      await page.goto(`${BASE_URL}/analytics`);
      await shot(page, join(OUT_DIR, "ss4-analytics.png"), {
        waitFor: '.recharts-wrapper',
      });
      await ctx.close();
    }

    // ss5-reports.png — /reports Usage tab, business tier, populated usage table
    {
      log("Capturing ss5-reports.png …");
      const { page, ctx } = await newPage("business");
      await page.route("**/api/v1/reports/usage**",    (r) => r.fulfill(json(REPORTS_USAGE)));
      await page.route("**/api/v1/reports/schedules**",(r) => r.fulfill(json(REPORTS_SCHEDULES)));
      await page.route("**/api/v1/admin/tenants**",    (r) => r.fulfill(json(REPORTS_TENANTS)));
      await page.goto(`${BASE_URL}/reports`);
      await shot(page, join(OUT_DIR, "ss5-reports.png"), {
        waitFor: '[role="tablist"]',
      });
      await ctx.close();
    }

    // ss6-probes.png — /probes with 3 probes, pro+ tier
    {
      log("Capturing ss6-probes.png …");
      const { page, ctx } = await newPage("pro");
      await page.route(/\/api\/v1\/probes/, (r) => r.fulfill(json(PROBES_DATA)));
      await page.goto(`${BASE_URL}/probes`);
      await shot(page, join(OUT_DIR, "ss6-probes.png"), {
        waitFor: '[role="table"]',
      });
      await ctx.close();
    }

    // ══════════════════════════════════════════════════════════════════════
    //  USER-GUIDE SET
    // ══════════════════════════════════════════════════════════════════════

    // ug-qoe.png — /qoe populated
    {
      log("Capturing ug-qoe.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/qoe/summary**", (r) => r.fulfill(json(QOE_SUMMARY)));
      await page.goto(`${BASE_URL}/qoe`);
      await shot(page, join(OUT_DIR, "ug-qoe.png"), {
        waitFor: '.recharts-wrapper',
      });
      await ctx.close();
    }

    // ug-fleet.png — /fleet with 3 nodes, card view
    {
      log("Capturing ug-fleet.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/fleet/nodes**", (r) => r.fulfill(json(FLEET_NODES)));
      await page.goto(`${BASE_URL}/fleet`);
      await shot(page, join(OUT_DIR, "ug-fleet.png"), {
        waitFor: async (p) => p.waitForSelector('[role="radiogroup"]', { timeout: 8_000 }),
      });
      await ctx.close();
    }

    // ug-anomalies.png — /anomalies with 3 rows, enterprise tier
    {
      log("Capturing ug-anomalies.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/anomalies**", (r) => r.fulfill(json(ANOMALIES_DATA)));
      await page.goto(`${BASE_URL}/anomalies`);
      await shot(page, join(OUT_DIR, "ug-anomalies.png"), {
        waitFor: '[role="table"]',
      });
      await ctx.close();
    }

    // ug-audit-log.png — /audit-log with rows
    {
      log("Capturing ug-audit-log.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.route("**/api/v1/admin/audit-log**", (r) => r.fulfill(json(AUDIT_LOG_DATA)));
      await page.goto(`${BASE_URL}/audit-log`);
      await shot(page, join(OUT_DIR, "ug-audit-log.png"), {
        waitFor: '[role="table"]',
      });
      await ctx.close();
    }

    // ug-settings-sources.png — /settings Sources tab
    {
      log("Capturing ug-settings-sources.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.goto(`${BASE_URL}/settings`);
      await shot(page, join(OUT_DIR, "ug-settings-sources.png"), {
        waitFor: '[role="tabpanel"]',
      });
      await ctx.close();
    }

    // ug-settings-license.png — /settings License tab with tier + limits
    {
      log("Capturing ug-settings-license.png …");
      const { page, ctx } = await newPage("enterprise");
      await page.goto(`${BASE_URL}/settings`);
      await page.waitForLoadState("networkidle").catch(() => {});
      await page.waitForSelector('[role="tablist"]', { timeout: 8_000 }).catch(() => {});
      const licTab = page.getByRole("tab", { name: "License" });
      if (await licTab.count() > 0) await licTab.click();
      await shot(page, join(OUT_DIR, "ug-settings-license.png"), {
        waitFor: '[role="tabpanel"]',
      });
      await ctx.close();
    }

    // ug-login.png — AuthGate card (NO token injected, separate context)
    {
      log("Capturing ug-login.png …");
      const ctx = await browser.newContext({ viewport, deviceScaleFactor });
      const page = await ctx.newPage();
      // Do NOT inject token — we want the login gate
      await page.route("**/auth/oidc/status", (r) =>
        r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(OIDC_OFF) })
      );
      await page.route("**/api/v1/admin/license", (r) =>
        r.fulfill(json(license("enterprise")))
      );
      await page.goto(`${BASE_URL}/`);
      await shot(page, join(OUT_DIR, "ug-login.png"), {
        waitFor: 'button:has-text("Sign in")',
      });
      await ctx.close();
    }

    // ug-onboarding-step2.png — /onboarding wizard, Add-AMS-source step
    // (healthz returns ams_env_configured: false so the guard fires; but we
    //  navigate directly to /onboarding to skip the redirect race)
    {
      log("Capturing ug-onboarding-step2.png …");
      const ctx = await browser.newContext({ viewport, deviceScaleFactor });
      const page = await ctx.newPage();
      // Inject token so AuthGate passes, but healthz says no AMS configured
      await page.addInitScript(
        ([k, v]) => localStorage.setItem(k, v),
        ["pulse_token", "plt_marketplace_test"],
      );
      await page.route("**/api/v1/admin/license", (r) => r.fulfill(json(license("enterprise"))));
      await page.route("**/auth/me",         (r) => r.fulfill(json(AUTH_ME)));
      await page.route("**/auth/oidc/status",(r) => r.fulfill(json(OIDC_OFF)));
      await page.route("**/healthz",         (r) => r.fulfill(json(HEALTHZ_NEW)));
      await page.route("**/api/v1/admin/sources", (r) => r.fulfill(json({ items: [] })));
      await page.route("**/api/v1/admin/tokens",  (r) => r.fulfill(json(TOKENS_BODY)));
      // Navigate directly to onboarding (step: welcome)
      await page.goto(`${BASE_URL}/onboarding`);
      await page.waitForLoadState("networkidle").catch(() => {});
      // Advance to "source" step by clicking "Get Started" / "Next" / first CTA
      await page.waitForTimeout(500);
      const startBtn = page.locator('button').filter({ hasText: /get started|next|add source|begin/i }).first();
      if (await startBtn.count() > 0) await startBtn.click();
      await page.waitForTimeout(600);
      await shot(page, join(OUT_DIR, "ug-onboarding-step2.png"), {
        waitFor: 'input, form, [role="form"]',
      });
      await ctx.close();
    }

    // ss1-light.png — dashboard in light theme
    {
      log("Capturing ss1-light.png …");
      const ctx = await browser.newContext({ viewport, deviceScaleFactor });
      const page = await ctx.newPage();
      await page.addInitScript(
        ([k, v]) => localStorage.setItem(k, v),
        ["pulse_theme", "light"],
      );
      await stubBoot(page, { tier: "enterprise" });
      await page.route("**/api/v1/live/overview", (r) => r.fulfill(json(OVERVIEW_POPULATED)));
      await page.route("**/api/v1/live/streams**",  (r) => r.fulfill(json(STREAMS_POPULATED)));
      await page.goto(`${BASE_URL}/`);
      await shot(page, join(OUT_DIR, "ss1-light.png"), {
        waitFor: 'h1, [data-testid="stat-card"], .recharts-wrapper',
      });
      await ctx.close();
    }

    log("\nAll screenshots captured successfully.");

  } finally {
    await browser.close();
    if (previewProc) {
      previewProc.kill("SIGTERM");
      log("vite preview stopped");
    }
  }
}

main().catch((err) => {
  console.error("[capture] FATAL:", err);
  process.exit(1);
});
