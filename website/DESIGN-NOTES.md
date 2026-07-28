# Pulse Website — Design Notes

Visual foundation for the Pulse public website.
Generated from `brandkit/design-system/tokens.json` v1.0.0.

Last updated: 2026-07-28 — contrast table recomputed in D-186 after the muted-token fixes

## 1. Font Stack — Known Gap

The brandkit specifies IBM Plex Sans and IBM Plex Mono (both OFL-licensed).
However, **IBM Plex is not self-hosted anywhere in this repository**. The
product UI (`web/src/styles/global.css`) imports the fonts via
`@fontsource/ibm-plex-*`, which Vite bundles from `node_modules` — but the
marketing site is static HTML with no build step.

**Current behavior:** The CSS declares the full font stack from tokens.json:

```css
--font-sans: 'IBM Plex Sans', 'Helvetica Neue', Helvetica, Arial, sans-serif;
--font-mono: 'IBM Plex Mono', ui-monospace, 'SF Mono', Menlo, monospace;
```

Without the font files, the browser falls back to Helvetica Neue/Arial for
body text and the system monospace for code/labels.

**Fix:** Self-host the OFL woff2 files in `website/assets/fonts/`. Download
from [Google Fonts](https://fonts.google.com/specimen/IBM+Plex+Sans) or the
[IBM Plex repo](https://github.com/IBM/plex), then add `@font-face` rules at
the top of `pulse.css`. Do **not** add a Google Fonts CDN link — external
network assets violate the repo architecture rules (docs/ARCHITECTURE.md
section 3).

## 2. Assets Copied into website/assets/

| File | Source | Purpose |
|------|--------|---------|
| `favicon.svg` | `brandkit/logo/favicon.svg` | Browser tab icon (SVG, 32x32 viewBox). Dark background (#0A0E14) + signal pulse stroke. |
| `favicon-16.png` | `brandkit/logo/png/favicon-16.png` | Legacy ICO fallback (16x16). |
| `favicon-32.png` | `brandkit/logo/png/favicon-32.png` | Retina tab icon (32x32). |
| `favicon-48.png` | `brandkit/logo/png/favicon-48.png` | Windows tile (48x48). |
| `pulse-mark.svg` | `brandkit/logo/pulse-mark.svg` | Standalone mark for dark backgrounds (64x64 viewBox). Used in nav, hero product frame. |
| `pulse-mark-light.svg` | `brandkit/logo/pulse-mark-light.svg` | Standalone mark for light backgrounds. White fill with border (#D7DEE5). |
| `pulse-logo-primary-dark.svg` | `brandkit/logo/pulse-logo-primary-dark.svg` | Full logo (mark + wordmark) for dark backgrounds. |
| `pulse-logo-primary-light.svg` | `brandkit/logo/pulse-logo-primary-light.svg` | Full logo for light backgrounds. Dark wordmark (#10181F). |
| `og-image.png` | Generated | Open Graph image (1200x630). Generated from `tools/og-image-template.html` via Playwright. |

### Asset selection rationale

- **favicon.svg** is the SVG favicon for modern browsers. Includes the full
  heartbeat/pulse mark with rounded corners — reads at 16px in the browser
  tab.
- **pulse-mark.svg** (dark) and **pulse-mark-light.svg** are the standalone
  mark files. The site uses the dark mark in the nav and hero since the
  default theme is dark. Light-theme support uses `pulse-mark-light.svg`.
- **pulse-logo-primary-{dark,light}.svg** are the full logos (mark +
  wordmark). Needed if the site adds a larger brand lockup.
- **PNG favicons** (16/32/48) provide fallbacks for browsers that do not
  support SVG favicons.
- **og-image.png** is the Open Graph / Twitter Card image shown when the site
  is shared. Generated via `./website/tools/generate-og-image.sh`.

## 3. Contrast Ratios — Verified

All ratios computed with the WCAG 2.x sRGB relative-luminance formula.
Script used: `/tmp/contrast.py` (runs `python3 contrast.py`).

### Dark Theme (default)

| Foreground | Background | Ratio | WCAG Level |
|------------|------------|------:|:-----------|
| textPrimary #E8EEF4 | bg #0A0E14 | 16.55:1 | AAA |
| textPrimary #E8EEF4 | surface #10161D | 15.56:1 | AAA |
| textPrimary #E8EEF4 | raised #161E27 | 14.38:1 | AAA |
| textSecondary #9FB0C0 | bg #0A0E14 | 8.70:1 | AAA |
| textSecondary #9FB0C0 | surface #10161D | 8.18:1 | AAA |
| textMuted #5C6F80 | bg #0A0E14 | 3.72:1 | **3:1 UI only** |
| textMuted #5C6F80 | surface #10161D | 3.50:1 | **3:1 UI only** |
| signal #2CE5A7 | bg #0A0E14 | 11.86:1 | AAA |
| signal #2CE5A7 | surface #10161D | 11.15:1 | AAA |
| signalHover #4FEDB9 | bg #0A0E14 | 13.03:1 | AAA |
| onSignal #0A0E14 | signal #2CE5A7 | 11.86:1 | AAA (buttons) |
| warning #FFB224 | bg #0A0E14 | 10.73:1 | AAA |
| warning #FFB224 | surface #10161D | 10.09:1 | AAA |
| critical #FF5C68 | bg #0A0E14 | 6.43:1 | AA |
| critical #FF5C68 | surface #10161D | 6.05:1 | AA |
| info #58A6FF | bg #0A0E14 | 7.66:1 | AAA |
| info #58A6FF | surface #10161D | 7.20:1 | AAA |

### Light Theme

| Foreground | Background | Ratio | WCAG Level |
|------------|------------|------:|:-----------|
| textPrimary #10181F | bg #F7F9FA | 16.96:1 | AAA |
| textPrimary #10181F | surface #FFFFFF | 17.91:1 | AAA |
| textSecondary #4A5B6B | bg #F7F9FA | 6.63:1 | AA |
| textSecondary #4A5B6B | surface #FFFFFF | 7.00:1 | AAA |
| textMuted #6B7B88 | bg #F7F9FA | 4.13:1 | **3:1 UI only** |
| textMuted #6B7B88 | surface #FFFFFF | 4.36:1 | **3:1 UI only** |
| signal #087A59 | bg #F7F9FA | 5.05:1 | AA |
| signal #087A59 | surface #FFFFFF | 5.33:1 | AA |
| signalHover #07684C | bg #F7F9FA | 6.43:1 | AA |
| signalHover #07684C | surface #FFFFFF | 6.79:1 | AA |
| onSignal #FFFFFF | signal #087A59 | 5.33:1 | AA (buttons) |
| onSignal #FFFFFF | signalHover #07684C | 6.79:1 | AA (buttons) |
| healthy #0BA678 | surface #FFFFFF | 3.12:1 | **3:1 graphic only** |
| warning #B45309 | bg #F7F9FA | 4.76:1 | AA |
| warning #B45309 | surface #FFFFFF | 5.02:1 | AA |
| critical #DC2626 | bg #F7F9FA | 4.57:1 | AA |
| critical #DC2626 | surface #FFFFFF | 4.83:1 | AA |
| info #1B5EAD | bg #F7F9FA | 6.12:1 | AA |
| info #1B5EAD | surface #FFFFFF | 6.46:1 | AA |
| info #1B5EAD | info-tint #E8EFF7 | 5.57:1 | AA |

### Notes on "UI only" rows

Per `brandkit/documentation/design-rationale.md` section 2:

- **textMuted (#5C6F80 dark / #6B7B88 light)** is below the 4.5:1 AA bar for
  normal body text. It may only be used for non-text UI (borders, dividers,
  decorative icons) where the 3:1 bar applies. For labels, captions, or any
  readable text, use textSecondary instead.

- **healthy #0BA678** in light theme is 3.12:1 on white — passing the 3:1
  graphics bar but failing for text. It is a status graphic color only
  (dots/badges). The darker signal #087A59 is used for text links and CTAs.

### Brandkit defect acknowledgement

The textMuted color was documented in the brandkit's WCAG table as "~4.6:1 —
AA — labels/captions only" but the true ratio is 3.72:1 (dark) and 4.13:1
(light), both below AA. The design-rationale file now carries the corrected
values and the warning that textMuted is invalid for text. This stylesheet
never uses textMuted for text — only for decorative borders and icons.

## 4. Status Chip Accessibility

Per `brandkit/documentation/design-rationale.md` section 2:

> State is never encoded by hue alone. Healthy/warn/critical/offline each
> pair a fixed shape (dot / diamond / triangle / outlined dot) with the
> color.

The `.chip-*` classes in `pulse.css` implement this via `::before`
pseudo-elements:

| Status | Shape | Color variable |
|--------|-------|----------------|
| Healthy/Live | Filled circle | `--color-healthy` |
| Warning/Degraded | Diamond (rotated square) | `--color-warning` |
| Critical/Firing | Triangle | `--color-critical` |
| Offline/Unknown | Outlined circle | `--color-neutral` |

This ensures colorblind users can distinguish status without relying on
hue differentiation.

## 5. Motion

Per `tokens.json`:

> Live data updates fade, never slide. No bounce.

The stylesheet uses `--motion-fast: 120ms ease-out` and
`--motion-base: 200ms ease-out` only for interactive state changes (hover,
focus). Animations are limited to opacity fades.

`prefers-reduced-motion: reduce` collapses all motion to 0ms.

## 6. Responsive Breakpoints

Mobile-first. The grid collapses at 640px; nav links hide at 768px; hero
title scales down at 768px and 480px. No horizontal scroll at any viewport
width down to 320px.

Wide content (tables, code blocks) uses `overflow-x: auto` containers so
the page body never scrolls horizontally.

## 7. Type Scale

From `tokens.json type.*`:

| Style | Size | Line | Weight | Notes |
|-------|-----:|-----:|:-------|:------|
| display | 44px | 48px | 700 | Hero headlines |
| h1 | 32px | 40px | 700 | Section titles |
| h2 | 22px | 28px | 600 | Card headings |
| h3 | 16px | 24px | 600 | Subheadings |
| body | 14px | 22px | 400 | Default text |
| caption | 12px | 16px | 500 | Help text |
| label | 11px | 16px | 500 | Mono, uppercase, 0.1em tracking |
| metric | 40px | 44px | 700 | tabular-nums |

The hero title uses a larger 64px on desktop (from the hi-fi), scaling down
to 44px (tablet) and 32px (mobile).

## 8. Spacing Scale

From `tokens.json space`:

```
--space-1:  4px
--space-2:  8px
--space-3:  12px
--space-4:  16px
--space-5:  24px
--space-6:  32px
--space-7:  48px
--space-8:  64px
--space-9:  96px
```

Section vertical rhythm: `--space-9` (96px) top/bottom. Card padding:
`--space-6` (32px). Grid gap: `--space-5` (24px).

## 9. OG Image Generation

The Open Graph image (`assets/og-image.png`) is generated from an HTML
template using Playwright screenshot. This avoids needing image editing
tools or external rasterizers.

**Template:** `tools/og-image-template.html`
**Generator:** `tools/generate-og-image.sh`

The template:
- Uses inline styles with tokens.json values
- Embeds the pulse-mark SVG directly (no external assets)
- Renders at exactly 1200x630 (OG standard size)
- Uses the dark theme (#0A0E14 background)

To regenerate:

```bash
./website/tools/generate-og-image.sh
```

This requires Playwright installed under `web/`:

```bash
cd web && npm install && npx playwright install chromium
```

## 10. Verification Checklist

What is verified by automated checks (`tests/run-checks.sh`):

- [x] HTML validity (html-validate)
- [x] Internal link integrity (href/src resolve)
- [x] No external resources (no CDN, no remote fonts)
- [x] Document-relative paths (no root-absolute `/assets/...`)
- [x] Main landmark present on every page
- [x] All images have alt attributes
- [x] OG/Twitter images exist
- [x] Stylesheets load from subpath (/ams-pulse/)
- [x] Basic accessibility (lang, title, headings, link names, labels)

What is NOT verified automatically (requires manual review):

- [ ] IBM Plex fonts self-hosted (currently falling back to system fonts)
- [ ] Color contrast in rendered output (relies on tokens.json verification)
- [ ] Full axe-core WCAG 2.2 AA audit (basic checks only, not full axe)
- [ ] Visual regression (no screenshot comparison)
- [ ] Mobile responsive behavior below 320px
