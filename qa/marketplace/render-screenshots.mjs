/**
 * qa/marketplace/render-screenshots.mjs
 *
 * Automated screenshot renderer for Pulse marketplace listing images.
 * Targets: SS1 (Dashboard), SS2 (Stream Detail), SS4 (Analytics) from
 * brandkit/ui/Pulse App - Screens.dc.html
 *
 * FONT CDN NOTICE:
 *   brandkit/ui/Pulse App - Screens.dc.html lines 14–15 pull IBM Plex Sans and
 *   IBM Plex Mono from the Google Fonts CDN (fonts.googleapis.com /
 *   fonts.gstatic.com). This violates the CLAUDE.md §6 self-hosting mandate
 *   ("Fonts (IBM Plex, OFL) are self-hosted only — never a CDN"). This script
 *   works around the violation by patching the dc.html in a temporary render
 *   copy — replacing the two CDN <link> tags with inline @font-face CSS that
 *   references woff2 files already installed in web/node_modules/@fontsource/.
 *   The brandkit source fix (removing the CDN links from the original dc.html)
 *   is a designer/operator decision and is NOT made here.
 *
 *   The support.js runtime (brandkit/ui/support.js) additionally loads React,
 *   ReactDOM, and Babel from unpkg.com CDN at runtime. Our route handler aborts
 *   those requests; instead we patch the render-copy's support.js to pre-inject
 *   stub globals (window.React, window.ReactDOM) so the runtime boots without
 *   the CDN — the dc.html screens are plain static HTML requiring no React
 *   components beyond the basic runtime boot.
 *
 * Usage:
 *   node qa/marketplace/render-screenshots.mjs
 *
 * Output:
 *   docs/marketplace/screenshots/ss1-dashboard.png
 *   docs/marketplace/screenshots/ss2-stream-detail.png
 *   docs/marketplace/screenshots/ss4-analytics.png
 *
 * Exit code:
 *   0 — all 3 PNGs written, non-zero size
 *   1 — any PNG missing or zero-byte
 */

import { createRequire } from 'module';
import { promises as fs } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname  = path.dirname(__filename);

// Resolve playwright from web/node_modules (project-local install)
const require = createRequire(import.meta.url);
const playwrightPath = path.resolve(__dirname, '../../web/node_modules/playwright');
const { chromium } = require(playwrightPath);

// ── Path constants ─────────────────────────────────────────────────────────
const REPO_ROOT      = path.resolve(__dirname, '../..');
const BRANDKIT       = path.join(REPO_ROOT, 'brandkit', 'ui');
const DC_SRC         = path.join(BRANDKIT, 'Pulse App - Screens.dc.html');
const SUPP_SRC       = path.join(BRANDKIT, 'support.js');
const OUT_DIR        = path.join(REPO_ROOT, 'docs', 'marketplace', 'screenshots');
const RENDER_DIR     = '/tmp/pulse-marketplace-render';

// Playwright ships its own Chromium binary which needs system ATK/X11 libs
// that may not be installed (e.g. libatk-1.0, libgbm, libasound).  We bundle
// them into a local lib-cache at first run so the script is self-contained.
const LIB_CACHE_DIR  = '/tmp/pulse-marketplace-libs';
const HEADLESS_SHELL = path.join(
  process.env.HOME,
  '.cache/ms-playwright/chromium_headless_shell-1228/' +
  'chrome-headless-shell-linux64/chrome-headless-shell',
);

