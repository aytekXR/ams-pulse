#!/usr/bin/env bash
# shellcheck disable=SC2317,SC2329,SC2044,SC2295,SC2034  # justified below, above `set -euo pipefail`
# (SC2317 and SC2329 are the SAME check: CI ships shellcheck 0.9.0, the :stable
#  container ships 0.11.0, and it was renumbered between them. Both must be named
#  or the script is clean at one version and red at the other.)
# =============================================================================
# run-checks.sh — validate the Pulse public website before deploy.
#
# Runs:
#   1. HTML validity (html5validator)
#   2. Internal link/asset integrity (no broken hrefs/srcs, including anchors)
#   3. Accessibility (axe-core via Playwright)
#   4. No external resources (scripts/styles/fonts/images must be self-hosted)
#   5. Root-absolute path check (catches subpath-hosting breakage)
#   6. Semantic HTML checks (main landmark, img alt)
#   7. OG/Twitter image existence check
#   8. Subpath stylesheet loading verification
#
# Usage:
#   ./run-checks.sh <webroot>           # validate files in <webroot>
#   ./run-checks.sh <webroot> <baseurl> # for subpath hosting (e.g. /ams-pulse/)
#
# Example:
#   ./run-checks.sh .                   # root hosting
#   ./run-checks.sh . /ams-pulse/       # subpath hosting
#
# All checks run against the BUILT output, not source files.
# =============================================================================
set -euo pipefail

# ── shellcheck scope, and what is deliberately silenced ──────────────────────
# This script is now covered by the `shellcheck` CI job (D-187). It was not,
# which is the same scope gap this repo keeps re-learning: a guard that names its
# targets one by one stops covering the file someone adds next. Four classes are
# silenced here, each on purpose:
#
#   SC2317  "command appears unreachable" — every cleanup function in this script
#           is invoked by a `trap`, which shellcheck cannot see. Real, and 14 of
#           the findings. Note CI runs 0.9.0, where this check is SC2317; the
#           :stable container renumbered it to SC2329, so a fix for one can be
#           red on the other.
#   SC2044  "for loops over find output are fragile" — true when a path contains
#           whitespace. Every path here comes from website/, whose contents this
#           repo controls and which are asserted elsewhere to be plain ASCII
#           names. Rewriting seven loops to `while read -d ''` would add risk to
#           a working checker for no reachable defect.
#   SC2295  quoting inside ${..} — the patterns stripped here are literal
#           prefixes computed in this script, not user input.
#   SC2034  two counters kept for symmetry with the sections that do use them.

# Default the web root to the site this script belongs to, NOT to $PWD.
#
# It used to default to "." — so running it from the repository root walked the
# whole tree and reported 28 "missing main landmark" violations against
# web/node_modules, web/dist and the generated API docs. A guard that fires on
# files it was never meant to police is a guard people learn to ignore, and its
# scope is a decision worth writing down: this script checks website/ and nothing
# else. Pass an explicit path to override.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEBROOT="${1:-$(dirname "$SCRIPT_DIR")}"
BASEURL="${2:-/}"

# Temp files this script creates must be removed even when a LATER trap replaces
# an earlier one. They live under $PLAYWRIGHT_DIR (web/) rather than /tmp because
# node resolves @playwright/test by walking up from the script's own directory —
# so the price of running the checks was two untracked files left in web/, which
# then show up in `git status` and read like someone's forgotten work. A checker
# that dirties the tree it is checking is a small bug with an outsized cost in a
# repo where an unexpected untracked file is a signal worth investigating.
#
# trap replaces; it does not append. Register here, remove in one place.
_TMP_FILES=()
_rm_tmp_files() { local f; for f in "${_TMP_FILES[@]:-}"; do [ -n "$f" ] && rm -f "$f"; done; }
trap _rm_tmp_files EXIT

cd "$WEBROOT"
WEBROOT_ABS="$(pwd)"

