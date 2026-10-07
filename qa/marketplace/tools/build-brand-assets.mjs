#!/usr/bin/env node
/**
 * Build the marketplace branding kit from the brand source of truth (brandkit/logo/*.svg).
 *
 *   node qa/marketplace/tools/build-brand-assets.mjs
 *
 * Output → docs/marketplace/antmedia-submission/assets/branding/
 *   svg/          the brandkit SVGs as shipped (live <text>, IBM Plex Sans)
 *   svg-outlined/ the same artwork with the wordmark converted to vector outlines, so
 *                 it renders identically on machines without IBM Plex Sans installed
 *   png/          transparent-background PNG exports at several sizes
 *
 * Nothing is redrawn: the pulse-line geometry and colours are copied verbatim from
 * brandkit/logo; only <text> nodes are converted to <path> using the self-hosted
 * IBM Plex Sans SemiBold (OFL) that the web UI already bundles. A pixel comparison of
 * live-text vs outlined renders is printed so drift is visible, not assumed.
 */

import { readFileSync, writeFileSync, mkdirSync, readdirSync } from "node:fs";
import { join, dirname, resolve, basename } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");
const require = createRequire(import.meta.url);
const opentype = require("opentype.js");
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const SRC = join(REPO, "brandkit/logo");
const OUT = join(REPO, "docs/marketplace/antmedia-submission/assets/branding");
const FONTS = join(REPO, "web/node_modules/@fontsource/ibm-plex-sans/files");
const fontFor = (weight) => opentype.parse(
  readFileSync(join(FONTS, `ibm-plex-sans-latin-${weight}-normal.woff`)).buffer,
);
const FONT_CACHE = {};

for (const d of ["svg", "svg-outlined", "png"]) mkdirSync(join(OUT, d), { recursive: true });

/** Replace every <text …>string</text> with an equivalent outlined <path>. */
function outlineText(svg) {
  return svg.replace(/<text([^>]*)>([^<]*)<\/text>/g, (_, attrs, content) => {
    const attr = (name, dflt) => (attrs.match(new RegExp(`${name}="([^"]*)"`)) || [])[1] ?? dflt;
    const x = parseFloat(attr("x", "0"));
    const y = parseFloat(attr("y", "0"));
    const size = parseFloat(attr("font-size", "16"));
    const weight = attr("font-weight", "400");
    const spacing = parseFloat(attr("letter-spacing", "0"));
    const anchor = attr("text-anchor", "start");
    const fill = attr("fill", "#000000");
    const font = (FONT_CACHE[weight] ??= fontFor(weight));
    const scale = size / font.unitsPerEm;
    const glyphs = font.stringToGlyphs(content);
    // Total advance (with kerning + letter-spacing) for anchor handling.
    let width = 0;
    glyphs.forEach((g, i) => {
      width += g.advanceWidth * scale;
      if (i < glyphs.length - 1) width += font.getKerningValue(g, glyphs[i + 1]) * scale + spacing;
    });
    let cursor = anchor === "middle" ? x - width / 2 : anchor === "end" ? x - width : x;
    const parts = [];
    glyphs.forEach((g, i) => {
      parts.push(g.getPath(cursor, y, size).toPathData(3));
      cursor += g.advanceWidth * scale;
      if (i < glyphs.length - 1) cursor += font.getKerningValue(g, glyphs[i + 1]) * scale + spacing;
    });
    return `<path aria-label="${content}" fill="${fill}" d="${parts.join(" ")}"></path>`;
  });
}

// Export plan: [source file, export name, pixel widths].
const PLAN = [
  ["pulse-logo-primary-dark.svg", "pulse-logo-for-dark-backgrounds", [480, 960, 1920]],
  ["pulse-logo-primary-light.svg", "pulse-logo-for-light-backgrounds", [480, 960, 1920]],
  ["pulse-logo-mono-white.svg", "pulse-logo-mono-white", [480, 960, 1920]],
  ["pulse-logo-mono-black.svg", "pulse-logo-mono-black", [480, 960, 1920]],
  ["pulse-logo-secondary-stacked.svg", "pulse-logo-stacked-for-dark-backgrounds", [320, 640, 1280]],
  ["pulse-mark.svg", "pulse-mark", [64, 128, 256, 512, 1024]],
  ["pulse-mark-light.svg", "pulse-mark-light", [64, 128, 256, 512, 1024]],
  ["favicon.svg", "pulse-favicon", [16, 32, 48, 64, 180]],
  ["powered-by-pulse-badge.svg", "powered-by-pulse-badge", [300, 600]],
];

// Fonts are inlined as data: URLs — a page created with setContent() has an opaque
// origin and cannot fetch file:// URLs, which would silently fall back to Arial.
const fontFace = ["400", "500", "600", "700"].map((w) =>
  `@font-face{font-family:'IBM Plex Sans';font-weight:${w};src:url(data:font/woff2;base64,${readFileSync(join(FONTS, `ibm-plex-sans-latin-${w}-normal.woff2`)).toString("base64")}) format('woff2');}`,
).join("\n");

const browser = await chromium.launch();
const page = await browser.newPage();

