#!/usr/bin/env node
/**
 * Compose the marketplace hero / social images from REAL screenshots.
 *
 *   node qa/marketplace/tools/compose-hero.mjs <spec.json>
 *
 * Every UI pixel in the output is a real capture (capture-real-stack.mjs, Mailpit).
 * The only drawn elements are the annotation (label, title, detail line), a focus ring,
 * a connector and the caption — the "annotated screenshot" treatment Ant Media's draft
 * asks for. Annotation text must be copied from what Pulse recorded (alert history,
 * delivery log); this tool never invents values.
 *
 * spec.json keys:
 *   out                 output directory
 *   dashboardIncident   1920×1080 dashboard capture taken DURING the incident (@2x preferred)
 *   dashboardBaseline   1920×1080 dashboard capture with nothing firing (@2x preferred)
 *   evidence            { src, clip:[x,y,w,h] }  real proof of delivery (e.g. the alert e-mail), CSS px
 *   focusRow            [x,y,w,h]  dashboard row the alert refers to, CSS px of the 1920×1080 capture
 *   label, title, detail, footnote, caption, headline, subhead   annotation / copy strings
 */

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, "../../..");
const require = createRequire(import.meta.url);
const pw = require(join(REPO, "web/node_modules/@playwright/test/index.js"));
const { chromium } = pw.default ?? pw;

const spec = JSON.parse(readFileSync(process.argv[2], "utf8"));
mkdirSync(spec.out, { recursive: true });
const b64 = (p) => readFileSync(p).toString("base64");
const F = join(REPO, "web/node_modules/@fontsource");
const font = (pkg, file) => readFileSync(join(F, pkg, "files", file)).toString("base64");
const FONT_CSS = [
  ...["400", "500", "600", "700"].map((w) => `@font-face{font-family:'IBM Plex Sans';font-weight:${w};src:url(data:font/woff2;base64,${font("ibm-plex-sans", `ibm-plex-sans-latin-${w}-normal.woff2`)}) format('woff2')}`),
  `@font-face{font-family:'IBM Plex Mono';font-weight:500;src:url(data:font/woff2;base64,${font("ibm-plex-mono", "ibm-plex-mono-latin-500-normal.woff2")}) format('woff2')}`,
].join("");
const MARK = (s) => `<svg width="${s}" height="${s}" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#10161D" stroke="#2B3947"/><path d="M12 32 H22 L27 18 L36 46 L41 32 H52" fill="none" stroke="#2CE5A7" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/></svg>`;

const BASE_CSS = `${FONT_CSS}
*{box-sizing:border-box;margin:0}
body{overflow:hidden;background:#0A0E14;font-family:'IBM Plex Sans',sans-serif;color:#E8EEF4;position:relative}
.glow{position:absolute;inset:0;background:radial-gradient(1100px 620px at 30% 35%,rgba(44,229,167,0.09),transparent 62%),radial-gradient(900px 520px at 95% 90%,rgba(255,178,36,0.06),transparent 60%)}
.grid{position:absolute;inset:0;background-image:linear-gradient(rgba(255,255,255,0.022) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.022) 1px,transparent 1px);background-size:48px 48px}
.frame{position:absolute;border-radius:12px;overflow:hidden;border:1px solid #2B3947;box-shadow:0 24px 64px rgba(0,0,0,0.5);background:#10161D}
.bar{height:28px;background:#161E27;border-bottom:1px solid #1E2833;display:flex;align-items:center;gap:7px;padding:0 12px}
.bar i{width:10px;height:10px;border-radius:50%;background:#2B3947;display:block}
.bar span{margin-left:12px;font:500 11px 'IBM Plex Mono',monospace;color:#5C6F80}
.frame img{display:block;width:100%}
.focus{position:absolute;border:2px solid #FFB224;border-radius:8px;box-shadow:0 0 0 5px rgba(255,178,36,0.16)}
.card{position:absolute;border-radius:12px;background:#10161D;border:1px solid #FFB224;box-shadow:0 24px 64px rgba(0,0,0,0.55);padding:16px 18px 18px}
.lbl{font:500 12px 'IBM Plex Mono',monospace;letter-spacing:.1em;color:#FFB224;display:flex;align-items:center;gap:8px;margin-bottom:10px}
.lbl b{width:9px;height:9px;border-radius:50%;background:#FFB224;display:inline-block;box-shadow:0 0 0 4px rgba(255,178,36,.18)}
.ttl{font-weight:600;line-height:1.25;margin-bottom:6px}
.dt{color:#9FB0C0;line-height:1.45;margin-bottom:14px}
.ev{border-radius:8px;overflow:hidden;border:1px solid #2B3947}
.ev img{display:block;width:100%}
.fn{font:500 11px 'IBM Plex Mono',monospace;color:#5C6F80;margin-top:12px;line-height:1.5}
.cap{position:absolute;font:500 11px 'IBM Plex Mono',monospace;color:#5C6F80}
svg.wire{position:absolute;inset:0;pointer-events:none}
`;

function frame(src, x, y, w) {
  return `<div class="frame" style="left:${x}px;top:${y}px;width:${w}px"><div class="bar"><i></i><i></i><i></i><span>https://pulse.your-domain</span></div><img src="data:image/png;base64,${b64(src)}"></div>`;
}

