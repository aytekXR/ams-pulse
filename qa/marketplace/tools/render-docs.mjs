#!/usr/bin/env node
/**
 * Render marketplace Markdown documents to brand-styled PDFs (and standalone HTML).
 *
 *   node qa/marketplace/tools/render-docs.mjs <out-dir> <file.md>[=Title] ...
 *
 * - Markdown → HTML with `marked` (pinned in this directory's package.json).
 * - Styling follows brandkit/design-system/tokens.json (light theme for print).
 * - Fonts (IBM Plex Sans/Mono, OFL) and every referenced local image are inlined
 *   as data: URLs, so the output is self-contained and nothing is fetched remotely.
 * - Chromium (Playwright) prints A4 PDFs with a page footer.
 */

import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { dirname, join, resolve, basename, extname } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";
import { marked } from "marked";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");
const require = createRequire(import.meta.url);
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const [outDir, ...inputs] = process.argv.slice(2);
if (!outDir || inputs.length === 0) {
  console.error("usage: render-docs.mjs <out-dir> <file.md>[=Title] ...");
  process.exit(2);
}
mkdirSync(outDir, { recursive: true });

const FONT_DIR = join(REPO, "web/node_modules/@fontsource");
const font = (pkg, file) => readFileSync(join(FONT_DIR, pkg, "files", file)).toString("base64");
const FONT_CSS = [
  ...["400", "500", "600", "700"].map((w) => `@font-face{font-family:'IBM Plex Sans';font-weight:${w};src:url(data:font/woff2;base64,${font("ibm-plex-sans", `ibm-plex-sans-latin-${w}-normal.woff2`)}) format('woff2')}`),
  ...["400", "500"].map((w) => `@font-face{font-family:'IBM Plex Mono';font-weight:${w};src:url(data:font/woff2;base64,${font("ibm-plex-mono", `ibm-plex-mono-latin-${w}-normal.woff2`)}) format('woff2')}`),
].join("\n");

const LOGO = readFileSync(
  join(REPO, "docs/marketplace/antmedia-submission/assets/branding/svg-outlined/pulse-logo-for-light-backgrounds-trimmed.svg"),
  "utf8",
).replace(/width="[\d.]+"/, 'width="132"').replace(/height="[\d.]+"/, "");

// tokens.json, light theme
const CSS = `
${FONT_CSS}
:root{--bg:#FFFFFF;--raised:#F0F4F7;--border:#D7DEE5;--text:#10181F;--text2:#4A5B6B;--signal:#087A59;--warn:#B45309;--crit:#DC2626}
*{box-sizing:border-box}
html{font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;color:var(--text);font-size:10.5pt;line-height:1.55}
body{margin:0}
header.doc{display:flex;align-items:center;justify-content:space-between;border-bottom:2px solid var(--signal);padding-bottom:10px;margin-bottom:18px}
header.doc .meta{font-size:8.5pt;color:var(--text2);text-align:right}
h1{font-size:21pt;line-height:1.2;margin:0 0 6px;letter-spacing:-0.02em}
h2{font-size:14pt;margin:22px 0 8px;padding-top:4px;border-top:1px solid var(--border);break-after:avoid}
h3{font-size:11.5pt;margin:16px 0 6px;break-after:avoid}
h4{font-size:10.5pt;margin:12px 0 4px;break-after:avoid}
p,li{orphans:3;widows:3}
a{color:var(--signal);text-decoration:none}
code{font-family:'IBM Plex Mono',ui-monospace,monospace;font-size:8.8pt;background:var(--raised);border-radius:4px;padding:1px 4px;overflow-wrap:anywhere}
pre{background:#0A0E14;color:#E8EEF4;border-radius:8px;padding:10px 12px;overflow-wrap:anywhere;white-space:pre-wrap;break-inside:avoid}
pre code{background:none;color:inherit;padding:0;font-size:8.4pt}
table{border-collapse:collapse;width:100%;margin:8px 0 12px;font-size:9pt;break-inside:auto}
th,td{border:1px solid var(--border);padding:5px 7px;text-align:left;vertical-align:top}
th{background:var(--raised);font-weight:600}
tr{break-inside:avoid}
blockquote{margin:10px 0;padding:8px 12px;border-left:3px solid var(--signal);background:#F4FAF7;color:var(--text)}
blockquote p{margin:4px 0}
img{max-width:100%;border:1px solid var(--border);border-radius:6px;display:block;margin:8px 0;break-inside:avoid}
hr{border:none;border-top:1px solid var(--border);margin:18px 0}
hr + h2{border-top:none;padding-top:0}
`;

function inlineImages(html, mdDir) {
  return html.replace(/<img([^>]*?)src="([^"]+)"/g, (m, pre, src) => {
    if (/^(https?:|data:)/.test(src)) return m;
    const p = resolve(mdDir, decodeURIComponent(src));
    if (!existsSync(p)) {
      console.warn(`  ! image not found: ${src}`);
      return m;
    }
    const mime = { ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".svg": "image/svg+xml" }[extname(p).toLowerCase()] ?? "application/octet-stream";
    return `<img${pre}src="data:${mime};base64,${readFileSync(p).toString("base64")}"`;
  });
}

const browser = await chromium.launch();
const page = await browser.newPage();
const date = new Date().toISOString().slice(0, 10);

for (const spec of inputs) {
  const [file, title] = spec.split("=");
  const md = readFileSync(file, "utf8");
  const body = inlineImages(marked.parse(md, { gfm: true }), dirname(resolve(file)));
  const docTitle = title || (md.match(/^#\s+(.+)$/m)?.[1] ?? basename(file, ".md"));
  const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>${docTitle}</title><style>${CSS}</style></head>
<body><header class="doc">${LOGO}<div class="meta">Pulse for Ant Media Server<br>${date}</div></header>${body}</body></html>`;
  const stem = basename(file, ".md");
  writeFileSync(join(outDir, `${stem}.html`), html);
  await page.setContent(html, { waitUntil: "load" });
  await page.evaluate(() => document.fonts.ready);
  await page.pdf({
    path: join(outDir, `${stem}.pdf`),
    format: "A4",
    printBackground: true,
    margin: { top: "16mm", bottom: "18mm", left: "16mm", right: "16mm" },
    displayHeaderFooter: true,
    headerTemplate: "<span></span>",
    footerTemplate: `<div style="font-family:Helvetica,Arial,sans-serif;font-size:7.5px;color:#6B7B88;width:100%;padding:0 16mm;display:flex;justify-content:space-between"><span>${docTitle.replace(/</g, "&lt;")}</span><span><span class="pageNumber"></span> / <span class="totalPages"></span></span></div>`,
  });
  console.log(`rendered ${stem}.pdf`);
}
await browser.close();