// ── Ensure runtime libraries are available ────────────────────────────────
async function ensureLibs () {
  const markerFile = path.join(LIB_CACHE_DIR, '.ready');
  try {
    await fs.access(markerFile);
    return; // already extracted
  } catch { /* not ready yet */ }

  console.log('[libs] Downloading missing Chromium system libraries…');
  await fs.mkdir(LIB_CACHE_DIR, { recursive: true });

  const { execSync } = await import('child_process');
  const tmpDeb = path.join(LIB_CACHE_DIR, '_deb');
  await fs.mkdir(tmpDeb, { recursive: true });

  const PACKAGES = [
    'libatk1.0-0t64', 'libatk-bridge2.0-0t64', 'libatspi2.0-0t64',
    'libcups2t64', 'libgbm1', 'libxcomposite1', 'libxdamage1',
    'libxfixes3', 'libxrandr2', 'libasound2t64',
    'libcairo2', 'libpango-1.0-0',
    'libxrender1', 'libxext6', 'libx11-6', 'libxcb1',
    'libxi6', 'libxkbcommon0',
  ];

  execSync(`cd "${tmpDeb}" && apt-get download ${PACKAGES.join(' ')}`, { stdio: 'inherit' });

  for (const deb of (await fs.readdir(tmpDeb)).filter(f => f.endsWith('.deb'))) {
    execSync(`dpkg-deb -x "${path.join(tmpDeb, deb)}" "${LIB_CACHE_DIR}"`,
             { stdio: 'pipe' });
  }
  await fs.writeFile(markerFile, 'ok');
  console.log('[libs] Runtime libraries ready.');
}

function getLdLibraryPath () {
  return path.join(LIB_CACHE_DIR, 'usr', 'lib', 'x86_64-linux-gnu');
}

// IBM Plex Sans weights used in dc.html: 400, 500, 600, 700
// IBM Plex Mono weights used in dc.html: 400, 500, 700
const SANS_WEIGHTS  = [400, 500, 600, 700];
const MONO_WEIGHTS  = [400, 500, 700];
const FONTSOURCE    = path.join(REPO_ROOT, 'web', 'node_modules', '@fontsource');

// ── Build inline @font-face CSS to replace CDN links ─────────────────────
function buildFontFaceCSS () {
  const faces = [];

  for (const w of SANS_WEIGHTS) {
    const file = `ibm-plex-sans-latin-${w}-normal.woff2`;
    faces.push(
      `@font-face{font-family:'IBM Plex Sans';font-style:normal;font-weight:${w};` +
      `font-display:swap;src:url('./fonts/${file}') format('woff2');}`
    );
  }
  for (const w of MONO_WEIGHTS) {
    const file = `ibm-plex-mono-latin-${w}-normal.woff2`;
    faces.push(
      `@font-face{font-family:'IBM Plex Mono';font-style:normal;font-weight:${w};` +
      `font-display:swap;src:url('./fonts/${file}') format('woff2');}`
    );
  }

  return `<style>\n${faces.join('\n')}\n</style>`;
}

// ── Copy woff2 files into render dir ──────────────────────────────────────
async function copyFontFiles (renderFontsDir) {
  await fs.mkdir(renderFontsDir, { recursive: true });

  for (const w of SANS_WEIGHTS) {
    const file = `ibm-plex-sans-latin-${w}-normal.woff2`;
    const src  = path.join(FONTSOURCE, 'ibm-plex-sans', 'files', file);
    await fs.copyFile(src, path.join(renderFontsDir, file));
  }
  for (const w of MONO_WEIGHTS) {
    const file = `ibm-plex-mono-latin-${w}-normal.woff2`;
    const src  = path.join(FONTSOURCE, 'ibm-plex-mono', 'files', file);
    await fs.copyFile(src, path.join(renderFontsDir, file));
  }
}

