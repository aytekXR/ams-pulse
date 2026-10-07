#!/usr/bin/env node
/**
 * Render a CAPTURED terminal log as a terminal-window image for the install walkthrough.
 *
 *   node qa/marketplace/tools/render-terminal.mjs <log.txt> <out.png> "<command as typed>" [exit-code]
 *
 * The log is shown verbatim except for two redactions, applied to both the command and
 * the output: Pulse admin/API tokens (plt_…) and the value of --password. Lines that
 * the capture wrapper itself added (starting with "[installer exit code") are dropped;
 * pass the exit code as the 4th argument to show it as `$ echo $?`.
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

const [logFile, out, command, exitCode] = process.argv.slice(2);
const esc = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const redact = (s) =>
  s.replace(/plt_[a-f0-9]{8,}/g, (m) => "plt_" + "•".repeat(Math.min(44, m.length - 4)))
    .replace(/(--password\s+)(\S+)/g, (_, a) => `${a}'••••••••'`);
const lines = readFileSync(logFile, "utf8").split("\n").filter((l) => !l.startsWith("[installer exit code"));
while (lines.length && lines[lines.length - 1].trim() === "") lines.pop();

const F = join(REPO, "web/node_modules/@fontsource");
const font = (w) => readFileSync(join(F, "ibm-plex-mono", "files", `ibm-plex-mono-latin-${w}-normal.woff2`)).toString("base64");
const body = lines.map((l) => {
  const r = esc(redact(l));
  if (/plt_•/.test(r)) return `<span class="tok">${r}</span>`;
  if (/^(Pulse is healthy\.|Image OK\.)/.test(l)) return `<span class="ok">${r}</span>`;
  if (/^(Next steps:|  UI:)/.test(l)) return `<span class="hi">${r}</span>`;
  return r;
}).join("\n");
const html = `<!doctype html><html><head><meta charset="utf-8"><style>
@font-face{font-family:'IBM Plex Mono';font-weight:400;src:url(data:font/woff2;base64,${font(400)}) format('woff2')}
@font-face{font-family:'IBM Plex Mono';font-weight:500;src:url(data:font/woff2;base64,${font(500)}) format('woff2')}
*{box-sizing:border-box;margin:0}
body{width:1920px;height:1080px;background:#0A0E14;display:flex;align-items:center;justify-content:center;
  background-image:linear-gradient(rgba(255,255,255,0.022) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.022) 1px,transparent 1px);background-size:48px 48px}
.win{width:1680px;border-radius:12px;overflow:hidden;border:1px solid #2B3947;box-shadow:0 24px 64px rgba(0,0,0,0.5);background:#0D1218}
.bar{height:34px;background:#161E27;border-bottom:1px solid #1E2833;display:flex;align-items:center;gap:8px;padding:0 14px;position:relative}
.bar i{width:12px;height:12px;border-radius:50%;background:#2B3947;display:block}
.bar span{position:absolute;left:0;right:0;text-align:center;font:500 13px 'IBM Plex Mono',monospace;color:#5C6F80}
pre{font:400 14.2px/1.42 'IBM Plex Mono',monospace;color:#C9D4DE;padding:16px 22px 18px;white-space:pre-wrap;word-break:break-all}
.ps{color:#2CE5A7}.cmd{color:#E8EEF4}.ok{color:#2CE5A7}.hi{color:#E8EEF4;font-weight:500}.tok{color:#FFB224}
</style></head><body><div class="win"><div class="bar"><i></i><i></i><i></i><span>bash — Pulse quickstart install</span></div>
<pre><span class="ps">$</span> <span class="cmd">${esc(redact(command))}</span>
${body}${exitCode !== undefined ? `\n<span class="ps">$</span> <span class="cmd">echo $?</span>\n${esc(exitCode)}` : ""}</pre></div></body></html>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
await page.setContent(html);
await page.evaluate(() => document.fonts.ready);
const h = await page.evaluate(() => document.querySelector(".win").getBoundingClientRect().height);
// Never clip or condense the log: grow the canvas (1920 wide) to fit the whole window.
const canvasH = Math.max(1080, Math.ceil(h + 96));
await page.setViewportSize({ width: 1920, height: canvasH });
await page.evaluate((ch) => { document.body.style.height = ch + "px"; }, canvasH);
writeFileSync(out, await page.screenshot());
await browser.close();
console.log(`rendered ${out} (${lines.length} log lines, window ${Math.round(h)}px, canvas 1920x${canvasH})`);