async function render(svg, widthPx) {
  const vb = svg.match(/viewBox="([\d.\s-]+)"/)[1].split(/\s+/).map(Number);
  const heightPx = Math.round((widthPx * vb[3]) / vb[2]);
  const sized = svg.replace(/width="[\d.]+"/, `width="${widthPx}"`).replace(/height="[\d.]+"/, `height="${heightPx}"`);
  await page.setViewportSize({ width: widthPx, height: heightPx });
  await page.setContent(
    `<!doctype html><html><head><style>${fontFace}html,body{margin:0;padding:0;background:transparent}svg{display:block}</style></head><body>${sized}</body></html>`,
  );
  await page.evaluate(async () => {
    await document.fonts.load("600 34px 'IBM Plex Sans'");
    await document.fonts.load("500 12px 'IBM Plex Sans'");
    await document.fonts.ready;
  });
  return page.screenshot({ omitBackground: true, clip: { x: 0, y: 0, width: widthPx, height: heightPx } });
}

/** Fraction of differing RGBA bytes between two equal-size PNG renders (decoded in-page). */
async function diffRatio(a, b) {
  return page.evaluate(async ([a64, b64]) => {
    const load = (b) => new Promise((res) => { const i = new Image(); i.onload = () => res(i); i.src = `data:image/png;base64,${b}`; });
    const [ia, ib] = await Promise.all([load(a64), load(b64)]);
    const c = document.createElement("canvas");
    c.width = ia.width; c.height = ia.height;
    const x = c.getContext("2d");
    x.drawImage(ia, 0, 0); const da = x.getImageData(0, 0, c.width, c.height).data;
    x.clearRect(0, 0, c.width, c.height);
    x.drawImage(ib, 0, 0); const db = x.getImageData(0, 0, c.width, c.height).data;
    let diff = 0;
    for (let i = 0; i < da.length; i += 4) {
      if (Math.abs(da[i] - db[i]) + Math.abs(da[i + 1] - db[i + 1]) + Math.abs(da[i + 2] - db[i + 2]) + Math.abs(da[i + 3] - db[i + 3]) > 96) diff++;
    }
    return diff / (da.length / 4);
  }, [a.toString("base64"), b.toString("base64")]);
}

/**
 * Tight-crop variant: same artwork, viewBox fitted to the drawn content plus a small
 * margin. The brandkit masters keep generous canvas padding (a 240-wide canvas for a
 * ~165-wide wordmark); web teams placing a logo usually want it trimmed.
 */
async function trimmed(svg) {
  await page.setContent(`<!doctype html><html><body>${svg.replace(/<svg([^>]*)>/, "<svg$1><g id=\"all\">").replace(/<\/svg>\s*$/, "</g></svg>")}</body></html>`);
  const b = await page.evaluate(() => {
    const r = document.getElementById("all").getBBox();
    return { x: r.x, y: r.y, w: r.width, h: r.height };
  });
  const pad = Math.max(1, Math.round(b.h * 0.04));
  const vb = [b.x - pad, b.y - pad, b.w + 2 * pad, b.h + 2 * pad].map((n) => Math.round(n * 100) / 100);
  return svg
    .replace(/viewBox="[^"]*"/, `viewBox="${vb.join(" ")}"`)
    .replace(/width="[\d.]+"/, `width="${vb[2]}"`)
    .replace(/height="[\d.]+"/, `height="${vb[3]}"`);
}
const TRIM = new Set([
  "pulse-logo-for-dark-backgrounds", "pulse-logo-for-light-backgrounds",
  "pulse-logo-mono-white", "pulse-logo-mono-black", "pulse-logo-stacked-for-dark-backgrounds",
]);

const report = [];
for (const [file, name, widths] of PLAN) {
  const original = readFileSync(join(SRC, file), "utf8");
  const outlined = outlineText(original)
    .replace(/<svg /, `<svg role="img" aria-label="Pulse" `);
  writeFileSync(join(OUT, "svg", `${name}.svg`), original);
  writeFileSync(join(OUT, "svg-outlined", `${name}.svg`), outlined);
  for (const w of widths) {
    const png = await render(outlined, w);
    writeFileSync(join(OUT, "png", `${name}-${w}w.png`), png);
  }
  if (TRIM.has(name)) {
    const t = await trimmed(outlined);
    writeFileSync(join(OUT, "svg-outlined", `${name}-trimmed.svg`), t);
    for (const w of widths) writeFileSync(join(OUT, "png", `${name}-trimmed-${w}w.png`), await render(t, w));
  }
  // Fidelity check at the largest size: live text (with IBM Plex loaded) vs outlines.
  const big = widths[widths.length - 1];
  const ratio = original.includes("<text") ? await diffRatio(await render(original, big), await render(outlined, big)) : 0;
  report.push({ name, outlinedText: original.includes("<text"), pixelDiffAtMax: `${(ratio * 100).toFixed(3)}%` });
}
await browser.close();

console.table(report);
console.log(`wrote ${readdirSync(join(OUT, "png")).length} PNGs, ${PLAN.length} SVGs (+ outlined) → ${OUT}`);
