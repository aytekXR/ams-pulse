#!/usr/bin/env bash
# =============================================================================
# generate-og-image.sh — render the OG image using Playwright screenshot.
#
# This script renders og-image-template.html at exactly 1200x630 and captures
# it to website/assets/og-image.png. The template uses only inline styles and
# SVG — no external assets.
#
# Prerequisites:
#   - Node.js 18+
#   - Playwright installed under web/ (npm install in web/)
#   - Playwright browsers installed (npx playwright install chromium)
#
# Usage:
#   cd website/tools
#   ./generate-og-image.sh
#
# Or from repo root:
#   ./website/tools/generate-og-image.sh
#
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WEBSITE_DIR="$REPO_ROOT/website"
WEB_DIR="$REPO_ROOT/web"
TEMPLATE="$SCRIPT_DIR/og-image-template.html"
OUTPUT="$WEBSITE_DIR/assets/og-image.png"

echo "=== Generating OG image ==="
echo "Template: $TEMPLATE"
echo "Output:   $OUTPUT"

# Verify template exists
if [ ! -f "$TEMPLATE" ]; then
  echo "ERROR: Template not found: $TEMPLATE" >&2
  exit 1
fi

# Verify Playwright is available
if [ ! -d "$WEB_DIR/node_modules/@playwright/test" ]; then
  echo "ERROR: Playwright not installed. Run: cd web && npm install" >&2
  exit 1
fi

# Create a temporary Node.js script in the web directory to access its node_modules
TEMP_SCRIPT="$WEB_DIR/_og-generator.mjs"
# Single quotes: $TEMP_SCRIPT must expand when the trap FIRES, not when it is
# installed. With double quotes the path is baked in at install time, which is
# usually the same string — until someone moves the assignment below the trap.
# shellcheck disable=SC2064  # intentional: see above, the value is already final
trap 'rm -f "$TEMP_SCRIPT"' EXIT

cat > "$TEMP_SCRIPT" << PLAYWRIGHT_SCRIPT
import { chromium } from '@playwright/test';

const templatePath = '$TEMPLATE';
const outputPath = '$OUTPUT';

async function generateOgImage() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1200, height: 630 },
    deviceScaleFactor: 1,
  });

  const page = await context.newPage();

  // Load the template as a file URL
  const templateUrl = 'file://' + templatePath;
  await page.goto(templateUrl, { waitUntil: 'networkidle' });

  // Wait for fonts to load (system fonts should be immediate)
  await page.waitForTimeout(100);

  // Take screenshot at exact viewport size
  await page.screenshot({
    path: outputPath,
    type: 'png',
    fullPage: false, // Only the viewport
    clip: { x: 0, y: 0, width: 1200, height: 630 }
  });

  await browser.close();

  console.log('Generated: ' + outputPath);
}

generateOgImage().catch((err) => {
  console.error('Error generating OG image:', err);
  process.exit(1);
});
PLAYWRIGHT_SCRIPT

# Run from the web directory where Playwright is installed
cd "$WEB_DIR"
node "$TEMP_SCRIPT"

# Verify output
if [ -f "$OUTPUT" ]; then
  SIZE=$(stat -c%s "$OUTPUT" 2>/dev/null || stat -f%z "$OUTPUT" 2>/dev/null)
  echo "Success: $OUTPUT ($SIZE bytes)"
else
  echo "ERROR: Output file not created" >&2
  exit 1
fi
