# Proposal: `color.light.linkBody` token in tokens.json

**Status:** PROPOSAL — awaiting operator/designer sign-off  
**Raised by:** S16 verifier (D-079)  
**Scope:** `brandkit/design-system/tokens.json` (add one key)  
**Companion CSS change:** `web/src/styles/global.css` (no value change — source of truth shifts)  
**Do NOT edit brandkit/ without the sign-off checklist below being completed.**

---

## 1. Current state — where the value lives and why that is fragile

`--color-link: #087A59` appears in the `[data-theme="light"]` block of
`web/src/styles/global.css` (line 121) as a hard-coded hex literal. The only
upstream justification for this value is a one-line parenthetical in
`brandkit/documentation/design-rationale.md` §2 (the binding WCAG table):

> "Light theme signal #0BA678 on #FFFFFF | ~3.2:1 | Large text/icons + non-text UI
> only; body links use #087A59 if needed"

This creates three fragility points:

1. **No machine-readable authority.** `tokens.json` has no `linkBody` key.
   Any tooling that auto-generates CSS variables from `tokens.json` (present or
   future) would silently omit `--color-link` in light mode, reverting to the
   `:root` fallback (`#4FEDB9`, a dark-mode value that fails light-mode contrast).

2. **Value is derivable but undiscoverable.** The value `#087A59` is not
   present in `tokens.json` under `color.light.*` at all — not as `signal`,
   `signalHover`, or any alias. A designer auditing the token file cannot know
   this value exists or is in use.

