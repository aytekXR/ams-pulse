#!/usr/bin/env node
/**
 * Capture marketplace screenshots from a RUNNING Pulse (the demo stack in
 * qa/marketplace/demo-stack, or any real deployment you point it at). Unlike
 * capture-live-screenshots.mjs, nothing is route-mocked: every pixel comes from the
 * real backend's responses.
 *
 *   node qa/marketplace/capture-real-stack.mjs [--only name,name] [--out dir]
 *        [--base http://127.0.0.1:18190] [--token-file path] [--scale 1] [--theme dark]
 *
 * Shots are listed in SHOTS below. Theme is pinned BOTH ways (Playwright colorScheme
 * and the app's `pulse_theme` key) and asserted before every capture — an unpinned
 * context renders light and nothing errors (D-172). After capturing, open each PNG
 * and read every panel: an absent field renders as "—" or 0 without failing.
 */

import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../..");
const require = createRequire(import.meta.url);
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const args = Object.fromEntries(
  process.argv.slice(2).reduce((acc, a, i, all) => (a.startsWith("--") ? [...acc, [a.slice(2), all[i + 1]]] : acc), []),
);
const BASE = args.base || "http://127.0.0.1:18190";
const MAILPIT = args.mailpit || "http://127.0.0.1:18125";
const OUT = resolve(args.out || join(REPO, "docs/marketplace/antmedia-submission/assets/screenshots"));
const TOKEN = readFileSync(args["token-file"] || join(HERE, "demo-stack/.state/admin-token"), "utf8").trim();
const SCALE = Number(args.scale || 1);
const THEME = args.theme || "dark";
const ONLY = args.only ? new Set(args.only.split(",")) : null;
mkdirSync(OUT, { recursive: true });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function settle(page, selector, ms = 1200) {
  await page.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => {});
  if (selector) await page.waitForSelector(selector, { timeout: 15_000 });
  await page.evaluate(() => document.fonts.ready);
  await sleep(ms); // let charts finish their (restrained) fade-in
}

async function clickTab(page, label) {
  await page.getByRole("tab", { name: label }).click();
  await settle(page);
}

// name → { path, prep(page), file, auth }
const SHOTS = [
  { name: "login", file: "07-sign-in.png", auth: false, path: "/", prep: (p) => settle(p, "input[type=password]") },
  {
    name: "dashboard", file: "01-live-dashboard.png", path: "/",
    prep: async (p) => {
      await settle(p, "table");
      // The connection badge flips to "Live" once the WebSocket push is up.
      await p.getByText("Live", { exact: true }).first().waitFor({ timeout: 20_000 }).catch(() => {});
      await sleep(2500);
    },
  },
  { name: "alerts-history", file: "02-alert-history-firing.png", path: "/alerts", prep: (p) => settle(p).then(() => clickTab(p, "History")) },
  // Rules render as a list of rows inside the tab panel, not a <table>.
  { name: "alerts-rules", file: "03-alert-rules.png", path: "/alerts", prep: (p) => settle(p, "#panel-rules") },
  { name: "alerts-channels", file: "04-alert-channels.png", path: "/alerts", prep: (p) => settle(p).then(() => clickTab(p, "Channels")) },
  {
    name: "alerts-rule-form", file: "05-alert-rule-editor.png", path: "/alerts",
    prep: async (p) => {
      await settle(p, "#panel-rules");
      // Open a configured rule (more telling than a blank form).
      const row = p.locator("#panel-rules > div > div").filter({ hasText: "Ingest bitrate below 1,500 kbps" }).first();
      await row.getByRole("button", { name: /^edit$/i }).click();
      await settle(p, "#rule-metric");
    },
  },
  { name: "qoe", file: "08-viewer-qoe.png", path: "/qoe", prep: (p) => settle(p, ".recharts-wrapper", 2500) },
  { name: "ingest", file: "09-ingest-health.png", path: "/ingest", prep: (p) => settle(p, "table", 2500) },
  {
    name: "ingest-detail", file: "09b-ingest-stream-detail.png", path: "/ingest",
    prep: async (p) => {
      await settle(p, "table");
      await p.locator("tr").filter({ hasText: args.stream || "studio-b" }).getByRole("button", { name: /details/i }).click();
      await settle(p, ".recharts-wrapper", 2500);
    },
  },
  { name: "analytics", file: "10-audience-analytics.png", path: "/analytics", prep: (p) => settle(p, null, 3000) },
  { name: "fleet", file: "11-fleet.png", path: "/fleet", prep: (p) => settle(p, null, 2000) },
  { name: "probes", file: "12-synthetic-probes.png", path: "/probes", prep: (p) => settle(p, "table", 2000) },
  { name: "reports", file: "13-usage-reports.png", path: "/reports", prep: (p) => settle(p, null, 2500) },
  { name: "anomalies", file: "14-anomalies.png", path: "/anomalies", prep: (p) => settle(p, null, 2000) },
  { name: "settings", file: "15-settings-license.png", path: "/settings", prep: (p) => settle(p).then(() => clickTab(p, "License")) },
  { name: "onboarding", file: "16-onboarding-wizard.png", path: "/onboarding", prep: (p) => settle(p, null, 1500) },
  {
    name: "mailpit", file: "06-alert-email-received.png", external: true,
    prep: async (p) => {
      await p.goto(MAILPIT, { waitUntil: "networkidle" });
      await p.locator(".message").first().click().catch(() => {});
      await p.locator("a.message, .message-list a, #message-page a").first().click().catch(() => {});
      await sleep(2500);
    },
  },
];

const browser = await chromium.launch();
const results = [];
for (const shot of SHOTS) {
  if (ONLY && !ONLY.has(shot.name)) continue;
  const ctx = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: SCALE,
    colorScheme: THEME,
    timezoneId: "UTC",
    locale: "en-US",
  });
  await ctx.addInitScript(
    ([theme, token, auth]) => {
      try {
        localStorage.setItem("pulse_theme", theme);
        localStorage.setItem("pulse_onboarding_dismissed", "1");
        if (auth) localStorage.setItem("pulse_token", token);
        else localStorage.removeItem("pulse_token");
      } catch {
        /* other origins (Mailpit) */
      }
    },
    [THEME, TOKEN, shot.auth !== false],
  );
  const page = await ctx.newPage();
  try {
    if (!shot.external) {
      await page.goto(`${BASE}${shot.path}`, { waitUntil: "domcontentloaded" });
      const theme = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
      if (theme !== THEME) throw new Error(`data-theme is ${theme}, expected ${THEME}`);
    }
    await shot.prep(page);
    const file = join(OUT, shot.file);
    writeFileSync(file, await page.screenshot({ fullPage: false }));
    results.push({ shot: shot.name, file: shot.file, ok: true });
  } catch (err) {
    results.push({ shot: shot.name, file: shot.file, ok: false, error: err.message.split("\n")[0] });
  } finally {
    await ctx.close();
  }
}
await browser.close();
console.table(results);
if (results.some((r) => !r.ok)) process.exitCode = 1;