// ── Patch support.js: pre-stub window.React/ReactDOM so loadReactUmd() ───
// resolves immediately without CDN.  The dc.html screens are static HTML
// that only needs the boot sequence to complete; no actual React components
// are used in the design-canvas screens we screenshot.
function patchSupportJs (src) {
  // Prepend minimal React stubs at the top of the IIFE, before any other code.
  // support.js checks `w.React && w.ReactDOM` and skips CDN load if truthy.
  const stub = `
/* render-screenshots patch — local React stub so CDN load is skipped */
(function() {
  if (window.React && window.ReactDOM) return;
  // Minimal stubs: support.js calls React.createElement, ReactDOM.createRoot
  var frag = Symbol('frag');
  window.React = {
    createElement: function(t, p) {
      var args = Array.prototype.slice.call(arguments, 2);
      return { type: t, props: Object.assign({}, p, { children: args.length === 1 ? args[0] : args }) };
    },
    Fragment: frag,
    createContext: function(d) { return { _currentValue: d, Provider: function(){}, Consumer: function(){} }; },
    useRef: function(v) { return { current: v }; },
    useState: function(v) { return [v, function(){}]; },
    useEffect: function() {},
    useLayoutEffect: function() {},
    useMemo: function(fn) { return fn(); },
    useCallback: function(fn) { return fn; },
    useContext: function(ctx) { return ctx._currentValue; },
    memo: function(c) { return c; },
    forwardRef: function(fn) { return fn; },
    isValidElement: function() { return false; },
    Children: { map: function(c, fn) { return Array.isArray(c) ? c.map(fn) : []; }, forEach: function(){}, count: function(c){ return Array.isArray(c) ? c.length : 0; }, toArray: function(c){ return Array.isArray(c) ? c : []; } },
    version: '18.3.1',
  };
  var noop = function() { return { render: function(){}, unmount: function(){} }; };
  window.ReactDOM = {
    createRoot: noop,
    render: function() {},
    unmountComponentAtNode: function() {},
    version: '18.3.1',
  };
})();
`;
  return stub + src;
}

// ── Prepare render copy of dc.html and support.js ─────────────────────────
async function prepareRenderDir () {
  await fs.mkdir(RENDER_DIR, { recursive: true });
  await fs.mkdir(OUT_DIR,    { recursive: true });

  // Patch support.js: pre-stub React globals to skip CDN load
  const supportSrc    = await fs.readFile(SUPP_SRC, 'utf8');
  const supportPatched = patchSupportJs(supportSrc);
  await fs.writeFile(path.join(RENDER_DIR, 'support.js'), supportPatched, 'utf8');

  // Copy woff2 files
  const renderFontsDir = path.join(RENDER_DIR, 'fonts');
  await copyFontFiles(renderFontsDir);

  // Patch dc.html:
  //   1. Replace two Google Fonts CDN <link> tags with inline @font-face CSS
  //   2. Keep support.js reference (./support.js) — it stays co-located
  let html = await fs.readFile(DC_SRC, 'utf8');

  // Remove CDN font link tags (lines 14–15)
  const cdnPattern =
    /<link\s[^>]*href=["']https:\/\/fonts\.(?:googleapis|gstatic)\.com[^"']*["'][^>]*>\s*/g;
  html = html.replace(cdnPattern, '');

  // Inject @font-face CSS after <helmet> open tag
  const fontFaceCSS = buildFontFaceCSS();
  html = html.replace('<helmet>', `<helmet>\n${fontFaceCSS}`);

  const dcDest = path.join(RENDER_DIR, 'Pulse App - Screens.dc.html');
  await fs.writeFile(dcDest, html, 'utf8');
  return dcDest;
}

// ── Screenshot targets ─────────────────────────────────────────────────────
const SHOTS = [
  {
    label    : 'Dashboard',
    outFile  : 'ss1-dashboard.png',
    viewport : { width: 1440, height: 900 },
  },
  {
    label    : 'Stream Detail',
    outFile  : 'ss2-stream-detail.png',
    viewport : { width: 1440, height: 900 },
  },
  {
    label    : 'Analytics',
    outFile  : 'ss4-analytics.png',
    viewport : { width: 1440, height: 900 },
  },
];

