# Pulse Public Website

Static marketing and documentation site for Pulse.

Last updated: 2026-07-28

## What this is

The public-facing website for Pulse, deployed to GitHub Pages at
`https://aytekxr.github.io/ams-pulse/` and optionally self-hosted via nginx.

This is a **static site** with no build step. HTML, CSS, and assets are
served directly. The design system is hand-coded from
`brandkit/design-system/tokens.json`.

## Directory structure

```
website/
  index.html          # Home page
  beta/               # iOS beta signup page
  privacy/            # Privacy policy
  support/            # Support page
  terms/              # Terms of service
  assets/
    pulse.css         # Main stylesheet (tokens from brandkit)
    favicon.svg       # Browser tab icon
    favicon-*.png     # Legacy PNG favicons
    pulse-mark.svg    # Standalone logo mark (dark bg)
    pulse-mark-light.svg    # Logo mark (light bg)
    pulse-logo-primary-*.svg  # Full logo with wordmark
    og-image.png      # Open Graph image (1200x630, generated)
  tools/
    og-image-template.html  # Source template for OG image
    generate-og-image.sh    # Playwright-based generator script
  tests/
    run-checks.sh     # Validation script (8 checks)
```

## Link and asset reference rule

The site is served from a **subpath** on GitHub Pages:
`https://aytekxr.github.io/ams-pulse/`

To work correctly, all internal links and assets use **document-relative paths**:

```html
<!-- CORRECT: document-relative -->
<link rel="stylesheet" href="assets/pulse.css">
<img src="assets/pulse-mark.svg" alt="Pulse">
<a href="beta/">iOS Beta</a>

<!-- WRONG: root-relative (breaks on subpath) -->
<link rel="stylesheet" href="/assets/pulse.css">
```

**Why not root-relative?** Root-relative links resolve from the domain root.
On GitHub Pages with a repository subpath, `/assets/pulse.css` resolves to
`aytekxr.github.io/assets/pulse.css` (wrong) instead of
`aytekxr.github.io/ams-pulse/assets/pulse.css` (correct).

**Exception:** `og:image` and `twitter:image` meta tags must use root-relative
paths (`/assets/og-image.png`) because external crawlers resolve them from the
page URL, and document-relative paths would break when shared.

The check script enforces this rule and will fail on root-absolute internal
hrefs or srcs (except the meta tag exceptions).

## No external resources

Per `docs/ARCHITECTURE.md` section 3, **all assets must be self-hosted**.
This means:

- No CDN links (Google Fonts, unpkg, cdnjs)
- No external images
- No remote scripts
- No @import from external URLs

The check script enforces this at build time.

## Running checks locally

The validation script runs 8 checks:

```bash
cd website
./tests/run-checks.sh .              # root hosting
./tests/run-checks.sh . /ams-pulse/  # subpath hosting (GitHub Pages)
```

### What the checks validate

1. **HTML validity** — Well-formed HTML5 via html-validate
2. **Link integrity** — All internal hrefs and srcs resolve; anchor links
   resolve to matching `id` attributes
3. **Accessibility** — WCAG 2.2 AA compliance via Playwright (checks lang,
   title, main landmark, img alt, heading order, link names, form labels)
4. **No external resources** — No `src=`, `srcset=`, or `url()` pointing to
   external origins
5. **Root-absolute path check** — Catches `/assets/...` paths that break
   subpath hosting (regression guard)
6. **Semantic HTML** — Main landmark present, all images have alt
7. **OG/Twitter image existence** — Meta image paths resolve to real files
8. **Subpath stylesheet verification** — CSS actually loads when served from
   a subpath (Playwright-based, runs only with baseurl argument)

### Prerequisites for local checks

The accessibility and subpath checks use Playwright from `web/`:

```bash
cd web
npm install
npx playwright install chromium
```

Without Playwright, checks 3 and 8 skip gracefully with a warning.

## Regenerating the OG image

The Open Graph image (`assets/og-image.png`) is generated from an HTML
template using Playwright:

```bash
./website/tools/generate-og-image.sh
```

This renders `tools/og-image-template.html` at 1200x630 and screenshots it.
The template uses inline styles and SVG from the brandkit — no external assets.

**When to regenerate:**
- If the brand mark SVG changes
- If the tagline or product name changes
- If the color tokens change

The check script will fail if the og-image.png is missing.

## Deployment

### GitHub Pages (automatic)

Push to `main` with changes in `website/`. The workflow at
`.github/workflows/pages.yml` runs checks and deploys.

Site URL: https://aytekxr.github.io/ams-pulse/

**Manual deployment from a feature branch** (for smoke testing):

```bash
gh workflow run pages.yml -f branch=feat/my-branch
```

This deploys the specified branch to the same Pages URL. Use for pre-merge
testing only — main should be the production branch.

### Self-hosted (nginx)

See `deploy/nginx/pulse-website.conf`. In short:

```bash
# Copy files
sudo install -d /var/www/pulse-website
sudo cp -r website/* /var/www/pulse-website/

# Install config (edit server_name and ssl paths first)
sudo cp deploy/nginx/pulse-website.conf /etc/nginx/sites-available/
sudo ln -sfn /etc/nginx/sites-available/pulse-website.conf /etc/nginx/sites-enabled/

# Get certificate
sudo certbot certonly --webroot -w /var/www/certbot -d your-domain.com

# Reload
sudo nginx -t && sudo systemctl reload nginx
```

## Design notes

See `DESIGN-NOTES.md` for:

- Font stack (IBM Plex, self-hosting gap)
- Asset provenance (copied from `brandkit/logo/`)
- Verified WCAG contrast ratios
- Status chip accessibility (shape + color)
- Motion policy
- Responsive breakpoints
- Type and spacing scales