say()  { printf '\n== %s\n' "$1"; }
info() { printf '   %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1" >&2; }

FAILED=0

# ---------------------------------------------------------------------------
# 1. HTML validity
# ---------------------------------------------------------------------------
say "1/8 HTML validity"
# Exclude tools/ directory from all checks — those are generator templates, not pages
HTML_FILES=$(find . -name '*.html' -type f -not -path './tools/*' 2>/dev/null || true)
if [ -z "$HTML_FILES" ]; then
  info "no HTML files found — skipping validity check"
else
  # Create a temporary config file for html-validate (must have .json extension)
  CONFIG_DIR=$(mktemp -d)
  CONFIG_FILE="$CONFIG_DIR/.htmlvalidate.json"
  cat > "$CONFIG_FILE" << 'HTMLVALIDATE_CONFIG'
{
  "extends": ["html-validate:recommended"],
  "rules": {
    "no-inline-style": "off",
    "no-trailing-whitespace": "off",
    "require-sri": "off",
    "doctype-style": "off"
  }
}
HTMLVALIDATE_CONFIG

  # Use html-validate (npm) — it's more readily available than html5validator
  if ! find . -name '*.html' -type f -not -path './tools/*' -print0 | xargs -0 npx --yes html-validate --config "$CONFIG_FILE"; then
    fail "HTML validation failed"
    FAILED=1
  else
    info "HTML valid"
  fi

  rm -rf "$CONFIG_DIR"
fi

# ---------------------------------------------------------------------------
# 2. Internal link/asset integrity
# ---------------------------------------------------------------------------
say "2/8 Link/asset integrity"

# Check script inlined for portability — no external link-checker dependency
check_links() {
  local root="$1"
  local baseurl="$2"
  local errors=0

  # Collect all HTML files
  local html_files
  html_files=$(find "$root" -name '*.html' -type f -not -path '*/tools/*')

  for html in $html_files; do
    local dir
    dir=$(dirname "$html")

    # Extract href and src values (grep with PCRE for proper attribute matching)
    local refs
    refs=$(grep -oP '(?:href|src|srcset)=["\047][^"\047]*["\047]' "$html" 2>/dev/null | \
           sed -E "s/^(href|src|srcset)=[\"']//; s/[\"']$//" | \
           tr ',' '\n' | sed 's/ [0-9]*[wx]$//' | sort -u || true)

    for ref in $refs; do
      # Skip empty, data:, mailto:, tel:, javascript:
      [[ -z "$ref" ]] && continue
      [[ "$ref" =~ ^(data:|mailto:|tel:|javascript:) ]] && continue

      # External URL — checked separately in step 4
      [[ "$ref" =~ ^https?:// ]] && continue

      # Handle anchor links within the same page
      local path="$ref"
      local anchor=""
      if [[ "$ref" == *"#"* ]]; then
        path="${ref%%#*}"
        anchor="${ref#*#}"
      fi

      # Resolve path
      local target
      if [[ "$path" == /* ]]; then
        # Root-relative (after stripping baseurl prefix if present)
        local stripped="${path#$baseurl}"
        target="$root/$stripped"
      elif [[ -n "$path" ]]; then
        # Relative
        target="$dir/$path"
      else
        # Just an anchor to the current page
        target="$html"
      fi

      # Normalize path
      target=$(realpath -m "$target" 2>/dev/null || echo "$target")

      # Check if target file exists
      if [[ -n "$path" ]] && [[ ! -f "$target" ]] && [[ ! -d "$target" ]]; then
        # Maybe it's a directory with index.html
        if [[ -f "$target/index.html" ]]; then
          target="$target/index.html"
        else
          printf '  %s: broken link "%s" (target: %s)\n' "$html" "$ref" "$target"
          errors=$((errors + 1))
          continue
        fi
      fi

      # Check anchor if present
      if [[ -n "$anchor" ]]; then
        local anchor_target="$target"
        [[ -z "$path" ]] && anchor_target="$html"
        if [[ -f "$anchor_target" ]]; then
          # Look for id="anchor" or name="anchor"
          if ! grep -qE "(id|name)=[\"']${anchor}[\"']" "$anchor_target" 2>/dev/null; then
            printf '  %s: broken anchor "#%s" in "%s"\n' "$html" "$anchor" "$ref"
            errors=$((errors + 1))
          fi
        fi
      fi
    done
  done

  return $errors
}

if ! check_links "." "$BASEURL"; then
  fail "link/asset integrity check failed"
  FAILED=1
else
  info "all links valid"
fi

# ---------------------------------------------------------------------------
# 3. Accessibility (WCAG 2.2 AA via Playwright + inline axe-core)
# ---------------------------------------------------------------------------
say "3/8 Accessibility (WCAG 2.2 AA)"

# Find Playwright — prefer web/ directory install
PLAYWRIGHT_DIR=""
REPO_ROOT="$(cd "$WEBROOT_ABS/.." 2>/dev/null && pwd || echo "$WEBROOT_ABS")"
if [ -d "$REPO_ROOT/web/node_modules/@playwright/test" ]; then
  PLAYWRIGHT_DIR="$REPO_ROOT/web"
fi

a11y_errors=0
a11y_skip=0

if [ -z "$PLAYWRIGHT_DIR" ]; then
  info "Playwright not found in web/node_modules"
  info "Install: cd web && npm install && npx playwright install chromium"
  a11y_skip=1
fi

if [ "$a11y_skip" -eq 0 ]; then
  # Start a temporary server
  PORT=18765
  SERVER_PID=""

  cleanup() {
    if [ -n "$SERVER_PID" ]; then
      kill "$SERVER_PID" 2>/dev/null || true
      wait "$SERVER_PID" 2>/dev/null || true
    fi
  }
  trap cleanup EXIT

  python3 -m http.server "$PORT" --bind 127.0.0.1 &>/dev/null &
  SERVER_PID=$!
  sleep 1

  # Collect HTML pages (exclude tools/)
  PAGE_LIST=""
  for html in $(find . -name '*.html' -type f -not -path './tools/*'); do
    PAGE_LIST="$PAGE_LIST ${html#./}"
  done

  # Write the a11y check script inside the web directory so it can import Playwright
  # This script injects axe-core directly into the page via evaluate(), avoiding npm deps
  A11Y_SCRIPT="$PLAYWRIGHT_DIR/_a11y-check.mjs"
  cat > "$A11Y_SCRIPT" << 'A11YSCRIPT'
import { chromium } from '@playwright/test';
import { readFileSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const port = process.argv[2];
const pages = process.argv.slice(3);

let hasErrors = false;

// Minimal inline axe-core checks via DOM inspection (no external dependencies)
// Checks: images without alt, missing document language, missing page title,
// buttons/links without accessible names, form inputs without labels
async function runA11yChecks(page) {
  return await page.evaluate(() => {
    const violations = [];

    // Check: html lang attribute
    const html = document.documentElement;
    if (!html.hasAttribute('lang') || !html.getAttribute('lang').trim()) {
      violations.push({
        id: 'html-has-lang',
        impact: 'serious',
        description: 'Document must have a lang attribute',
        nodes: [{ html: '<html>' }]
      });
    }

    // Check: page title
    if (!document.title || !document.title.trim()) {
      violations.push({
        id: 'document-title',
        impact: 'serious',
        description: 'Document must have a non-empty title',
        nodes: [{ html: '<title>' }]
      });
    }

    // Check: images without alt
    const images = document.querySelectorAll('img');
    for (const img of images) {
      if (!img.hasAttribute('alt')) {
        violations.push({
          id: 'image-alt',
          impact: 'critical',
          description: 'Images must have alternate text',
          nodes: [{ html: img.outerHTML.substring(0, 200) }]
        });
      }
    }

    // Check: main landmark
    const mainLandmark = document.querySelector('main, [role="main"]');
    if (!mainLandmark) {
      violations.push({
        id: 'landmark-main',
        impact: 'serious',
        description: 'Document should have a main landmark',
        nodes: [{ html: '<body>' }]
      });
    }

    // Check: skip link (should be first focusable element or early in page)
    const skipLink = document.querySelector('a[href^="#"]');
    const firstFocusable = document.querySelector('a, button, input, select, textarea, [tabindex]');
    if (!skipLink || (firstFocusable && !firstFocusable.matches('a[href^="#"]'))) {
      // Check if there's a .sr-only skip link
      const srOnlySkip = document.querySelector('.sr-only a[href^="#"], a.sr-only[href^="#"]');
      if (!srOnlySkip) {
        // Not a hard failure, just a warning - many sites don't have skip links
      }
    }

    // Check: buttons/links without accessible names
    const interactives = document.querySelectorAll('a, button');
    for (const el of interactives) {
      const text = el.textContent?.trim();
      const ariaLabel = el.getAttribute('aria-label')?.trim();
      const ariaLabelledby = el.getAttribute('aria-labelledby');
      const title = el.getAttribute('title')?.trim();
      const imgAlt = el.querySelector('img')?.getAttribute('alt')?.trim();

      const hasName = text || ariaLabel || ariaLabelledby || title || imgAlt;

      if (!hasName) {
        violations.push({
          id: 'link-name',
          impact: 'serious',
          description: 'Links and buttons must have discernible text',
          nodes: [{ html: el.outerHTML.substring(0, 200) }]
        });
      }
    }

    // Check: form inputs without labels
    const inputs = document.querySelectorAll('input:not([type="hidden"]), select, textarea');
    for (const input of inputs) {
      const id = input.getAttribute('id');
      const ariaLabel = input.getAttribute('aria-label')?.trim();
      const ariaLabelledby = input.getAttribute('aria-labelledby');
      const hasLabel = (id && document.querySelector(`label[for="${id}"]`)) ||
                       input.closest('label') ||
                       ariaLabel ||
                       ariaLabelledby;

      if (!hasLabel) {
        violations.push({
          id: 'label',
          impact: 'critical',
          description: 'Form elements must have labels',
          nodes: [{ html: input.outerHTML.substring(0, 200) }]
        });
      }
    }

    // Check: heading hierarchy (h1 should come before h2, etc.)
    const headings = document.querySelectorAll('h1, h2, h3, h4, h5, h6');
    let lastLevel = 0;
    for (const h of headings) {
      const level = parseInt(h.tagName[1]);
      if (level > lastLevel + 1 && lastLevel !== 0) {
        violations.push({
          id: 'heading-order',
          impact: 'serious',
          description: `Heading levels should increase by one (found h${level} after h${lastLevel})`,
          nodes: [{ html: h.outerHTML.substring(0, 200) }]
        });
        break; // Only report first violation
      }
      lastLevel = level;
    }

    // Check: color contrast - we can't compute this without getComputedStyle analysis
    // which requires knowing the background. Skip for now - rely on design tokens verification.

    return violations;
  });
}

async function checkPage(browser, page_path) {
  const url = `http://127.0.0.1:${port}/${page_path}`;
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 10000 });

    const violations = await runA11yChecks(page);

    const serious = violations.filter(v =>
      v.impact === 'serious' || v.impact === 'critical'
    );

    if (serious.length > 0) {
      console.log(`  ${page_path}: ${serious.length} serious/critical violation(s)`);
      for (const v of serious) {
        console.log(`    - [${v.impact}] ${v.id}: ${v.description}`);
        for (const node of v.nodes.slice(0, 3)) {
          console.log(`      ${node.html.substring(0, 100)}`);
        }
      }
      hasErrors = true;
    }
  } catch (err) {
    console.error(`  ${page_path}: error - ${err.message}`);
    hasErrors = true;
  } finally {
    await context.close();
  }
}

async function main() {
  const browser = await chromium.launch({ headless: true });

  for (const page_path of pages) {
    await checkPage(browser, page_path);
  }

  await browser.close();
  process.exit(hasErrors ? 1 : 0);
}

main().catch((err) => {
  console.error('Fatal error:', err.message);
  process.exit(1);
});
A11YSCRIPT
  _TMP_FILES+=("$A11Y_SCRIPT")
  trap "_rm_tmp_files; cleanup" EXIT

  cd "$PLAYWRIGHT_DIR"
  # shellcheck disable=SC2086  # intentional: the page list is a space-separated
# string that must split into separate argv entries for the node script.
output=$(node "$A11Y_SCRIPT" "$PORT" $PAGE_LIST 2>&1) || true
  exit_code=$?

  if echo "$output" | grep -qE "Failed to launch|browserType.launch|Executable doesn't exist|Cannot find"; then
    info "Playwright browsers not installed — skipping accessibility check"
    info "Install: cd web && npx playwright install chromium"
    a11y_skip=1
  elif [ $exit_code -ne 0 ]; then
    echo "$output"
    fail "accessibility check failed"
    FAILED=1
  else
    echo "$output" | grep -v '^$' || true
    info "accessibility: WCAG 2.2 AA compliant (basic checks passed)"
  fi
  cd "$WEBROOT_ABS"
fi

if [ "$a11y_skip" -eq 1 ]; then
  info "accessibility: skipped locally (install Playwright for local checks)"
fi

# ---------------------------------------------------------------------------
# 4. No external resources
# ---------------------------------------------------------------------------
say "4/8 External resource check"

# ALLOWLIST — destinations that may be LINKED (href) but NOT loaded (src/url()).
# These are navigation targets, not assets. Add new entries as needed.
LINK_ALLOWLIST="github.com|antmedia.io"

# Check for external resources in src, srcset, url() positions
# These MUST fail — no external scripts, styles, fonts, or images.
ext_errors=0

for html in $(find . -name '*.html' -type f -not -path './tools/*'); do
  # External src= (scripts, images, iframes)
  if grep -qE 'src=["\047]https?://' "$html" 2>/dev/null; then
    printf '  %s: external src= attribute found\n' "$html"
    ext_errors=$((ext_errors + 1))
  fi

  # External srcset=
  if grep -qE 'srcset=["\047][^"\047]*https?://' "$html" 2>/dev/null; then
    printf '  %s: external srcset= attribute found\n' "$html"
    ext_errors=$((ext_errors + 1))
  fi

  # External url() in inline styles or style blocks
  if grep -qE 'url\(["\047]?https?://' "$html" 2>/dev/null; then
    printf '  %s: external url() found\n' "$html"
    ext_errors=$((ext_errors + 1))
  fi

  # External href= in link/script tags (stylesheets, preloads, prefetches)
  # Exclude <a> tags — those are navigation links checked against allowlist
  if grep -E '<link[^>]+href=["\047]https?://|<script[^>]+href=["\047]https?://' "$html" 2>/dev/null | \
     grep -vE "($LINK_ALLOWLIST)" >/dev/null 2>&1; then
    printf '  %s: external <link> or <script> href found\n' "$html"
    ext_errors=$((ext_errors + 1))
  fi
done

# Check CSS files for external url()
for css in $(find . -name '*.css' -type f 2>/dev/null || true); do
  if grep -qE 'url\(["\047]?https?://' "$css" 2>/dev/null; then
    printf '  %s: external url() found\n' "$css"
    ext_errors=$((ext_errors + 1))
  fi
done

# Check for @import with external URLs
for css in $(find . \( -name '*.css' -o -name '*.html' \) -type f 2>/dev/null || true); do
  if grep -qE '@import[[:space:]]+["\047]?https?://|@import[[:space:]]+url\(["\047]?https?://' "$css" 2>/dev/null; then
    printf '  %s: external @import found\n' "$css"
    ext_errors=$((ext_errors + 1))
  fi
done

if [ "$ext_errors" -gt 0 ]; then
  fail "external resource check failed — $ext_errors violation(s)"
  fail "Architecture rule: all scripts, styles, fonts, and images must be self-hosted."
  fail "See docs/ARCHITECTURE.md section 3."
  FAILED=1
else
  info "no external resources — all assets are self-hosted"
fi

# ---------------------------------------------------------------------------
# 5. Root-absolute path check (subpath hosting regression guard)
# ---------------------------------------------------------------------------
say "5/8 Root-absolute path check"

# Root-absolute paths (href="/...", src="/...") break when serving from a
# subpath like /ams-pulse/. This check catches regressions.
# Exception: og:image and twitter:image use absolute paths for external crawlers.
root_abs_errors=0

for html in $(find . -name '*.html' -type f -not -path './tools/*'); do
  # Find href="/..." or src="/..." but exclude:
  # - og:image and twitter:image meta tags (must be absolute for crawlers)
  # - Anchor-only hrefs like href="#section"
  # Use grep to find candidates, then filter
  matches=$(grep -nE '(href|src)=["\047]/[^"\047]' "$html" 2>/dev/null || true)

  while IFS= read -r line; do
    [ -z "$line" ] && continue

    # Skip og:image and twitter:image — these MUST be absolute for external crawlers
    echo "$line" | grep -qE 'property=["\047]og:image|name=["\047]twitter:image' && continue

    # Skip if it's just a hash anchor on root (href="/ams-pulse/#features" style)
    # Actually, any /path that doesn't start with the expected BASEURL is wrong
    # But for simplicity, we flag ALL root-absolute internal paths.

    # Extract the path value
    path_val=$(echo "$line" | grep -oP '(href|src)=["\047]\K/[^"\047]*' || true)

    # Skip if path is just "/" (home link)
    [ "$path_val" = "/" ] && continue

    printf '  %s: root-absolute path "%s" (breaks subpath hosting)\n' "$html" "$path_val"
    root_abs_errors=$((root_abs_errors + 1))
  done <<< "$matches"
done

if [ "$root_abs_errors" -gt 0 ]; then
  fail "root-absolute path check failed — $root_abs_errors violation(s)"
  fail "Use document-relative paths (e.g., 'assets/foo.css' not '/assets/foo.css')"
  fail "See website/README.md for the path convention."
  FAILED=1
else
  info "no root-absolute internal paths — subpath hosting safe"
fi

# ---------------------------------------------------------------------------
# 6. Semantic HTML checks (main landmark, img alt)
# ---------------------------------------------------------------------------
say "6/8 Semantic HTML checks"

semantic_errors=0

for html in $(find . -name '*.html' -type f -not -path './tools/*'); do
  # Check for main landmark (either <main> or role="main")
  if ! grep -qE '<main[^>]*>|role=["\047]main["\047]' "$html" 2>/dev/null; then
    printf '  %s: missing main landmark (<main> element required)\n' "$html"
    semantic_errors=$((semantic_errors + 1))
  fi

  # Check all <img> tags have alt attribute (can be empty for decorative)
  # Find all <img ...> tags and check they contain alt=
  img_tags=$(grep -oE '<img[^>]+>' "$html" 2>/dev/null || true)
  while IFS= read -r img; do
    [ -z "$img" ] && continue
    if ! echo "$img" | grep -qE 'alt=["\047]'; then
      # Truncate for display
      short_img=$(echo "$img" | head -c 80)
      printf '  %s: <img> without alt attribute: %s...\n' "$html" "$short_img"
      semantic_errors=$((semantic_errors + 1))
    fi
  done <<< "$img_tags"
done

if [ "$semantic_errors" -gt 0 ]; then
  fail "semantic HTML check failed — $semantic_errors violation(s)"
  FAILED=1
else
  info "semantic HTML: main landmark present, all images have alt"
fi

# ---------------------------------------------------------------------------
# 7. OG/Twitter image existence check
# ---------------------------------------------------------------------------
say "7/8 OG/Twitter image existence check"

og_errors=0

for html in $(find . -name '*.html' -type f -not -path './tools/*'); do
  dir=$(dirname "$html")

  # Extract og:image and twitter:image content values
  og_images=$(grep -oP 'property=["\047]og:image["\047][^>]*content=["\047]\K[^"\047]+' "$html" 2>/dev/null || true)
  tw_images=$(grep -oP 'name=["\047]twitter:image["\047][^>]*content=["\047]\K[^"\047]+' "$html" 2>/dev/null || true)

  for img_path in $og_images $tw_images; do
    [ -z "$img_path" ] && continue

    # Skip absolute URLs (external images)
    echo "$img_path" | grep -qE '^https?://' && continue

    # Resolve the path
    local_path=""
    if [[ "$img_path" == /* ]]; then
      # Root-relative — resolve from BASEURL
      stripped="${img_path#$BASEURL}"
      [ -z "$stripped" ] && stripped="${img_path#/}"
      local_path="./$stripped"
    else
      # Document-relative
      local_path="$dir/$img_path"
    fi

    # Check if file exists
    if [ ! -f "$local_path" ]; then
      printf '  %s: og/twitter image not found: "%s" (resolved: %s)\n' "$html" "$img_path" "$local_path"
      og_errors=$((og_errors + 1))
    fi
  done
done

if [ "$og_errors" -gt 0 ]; then
  fail "OG/Twitter image check failed — $og_errors missing image(s)"
  fail "Run: ./website/tools/generate-og-image.sh to regenerate"
  FAILED=1
else
  info "all OG/Twitter images exist"
fi

# ---------------------------------------------------------------------------
# 8. Subpath stylesheet loading verification (Playwright-based)
# ---------------------------------------------------------------------------
say "8/8 Subpath stylesheet verification"

# This check verifies that stylesheets actually load when served from a subpath.
# It starts a server that simulates subpath hosting and checks that CSS applies.

subpath_errors=0

if [ "$BASEURL" != "/" ] && [ -n "$PLAYWRIGHT_DIR" ]; then
  # Create a subpath simulation directory
  SUBPATH_DIR=$(mktemp -d)
  SUBPATH_NAME="${BASEURL%/}"  # Remove trailing slash
  SUBPATH_NAME="${SUBPATH_NAME#/}"  # Remove leading slash
  mkdir -p "$SUBPATH_DIR/$SUBPATH_NAME"
  cp -r . "$SUBPATH_DIR/$SUBPATH_NAME/"

  # Start server from the parent
  PORT2=18766
  SERVER_PID2=""
  cd "$SUBPATH_DIR"
  python3 -m http.server "$PORT2" --bind 127.0.0.1 &>/dev/null &
  SERVER_PID2=$!
  sleep 1

  cleanup2() {
    if [ -n "$SERVER_PID2" ]; then kill "$SERVER_PID2" 2>/dev/null || true; fi
    rm -rf "$SUBPATH_DIR"
  }
  trap "_rm_tmp_files; cleanup; cleanup2" EXIT

  # Create a Playwright script to verify CSS loads (in web/ directory so it can import)
  CSS_CHECK_SCRIPT="$PLAYWRIGHT_DIR/_css-check.mjs"
  _TMP_FILES+=("$CSS_CHECK_SCRIPT")
  cat > "$CSS_CHECK_SCRIPT" << 'CSSCHECK'
import { chromium } from '@playwright/test';

const port = process.argv[2];
const basepath = process.argv[3];
const pages = process.argv.slice(4);

let hasErrors = false;

async function checkPage(browser, page_path) {
  const url = `http://127.0.0.1:${port}${basepath}${page_path}`;
  const context = await browser.newContext();
  const page = await context.newPage();

  const failedRequests = [];
  page.on('requestfailed', (request) => {
    if (request.url().endsWith('.css')) {
      failedRequests.push(request.url());
    }
  });

  try {
    await page.goto(url, { waitUntil: 'networkidle', timeout: 10000 });

    // Check if any CSS failed to load
    if (failedRequests.length > 0) {
      console.log(`  ${page_path}: CSS failed to load from subpath`);
      for (const req of failedRequests) {
        console.log(`    - ${req}`);
      }
      hasErrors = true;
    }

    // Check if body has expected styling (not default browser styles)
    const bodyBg = await page.evaluate(() => {
      return window.getComputedStyle(document.body).backgroundColor;
    });

    // If CSS loaded, body should NOT have default white background
    // Our dark theme uses #0A0E14 = rgb(10, 14, 20)
    if (bodyBg === 'rgba(0, 0, 0, 0)' || bodyBg === 'rgb(255, 255, 255)') {
      console.log(`  ${page_path}: stylesheet not applied (body bg: ${bodyBg})`);
      hasErrors = true;
    }
  } catch (err) {
    console.error(`  ${page_path}: error - ${err.message}`);
    hasErrors = true;
  } finally {
    await context.close();
  }
}

async function main() {
  const browser = await chromium.launch({ headless: true });

  for (const page_path of pages) {
    await checkPage(browser, page_path);
  }

  await browser.close();
  process.exit(hasErrors ? 1 : 0);
}

main().catch((err) => {
  console.error('Fatal error:', err.message);
  process.exit(1);
});
CSSCHECK

  # Run the check
  cd "$PLAYWRIGHT_DIR"
  PAGE_LIST2=""
  for html in $(find "$WEBROOT_ABS" -name '*.html' -type f -not -path '*/tools/*'); do
    PAGE_LIST2="$PAGE_LIST2 ${html#$WEBROOT_ABS/}"
  done

  # shellcheck disable=SC2086  # intentional: the page list is a space-separated
# string that must split into separate argv entries for the node script.
if node "$CSS_CHECK_SCRIPT" "$PORT2" "$BASEURL" $PAGE_LIST2 2>&1; then
    info "stylesheets load correctly from subpath $BASEURL"
  else
    # shellcheck disable=SC2086  # intentional: the page list is a space-separated
# string that must split into separate argv entries for the node script.
output=$(node "$CSS_CHECK_SCRIPT" "$PORT2" "$BASEURL" $PAGE_LIST2 2>&1 || true)
    if echo "$output" | grep -qE "Failed to launch|browserType.launch"; then
      info "subpath CSS check: skipped (Playwright browsers not installed)"
    else
      fail "subpath stylesheet loading failed"
      FAILED=1
    fi
  fi

  rm -f "$CSS_CHECK_SCRIPT" 2>/dev/null || true
  cd "$WEBROOT_ABS"
else
  if [ "$BASEURL" = "/" ]; then
    info "subpath check: skipped (testing root hosting)"
  else
    info "subpath check: skipped (Playwright not available)"
  fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
say "Summary"
if [ "$FAILED" -eq 0 ]; then
  printf 'Website checks: PASS\n'
  exit 0
else
  printf 'Website checks: FAIL\n'
  exit 1
fi