/** 1920×1080 annotated hero: dashboard left, alert card right, connector to the row. */
function heroAnnotated(W, H) {
  const fx = 56, fw = 1300, s = fw / 1920;
  const fy = Math.round((H - (1080 * s + 28)) / 2);
  const [rx, ry, rw, rh] = spec.focusRow;
  const focus = { x: fx + rx * s - 6, y: fy + 28 + ry * s - 5, w: rw * s + 12, h: rh * s + 10 };
  const cx = fx + fw + 44, cw = W - cx - 56;
  const [, , ew, eh] = spec.evidence.clip;
  const evH = Math.round((cw - 36) * (eh / ew));
  const cardH = 150 + evH + 40;
  const cy = Math.max(48, Math.round(focus.y + focus.h / 2 - cardH * 0.62));
  const wireY = focus.y + focus.h / 2;
  return `<!doctype html><html><head><meta charset="utf-8"><style>${BASE_CSS} body{width:${W}px;height:${H}px}</style></head><body>
<div class="glow"></div><div class="grid"></div>
${frame(spec.dashboardIncident, fx, fy, fw)}
<div class="focus" style="left:${focus.x}px;top:${focus.y}px;width:${focus.w}px;height:${focus.h}px"></div>
<svg class="wire" width="${W}" height="${H}"><path d="M${focus.x + focus.w} ${wireY} H ${cx - 18} V ${cy + 40} H ${cx}" stroke="#FFB224" stroke-width="2" fill="none" stroke-dasharray="6 5"/><circle cx="${focus.x + focus.w}" cy="${wireY}" r="4" fill="#FFB224"/></svg>
<div class="card" style="left:${cx}px;top:${cy}px;width:${cw}px">
  <div class="lbl"><b></b>${spec.label}</div>
  <div class="ttl" style="font-size:21px">${spec.title}</div>
  <div class="dt" style="font-size:14px">${spec.detail}</div>
  <div class="ev"><img src="data:image/png;base64,${spec.evidenceB64}"></div>
  <div class="fn">${spec.footnote}</div>
</div>
<div class="cap" style="left:${fx}px;bottom:22px">${spec.caption}</div>
</body></html>`;
}

/** 1920×1080 clean hero: baseline dashboard, centred, no annotation. */
function heroClean(W, H) {
  const fw = 1560, s = fw / 1920, fx = Math.round((W - fw) / 2);
  const fy = Math.round((H - (1080 * s + 28)) / 2) + 6;
  return `<!doctype html><html><head><meta charset="utf-8"><style>${BASE_CSS} body{width:${W}px;height:${H}px}</style></head><body>
<div class="glow"></div><div class="grid"></div>${frame(spec.dashboardBaseline, fx, fy, fw)}
<div class="cap" style="left:${fx}px;bottom:14px">${spec.caption}</div></body></html>`;
}

/** 1200×630 social card: brand + headline left, annotated dashboard crop right. */
function social(W, H) {
  const fw = 760, s = fw / 1920, fx = W - fw - 36, fy = 64;
  const [rx, ry, rw, rh] = spec.focusRow;
  const focus = { x: fx + rx * s - 4, y: fy + 28 + ry * s - 4, w: rw * s + 8, h: rh * s + 8 };
  return `<!doctype html><html><head><meta charset="utf-8"><style>${BASE_CSS} body{width:${W}px;height:${H}px}</style></head><body>
<div class="glow"></div><div class="grid"></div>
<div style="position:absolute;left:40px;top:56px;width:340px">
  <div style="display:flex;align-items:center;gap:10px;font-weight:600;font-size:17px;margin-bottom:40px">${MARK(30)}<span>Pulse</span></div>
  <div style="font-size:33px;line-height:1.12;font-weight:700;letter-spacing:-0.02em;margin-bottom:16px">${spec.headline}</div>
  <div style="font-size:15px;line-height:1.5;color:#9FB0C0">${spec.subhead}</div>
</div>
${frame(spec.dashboardIncident, fx, fy, fw)}
<div class="focus" style="left:${focus.x}px;top:${focus.y}px;width:${focus.w}px;height:${focus.h}px"></div>
<div class="card" style="left:${fx + 230}px;top:${focus.y - 118}px;width:330px;padding:12px 14px">
  <div class="lbl" style="font-size:10.5px;margin-bottom:6px"><b></b>${spec.label}</div>
  <div class="ttl" style="font-size:15px;margin-bottom:2px">${spec.title}</div>
  <div class="dt" style="font-size:11.5px;margin-bottom:0">${spec.detailShort}</div>
</div>
<div class="cap" style="left:40px;bottom:16px;font-size:9.5px">${spec.caption}</div>
</body></html>`;
}

const browser = await chromium.launch();
{ // crop the real proof-of-delivery from its capture
  const [x, y, w, h] = spec.evidence.clip;
  const p = await browser.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: spec.evidence.scale || 1 });
  await p.setContent(`<html><body style="margin:0"><img src="data:image/png;base64,${b64(spec.evidence.src)}" style="width:1920px;height:1080px;display:block"></body></html>`);
  spec.evidenceB64 = (await p.screenshot({ clip: { x, y, width: w, height: h } })).toString("base64");
  await p.close();
}
const variants = [
  { file: "pulse-hero-alert-1920x1080.png", W: 1920, H: 1080, html: heroAnnotated, dsf: 1 },
  { file: "pulse-hero-alert-3840x2160.png", W: 1920, H: 1080, html: heroAnnotated, dsf: 2 },
  { file: "pulse-hero-dashboard-1920x1080.png", W: 1920, H: 1080, html: heroClean, dsf: 1 },
  { file: "pulse-social-1200x630.png", W: 1200, H: 630, html: social, dsf: 1 },
  { file: "pulse-social-2400x1260.png", W: 1200, H: 630, html: social, dsf: 2 },
];
for (const v of variants) {
  const p = await browser.newPage({ viewport: { width: v.W, height: v.H }, deviceScaleFactor: v.dsf });
  await p.setContent(v.html(v.W, v.H), { waitUntil: "load" });
  await p.evaluate(() => document.fonts.ready);
  writeFileSync(join(spec.out, v.file), await p.screenshot());
  await p.close();
  console.log("wrote", v.file);
}
await browser.close();