// ── Main ──────────────────────────────────────────────────────────────────
async function main () {
  await ensureLibs();
  const dcDest = await prepareRenderDir();
  const fileUrl = `file://${dcDest}`;

  // Inject the local lib-cache into LD_LIBRARY_PATH so the Chromium subprocess
  // can find the ATK/X11/GBM libraries if they are not system-installed.
  const libPath = getLdLibraryPath();
  process.env.LD_LIBRARY_PATH = process.env.LD_LIBRARY_PATH
    ? `${libPath}:${process.env.LD_LIBRARY_PATH}`
    : libPath;

  const launchOpts = { headless: true };
  // Use the headless-shell binary (smaller, no GPU deps beyond what we supply)
  try {
    await fs.access(HEADLESS_SHELL);
    launchOpts.executablePath = HEADLESS_SHELL;
  } catch { /* fall through to default playwright binary */ }

  const browser = await chromium.launch(launchOpts);
  try {
    const context = await browser.newContext();
    const page    = await context.newPage();

    // Abort every non-file:// request — proves zero CDN/network reliance.
    // React/ReactDOM stubs are pre-injected into the support.js copy so the
    // runtime boots without CDN even with this route handler in place.
    await page.route('**/*', (route) => {
      const url = route.request().url();
      if (url.startsWith('file://')) {
        route.continue();
      } else {
        route.abort();
      }
    });

    const errors = [];
    page.on('pageerror', (err) => errors.push(err.message));

    for (const shot of SHOTS) {
      await page.setViewportSize(shot.viewport);
      await page.goto(fileUrl, { waitUntil: 'networkidle' });

      // Wait for support.js to boot and render (custom element hydration).
      // 3 s is ample for the stub React boot on static HTML.
      await page.waitForTimeout(3000);

      // support.js sets x-dc{display:none!important} to hide the raw template
      // while React re-renders it.  With stubs the re-render may not
      // produce full layout; we force x-dc visible so the static HTML is
      // directly screenshottable — the screens are pixel-complete static HTML
      // and do not rely on React for their visual output.
      await page.evaluate(() => {
        // Override the injected display:none rule
        const override = document.createElement('style');
        override.textContent = 'x-dc { display: block !important; }';
        document.head.appendChild(override);
      });

      await page.waitForTimeout(500); // let layout recalculate

      // Selector: the 1280×800 inner div that is the first <div> child of the screen wrapper
      const selector = `[data-screen-label="${shot.label}"] > div`;
      const el = page.locator(selector).first();

      const count = await el.count();
      if (count === 0) {
        throw new Error(`Selector not found for screen "${shot.label}": ${selector}`);
      }

      // The dc.html stacks all screens vertically; the target screen may be
      // below the viewport.  Use fullPage:true + absolute page coordinates so
      // the clip works regardless of scroll position.
      const box = await el.evaluate((node) => {
        const r = node.getBoundingClientRect();
        // getBoundingClientRect is viewport-relative; add scroll offset for
        // fullPage coordinates.
        return {
          x      : r.x      + window.scrollX,
          y      : r.y      + window.scrollY,
          width  : r.width,
          height : r.height,
        };
      });

      if (!box || box.width === 0 || box.height === 0) {
        throw new Error(`Zero bounding box for screen "${shot.label}": ${JSON.stringify(box)}`);
      }

      const outPath = path.join(OUT_DIR, shot.outFile);
      await page.screenshot({
        path     : outPath,
        type     : 'png',
        fullPage : true,
        clip     : { x: box.x, y: box.y, width: box.width, height: box.height },
      });

      // Verify non-zero size
      const stat = await fs.stat(outPath);
      if (stat.size === 0) {
        throw new Error(`Screenshot is zero bytes: ${outPath}`);
      }

      const w = Math.round(box.width);
      const h = Math.round(box.height);
      console.log(`OK  ${outPath}  [${w}x${h}px]  ${stat.size} bytes`);
    }

    if (errors.length > 0) {
      console.warn(`\nPage errors during render (${errors.length}):`);
      errors.forEach((e) => console.warn('  ', e));
    }
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  console.error('render-screenshots FAILED:', err.message);
  process.exit(1);
});