3. **Prose footnote is not a design token.** `design-rationale.md` is a
   rationale document, not a token registry. Future brandkit updates to §2
   (e.g., updating a different row's contrast notes) could silently invalidate
   the footnote without triggering a CSS review.

The dark theme counterpart is already correctly wired: `--color-link` in `:root`
(dark-default) is `var(--color-accent-hover)`, which resolves to
`color.dark.signalHover: #4FEDB9` — a key that exists in `tokens.json`.

---

## 2. Proposed token — exact JSON key, value, and placement

Add one key to the `color.light` object in `brandkit/design-system/tokens.json`,
after `neutral` (the last existing semantic key in that block):

```jsonc
// Before (truncated to relevant section):
"color": {
  "light": {
    "bg":           "#F7F9FA",
    "surface":      "#FFFFFF",
    "raised":       "#F0F4F7",
    "border":       "#D7DEE5",
    "borderStrong": "#C7D1DA",
    "textPrimary":  "#10181F",
    "textSecondary":"#4A5B6B",
    "textMuted":    "#6B7B88",
    "signal":       "#0BA678",
    "signalHover":  "#099168",
    "onSignal":     "#FFFFFF",
    "healthy":      "#0BA678",
    "warning":      "#B45309",
    "critical":     "#DC2626",
    "neutral":      "#64748B"
    // ← insert here
  },
  ...
}

// After:
"color": {
  "light": {
    "bg":           "#F7F9FA",
    "surface":      "#FFFFFF",
    "raised":       "#F0F4F7",
    "border":       "#D7DEE5",
    "borderStrong": "#C7D1DA",
    "textPrimary":  "#10181F",
    "textSecondary":"#4A5B6B",
    "textMuted":    "#6B7B88",
    "signal":       "#0BA678",
    "signalHover":  "#099168",
    "onSignal":     "#FFFFFF",
    "healthy":      "#0BA678",
    "warning":      "#B45309",
    "critical":     "#DC2626",
    "neutral":      "#64748B",
    "linkBody":     "#087A59"   // ← NEW: body-text link color, light theme
  },
  ...
}
```

**Rationale for the name `linkBody`:** it mirrors the prose term "body links"
from design-rationale §2, distinguishes clearly from `signal`/`signalHover`
(which are interaction/status colors, not link colors), and the `light.` prefix
scopes it unambiguously to the light theme. A `dark.linkBody` is not needed
(see §4) but can be added for symmetry at the operator's discretion.

---

## 3. Contrast ratios of `#087A59` on light background and surface tokens

### WCAG 2.1 relative-luminance formula

For each sRGB channel `c` in [0, 1] decoded from hex:

```
if c <= 0.04045:  c_lin = c / 12.92
else:             c_lin = ((c + 0.055) / 1.055)^2.4

L = 0.2126 * R_lin  +  0.7152 * G_lin  +  0.0722 * B_lin

contrast(L1, L2) = (L_lighter + 0.05) / (L_darker + 0.05)   [L1 >= L2]
```

### Step A — luminance of `#087A59` (the proposed link color)

| Channel | Hex | sRGB (÷255) | Condition | Linear |
|---------|-----|-------------|-----------|--------|
| R | 0x08 = 8 | 0.0314 | ≤ 0.04045 → ÷12.92 | 0.00243 |
| G | 0x7A = 122 | 0.4784 | > 0.04045 → ((0.4784+0.055)/1.055)^2.4 = (0.5056)^2.4 | 0.1947 |
| B | 0x59 = 89 | 0.3490 | > 0.04045 → ((0.3490+0.055)/1.055)^2.4 = (0.3831)^2.4 | 0.1000 |

```
L(#087A59) = 0.2126×0.00243 + 0.7152×0.1947 + 0.0722×0.1000
           = 0.000516 + 0.139233 + 0.007220
           = 0.1470
```

### Step B — luminance of each light-theme background token

**`color.light.surface` = `#FFFFFF`**

L(#FFFFFF) = 1.0000 (by definition)

**`color.light.bg` = `#F7F9FA`**

| Channel | Hex | sRGB | Linear |
|---------|-----|------|--------|
| R | 247 | 0.9686 | ((0.9686+0.055)/1.055)^2.4 = (0.9702)^2.4 = 0.9301 |
| G | 249 | 0.9765 | ((0.9765+0.055)/1.055)^2.4 = (0.9777)^2.4 = 0.9473 |
| B | 250 | 0.9804 | ((0.9804+0.055)/1.055)^2.4 = (0.9814)^2.4 = 0.9560 |

```
L(#F7F9FA) = 0.2126×0.9301 + 0.7152×0.9473 + 0.0722×0.9560
           = 0.1977 + 0.6776 + 0.0690
           = 0.9443
```

**`color.light.raised` = `#F0F4F7`**

| Channel | Hex | sRGB | Linear |
|---------|-----|------|--------|
| R | 240 | 0.9412 | ((0.9412+0.055)/1.055)^2.4 = (0.9443)^2.4 = 0.8714 |
| G | 244 | 0.9569 | ((0.9569+0.055)/1.055)^2.4 = (0.9591)^2.4 = 0.9048 |
| B | 247 | 0.9686 | 0.9301 (same as bg R above) |

```
L(#F0F4F7) = 0.2126×0.8714 + 0.7152×0.9048 + 0.0722×0.9301
           = 0.1853 + 0.6472 + 0.0672
           = 0.8997
```

### Step C — contrast ratios

| Background token | Token value | L(bg) | Contrast with `#087A59` (L=0.1470) | WCAG level |
|-----------------|-------------|-------|--------------------------------------|------------|
| `color.light.surface` | `#FFFFFF` | 1.0000 | (1.0000+0.05)/(0.1470+0.05) = **5.33:1** | **AA** (≥4.5:1 normal text) |
| `color.light.bg` | `#F7F9FA` | 0.9443 | (0.9443+0.05)/(0.1470+0.05) = **5.05:1** | **AA** (≥4.5:1 normal text) |
| `color.light.raised` | `#F0F4F7` | 0.8997 | (0.8997+0.05)/(0.1470+0.05) = **4.82:1** | **AA** (≥4.5:1 normal text) |

All three meet WCAG 2.1 SC 1.4.3 Level AA for normal-weight body text. None
reach AAA (≥7:1) — which is expected and consistent with how `color.light.signal`
(#0BA678, ~3.2:1 on white) was intentionally made lighter while deferring body
links to this darker value.

**Why not a darker value for AAA?** AAA body-link color on a near-white
background requires `L_link ≤ 0.0630` (contrast ≥ 7:1 against L=0.9443). At
that luminance the hue would be indistinguishable from `color.light.textPrimary`
(#10181F, L=0.0062). The designer's original decision — AA link, AAA body text
— is the correct tradeoff for a monitoring dashboard where links are always
underlined on hover and carry shape-based affordance.

---

## 4. Dark-theme counterpart consideration

The dark theme does **not** need a new `color.dark.linkBody` token at this time.

Current wiring in `global.css` `:root` (dark default):
```css
--color-link: var(--color-accent-hover);   /* resolves to #4FEDB9 */
```

`#4FEDB9` is `color.dark.signalHover` in `tokens.json` — an existing,
machine-readable token. Its contrast on the dark background:

- L(#4FEDB9) ≈ 0.6573 (computed from hex 0x4F, 0xED, 0xB9)
- L(#0A0E14) ≈ 0.0043
- Contrast = (0.6573+0.05)/(0.0043+0.05) = 0.7073/0.0543 ≈ **13.03:1 (AAA)**

The dark link is already fully anchored to a token; no orphaned literal exists.

If the operator wants semantic parity (explicit `linkBody` in both themes), a
`color.dark.linkBody: "#4FEDB9"` alias entry can be added to `tokens.json`
simultaneously, but this is cosmetic — the CSS behavior is already correct and
token-backed for the dark theme.

---

## 5. Adoption steps in `web/` once approved

These steps apply **after** the sign-off checklist below is completed and the
token lands in `brandkit/design-system/tokens.json`:

1. **Update `web/src/styles/global.css`** — change the comment and the hard-coded
   literal in `[data-theme="light"]` so the source of truth is clearly the new
   token:

   ```css
   /* Body link: tokens.json color.light.linkBody — AA on all light surfaces (§2.3:1 table) */
   --color-link: #087A59;  /* color.light.linkBody */
   ```

   The hex value stays identical. The change is documentation and traceability
   (the comment now names the token, not the rationale doc footnote). If a
   token-to-CSS code-gen pipeline is introduced in the future, this line becomes
   the generated output and the comment is removed.

2. **No runtime behavior change.** The resolved value `#087A59` does not change,
   so no visual regression tests need to be updated.

3. **Update `design-rationale.md` §2 footnote.** Replace the parenthetical
   "body links use #087A59 if needed" with a reference to the token:
   "body links use `color.light.linkBody` (#087A59, see tokens.json)." This is
   a docs-only change; it must be done in the same commit as the `tokens.json`
   change to keep the two files consistent.

4. **If a dark `linkBody` alias is added:** add a corresponding `:root` comment
   in `global.css` pointing `--color-link` → `color.dark.linkBody`. The value
   (`#4FEDB9`) does not change.

5. **WCAG re-audit is not required** — the value and surfaces are unchanged;
   the contrast numbers in §3 above serve as the audit record for the token.

---

## 6. Sign-off checklist for operator / designer

The following items must be explicitly confirmed before a developer edits
`brandkit/design-system/tokens.json` or opens a PR that touches the brandkit:

- [ ] **Value confirmed.** `#087A59` is the intended canonical body-link color
      for the Pulse light theme. (The designer rationale doc named it; this
      checklist promotes it to authoritative.)

- [ ] **Contrast record accepted.** The AA ratings (5.33:1, 5.05:1, 4.82:1)
      on surface/bg/raised respectively are acceptable for WCAG 2.1 Level AA
      conformance for this product. AAA is acknowledged as not achievable for
      this hue at this background lightness without losing brand identity.

- [ ] **Token name approved.** The key `color.light.linkBody` is the correct
      semantic name for this value in the token namespace. (Alternatives
      considered: `linkText`, `linkDefault`, `bodyLink` — operator selects or
      defers to designer.)

- [ ] **Dark-theme decision.** Either:
      - (a) No dark `linkBody` needed — dark is correctly wired via `signalHover`, or
      - (b) Add `color.dark.linkBody: "#4FEDB9"` for semantic parity (cosmetic,
            no behavior change).

- [ ] **Scope of change reviewed.** Operator confirms this proposal touches
      only `brandkit/design-system/tokens.json` and (for documentation sync)
      `brandkit/documentation/design-rationale.md`. No brandkit SVG/PNG assets
      are changed. No color value in the production bundle changes.

- [ ] **Single-writer rule.** The commit that implements this proposal is owned
      by a single agent/developer scoped to `brandkit/` and `web/src/styles/`.
      No concurrent PR may touch `tokens.json` while this is open.

---

*Document generated by S16 verifier subagent, D-079 session, 2026-07-11.*  
*Do not merge without completing the checklist above.*

---

## 7. Companion proposal: `color.light.info` (S17 addendum, same sign-off)

**Problem (S16 verifier finding, confirmed S17):** the web UI's `--color-info`
(#58A6FF, sourced from `dataviz[1]`) is used for UI **text** in six places
(ProbesPage SyntheticBadge/Results-button/notice text; AnomaliesPage delta/sigma
colors). S17 converted all six literals to `var(--color-info)` — but tokens.json
has **no `info` key in either theme tree**, so there is nothing authoritative to
override it with in light mode, where #58A6FF measures **2.39:1 on `light.bg`
#F7F9FA — a WCAG AA failure** for text. The dark theme is unaffected (#58A6FF on
#0A0E14 ≈ 6.9:1). Per D-071 (brandkit authoritative, never invent values) the
light value is escalated here rather than hard-coded in `web/`.

**Proposed:** add to `color.light`: `"info": "<designer-picked accessible blue>"`.
Candidate values for the designer to choose from (all ≥4.5:1 on #F7F9FA and
#FFFFFF, hue-adjacent to dataviz[1] so charts and info-text stay visually kin):

| Candidate | on #F7F9FA | on #FFFFFF | Note |
|-----------|-----------|-----------|------|
| `#1A6FC4` | 4.83:1 AA | 5.03:1 AA | closest hue to #58A6FF |
| `#2563EB` | 4.89:1 AA | 5.17:1 AA | tailwind-blue-600 familiar |
| `#0052CC` | 6.46:1 AA | 6.73:1 AA | most headroom |

Optionally also formalize `color.dark.info: "#58A6FF"` (today it exists only as
a `global.css` literal derived from `dataviz[1]`).

**Interim state until sign-off:** light mode renders `--color-info` at #58A6FF
(unchanged from S16 ship); the six call sites are now var-based, so adoption is
a one-line `global.css` `[data-theme="light"]` override once the token lands.
Dataviz usages of #58A6FF (charts, `--chart-2`, FleetPage mem-gauge blue) are
theme-invariant by design and are NOT covered by this token.
