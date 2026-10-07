#!/usr/bin/env node
/**
 * Build the marketplace asset sheet (A4 landscape PDF) from the package assets.
 *
 *   node qa/marketplace/tools/build-asset-sheet.mjs <package-dir> <out.pdf>
 *
 * Shows every deliverable image with its file name and caption, the logo set on dark and light
 * backgrounds, and the brand colours/typeface — a one-file overview for Ant Media's web team.
 * Captions are read from marketplace/asset-inventory.md so the two never drift.
 */
import { readFileSync, existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");
const require = createRequire(import.meta.url);
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const [pkg, out] = process.argv.slice(2);
if (!pkg || !out) {
  console.error("usage: build-asset-sheet.mjs <package-dir> <out.pdf>");
  process.exit(2);
}
const A = (p) => join(pkg, "assets", p);
const img = (p) => `data:image/png;base64,${readFileSync(A(p)).toString("base64")}`;
const svg = (p) => readFileSync(A(p), "utf8").replace(/<svg /, '<svg style="width:100%;height:auto" ');
const F = join(REPO, "web/node_modules/@fontsource");
const font = (pkgName, file) => readFileSync(join(F, pkgName, "files", file)).toString("base64");

// Captions from the inventory table rows: | 01 | `screenshots/01-…png` | *caption* |
const inventory = readFileSync(join(pkg, "marketplace/asset-inventory.md"), "utf8");
const caption = (file) => {
  const row = inventory.split("\n").find((l) => l.includes("`" + file) || l.includes(file + "`"));
  const m = row && row.match(/\*([^*]+)\*/);
  return m ? m[1].replace(/`/g, "") : "";
};

const shots = [
  "screenshots/01-live-dashboard.png", "screenshots/02-alert-history.png",
  "screenshots/03-alert-email-delivered.png", "screenshots/04-alert-inbox-fired-and-resolved.png",
  "screenshots/05-alert-rules.png", "screenshots/06-alert-rule-editor.png",
  "screenshots/07-alert-channels.png", "screenshots/08-ingest-health-bitrate-drop.png",
  "screenshots/09-live-dashboard-during-incident.png", "screenshots/10-anomaly-detection.png",
  "screenshots/11-synthetic-probes.png", "screenshots/12-fleet.png",
  "screenshots/13-sign-in.png", "screenshots/14-onboarding-wizard.png",
].filter((f) => existsSync(A(f)));

const tile = (f) => `<figure><img src="${img(f)}"><figcaption><code>${f}</code><span>${caption(f)}</span></figcaption></figure>`;

const html = `<!doctype html><html><head><meta charset="utf-8"><style>
@font-face{font-family:'IBM Plex Sans';font-weight:400;src:url(data:font/woff2;base64,${font("ibm-plex-sans", "ibm-plex-sans-latin-400-normal.woff2")}) format('woff2')}
@font-face{font-family:'IBM Plex Sans';font-weight:600;src:url(data:font/woff2;base64,${font("ibm-plex-sans", "ibm-plex-sans-latin-600-normal.woff2")}) format('woff2')}
@font-face{font-family:'IBM Plex Mono';font-weight:400;src:url(data:font/woff2;base64,${font("ibm-plex-mono", "ibm-plex-mono-latin-400-normal.woff2")}) format('woff2')}
@page{size:A4 landscape;margin:12mm}
*{box-sizing:border-box}
body{margin:0;font-family:'IBM Plex Sans',sans-serif;color:#10181F;font-size:9.5pt}
h1{font-size:20pt;margin:0 0 2mm;letter-spacing:-0.01em}
h2{font-size:12.5pt;margin:0 0 4mm;padding-bottom:2mm;border-bottom:2px solid #087A59}
.sub{color:#4A5B6B;margin:0 0 6mm}
section{break-before:page}
section:first-of-type{break-before:auto}
.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:5mm}
.grid2{display:grid;grid-template-columns:repeat(2,1fr);gap:6mm}
figure{margin:0;break-inside:avoid}
figure img{width:100%;display:block;border:1px solid #D7DEE5;border-radius:3px}
figcaption{margin-top:1.5mm;line-height:1.35}
figcaption code{font-family:'IBM Plex Mono',monospace;font-size:7.4pt;color:#4A5B6B;display:block}
figcaption span{font-size:8.4pt}
.logos{display:grid;grid-template-columns:repeat(2,1fr);gap:6mm}
.bg{border-radius:4px;padding:8mm 10mm;display:flex;align-items:center;justify-content:center;min-height:38mm}
.dark{background:#0A0E14}.light{background:#FFFFFF;border:1px solid #D7DEE5}
.bg .lg{width:60%}.bg .mk{width:22mm}
.swatches{display:flex;gap:4mm;margin-top:6mm}
.sw{flex:1;border-radius:4px;padding:3mm;font-family:'IBM Plex Mono',monospace;font-size:7.5pt;min-height:16mm;display:flex;align-items:flex-end}
.note{color:#4A5B6B;font-size:8.5pt;margin-top:5mm;line-height:1.45}
</style></head><body>
<section>
  <h1>Pulse — marketplace asset sheet</h1>
  <p class="sub">Ant Media Marketplace page assets · real application captures, demo environment (simulated AMS, synthetic viewers) · 2026-10-01</p>
  <h2>Brand mark (product logo — not a company logo)</h2>
  <div class="logos">
    <div class="bg dark"><div class="lg">${svg("branding/svg-outlined/pulse-logo-for-dark-backgrounds-trimmed.svg")}</div></div>
    <div class="bg light"><div class="lg">${svg("branding/svg-outlined/pulse-logo-for-light-backgrounds-trimmed.svg")}</div></div>
    <div class="bg dark"><div class="mk">${svg("branding/svg-outlined/pulse-mark.svg")}</div></div>
    <div class="bg light"><div class="mk">${svg("branding/svg-outlined/pulse-mark-light.svg")}</div></div>
  </div>
  <div class="swatches">
    <div class="sw" style="background:#2CE5A7;color:#0A0E14">signal #2CE5A7</div>
    <div class="sw" style="background:#0A0E14;color:#E8EEF4">base #0A0E14</div>
    <div class="sw" style="background:#10161D;color:#E8EEF4">surface #10161D</div>
    <div class="sw" style="background:#0BA678;color:#FFFFFF">signal on light #0BA678</div>
    <div class="sw" style="background:#FFB224;color:#0A0E14">warning #FFB224</div>
    <div class="sw" style="background:#FF5C68;color:#0A0E14">critical #FF5C68</div>
  </div>
  <p class="note">Files: <code>assets/branding/</code> — SVG (live text), SVG with outlined text (use on the web), transparent PNG at several sizes, "-trimmed" variants without padding. Typeface: IBM Plex Sans (SIL Open Font License).</p>
</section>
<section>
  <h2>Hero and social</h2>
  <div class="grid2">
    ${tile("hero/pulse-hero-alert-1920x1080.png")}
    ${tile("hero/pulse-hero-dashboard-1920x1080.png")}
    ${tile("hero/pulse-social-1200x630.png")}
    <figure><img src="data:image/png;base64,${readFileSync(A("diagrams/pulse-architecture.png")).toString("base64")}"><figcaption><code>diagrams/pulse-architecture.svg / .png</code><span>${caption("diagrams/pulse-architecture.svg")}</span></figcaption></figure>
  </div>
</section>
<section>
  <h2>Install walkthrough (draft steps 1 and 4)</h2>
  <div class="grid">
    ${tile("walkthrough/step-1-install-output.png")}
    ${tile("walkthrough/step-4a-sign-in.png")}
    ${tile("walkthrough/step-4b-dashboard-first-run.png")}
  </div>
</section>
<section>
  <h2>Screenshots (1920×1080)</h2>
  <div class="grid">${shots.slice(0, 6).map(tile).join("")}</div>
</section>
<section>
  <h2>Screenshots (continued)</h2>
  <div class="grid">${shots.slice(6, 12).map(tile).join("")}</div>
</section>
${shots.length > 12 ? `<section><h2>Screenshots (continued)</h2><div class="grid">${shots.slice(12).map(tile).join("")}</div>
<p class="note">All images: real Pulse application; Ant Media Server simulated; viewers synthetic; admin tokens masked. Captions and caveats: <code>marketplace/asset-inventory.md</code>.</p></section>` : ""}
</body></html>`;

const browser = await chromium.launch();
const page = await browser.newPage();
await page.setContent(html, { waitUntil: "load" });
await page.evaluate(() => document.fonts.ready);
await page.pdf({ path: out, printBackground: true, preferCSSPageSize: true });
await browser.close();
console.log("wrote", out);
