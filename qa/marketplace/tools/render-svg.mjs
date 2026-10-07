#!/usr/bin/env node
/**
 * Render an SVG to PNG with the self-hosted IBM Plex fonts embedded (data: URLs).
 *
 *   node qa/marketplace/tools/render-svg.mjs <in.svg> <out.png> [scale=1]
 */
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");
const require = createRequire(import.meta.url);
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const [input, output, scaleArg] = process.argv.slice(2);
if (!input || !output) {
  console.error("usage: render-svg.mjs <in.svg> <out.png> [scale]");
  process.exit(2);
}
const scale = Number(scaleArg || 1);
const F = join(REPO, "web/node_modules/@fontsource");
const b64 = (pkg, file) => readFileSync(join(F, pkg, "files", file)).toString("base64");
const css = [
  ...["400", "500", "600", "700"].map((w) => `@font-face{font-family:'IBM Plex Sans';font-weight:${w};src:url(data:font/woff2;base64,${b64("ibm-plex-sans", `ibm-plex-sans-latin-${w}-normal.woff2`)}) format('woff2')}`),
  ...["400", "500"].map((w) => `@font-face{font-family:'IBM Plex Mono';font-weight:${w};src:url(data:font/woff2;base64,${b64("ibm-plex-mono", `ibm-plex-mono-latin-${w}-normal.woff2`)}) format('woff2')}`),
].join("");

const svg = readFileSync(input, "utf8");
const [w, h] = svg.match(/viewBox="[\d.-]+ [\d.-]+ ([\d.]+) ([\d.]+)"/).slice(1).map(Number);
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: Math.round(w), height: Math.round(h) }, deviceScaleFactor: scale });
await page.setContent(`<!doctype html><html><head><style>${css}html,body{margin:0;background:transparent}svg{display:block}</style></head><body>${svg}</body></html>`);
await page.evaluate(async () => {
  for (const f of ["400 14px 'IBM Plex Sans'", "500 14px 'IBM Plex Sans'", "600 14px 'IBM Plex Sans'", "400 12px 'IBM Plex Mono'"]) await document.fonts.load(f);
  await document.fonts.ready;
});
writeFileSync(output, await page.screenshot({ omitBackground: true, clip: { x: 0, y: 0, width: Math.round(w), height: Math.round(h) } }));
await browser.close();
console.log(`rendered ${output} (${Math.round(w * scale)}×${Math.round(h * scale)})`);
