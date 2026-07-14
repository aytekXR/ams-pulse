# §2.19 UI/UX Refactor — Wave Plan

**What this is:** Page-by-page refactor plan produced by the S30 scoping work order (D-092).  
**Standing ruling (D-071 / §2.19):** uipro = refactor *method and quality*; brandkit = binding *values*.  
A skill recommendation that conflicts with `brandkit/design-system/tokens.json` or
`brandkit/documentation/design-rationale.md §2` is overridden by the token; conflicts are filed
for the operator, not resolved by the session author.  
**Source spec:** `agents/handoffs/ROADMAP-V2.md §2.19`.

---

## 1. Vendored-skill status

### 1.1 Vendor-review verdict

Full installed bundle verdict: **DO_NOT_COMMIT** (scout review, S30).

Five categories of blocker across the bundle:

| Category | Count | Representative evidence |
|---|---|---|
| Binding rule violations | 4 | 74+ Google Fonts CDN refs in typography.csv; wrong font families in slide template; shadcn/Tailwind stack pressure against a Vite+React project; design_system.py bypasses tokens.json as palette source |
| Live network calls in production scripts | 3 scripts | design/scripts/{cip,logo,icon}/generate.py → Google Gemini API; ui-styling/scripts/shadcn_add.py → npx → npm registry |
| Instruction injection in SKILL.md files | 3 files | design/SKILL.md instructs `pip install google-genai pillow`; design-system/SKILL.md embeds cdn.jsdelivr.net script tag; ui-styling/SKILL.md instructs `npx shadcn@latest init/add` |
| Non-test subprocess calls | 1 | shadcn_add.py `subprocess.run(['npx', 'shadcn@...', 'add'])` installs packages into the project at skill invocation time |
| Incomplete / absent licence declarations | 6 of 7 skills | Apache-2.0 with unfilled copyright template (ui-styling); MIT frontmatter only, no LICENSE file (design, banner-design, design-system); no declaration at all on the main 143-file skill ui-ux-pro-max and on brand, slides |

The `--offline` flag used at `uipro init` time does NOT make the generator scripts offline; it only controls CLI initialisation behaviour. The network calls are in the generated Python scripts themselves.

### 1.1b Disposition (S30 ruling): local-only, gitignored

`.claude/skills/` is **NOT committed** and is gitignored (S30). Decisive blocker even for
a pruned subset: the core `ui-ux-pro-max` skill carries **no license grant** — committing
it to this public repo would be redistribution without a granted right (independently
re-derived by the S30 commit-gate verifier). The skill still works for every session
because all sessions run on this VPS where it is installed on disk.

**Bootstrap (if `.claude/skills/ui-ux-pro-max/` is missing):**

```sh
uipro --version          # expect 2.11.0 (global CLI, ~/.nvm .../bin/uipro)
cd /home/aytek/repo/ams-pulse && uipro init --ai claude --offline
ls .claude/skills/ui-ux-pro-max/scripts/search.py   # must exist afterwards
```

Re-visit committing ONLY if upstream (ui-ux-pro-max-cli) publishes an explicit license
for the skill content — then a pruned `ui-ux-pro-max/`-only commit could be reconsidered
(the design/ui-styling/design-system blockers in §1.1 stand regardless).

### 1.2 Skills IN scope for waves

Only **ui-ux-pro-max/** is in scope. The parts that are safe to use:

- `scripts/search.py` — BM25 over local CSV data, Python stdlib only, no network calls.
  Invoke via targeted `--domain` and `--stack` searches as described in §2 below.
- `data/stacks/react.csv` — React-specific patterns; discard any code that imports from
  `@/components/ui/` (shadcn) or uses Tailwind `className=`. These patterns are inapplicable
  to this codebase (confirmed: zero Tailwind/shadcn/Radix deps in `web/package.json`).
- `data/charts.csv` rows for trend, real-time streaming, and anomaly detection — chart TYPE
  selection and accessibility rules are adoptable. Discard all `#RRGGBB` values; substitute
  `CHART_COLORS[]` index constants from `web/src/lib/chartColors.ts`.
- `SKILL.md` §1–§3 checklists (Accessibility, Touch, Performance) — methodology is correct and
  adoptable verbatim. These are the primary value the skill delivers.
- `SKILL.md` animation rules — timing ranges (150–300ms micro, ≤400ms complex) are compatible
  with token values `--motion-fast` (120ms) and `--motion-base` (200ms).

### 1.3 Skills OUT of scope

| Skill | Reason |
|---|---|
| `design/` | Gemini API callers in all three generate.py scripts; `pip install` instruction in SKILL.md |
| `ui-styling/` | Wrong stack (shadcn/ui + Tailwind). No Tailwind or Radix deps in web/package.json. Installs incompatible packages via npx at runtime |
| `design-system/scripts/generate-slide.py` | Google Fonts CDN hardcoded in HTML template (lines 49–50); Space Grotesk / Inter / JetBrains Mono font families conflict with IBM Plex binding |
| `design-system/SKILL.md` | Instructs embedding cdn.jsdelivr.net/chart.js in generated output |
| `slides/`, `banner-design/` | Inapplicable (marketing/presentation); no Pulse dashboard scope |
| `ui-ux-pro-max/scripts/design_system.py` Steps 1–2b | Generates parallel MASTER.md + palette from CSV; redundant and conflicting with brandkit as authoritative source. Skip these steps entirely |
| `ui-ux-pro-max/data/typography.csv` CSS Import column | All 74 rows carry `@import url(fonts.googleapis.com/...)`. These strings must never appear in any Pulse source file (self-hosted IBM Plex only via @fontsource) |

---

## 2. Method

### 2.1 How each wave uses the skill

At the start of each wave, load `SKILL.md` once (full read). Do NOT run Steps 1–2b of the
skill workflow (design system generation, palette/font/spacing selection — those come from
`brandkit/design-system/tokens.json` and `web/src/styles/global.css`).

Run targeted searches via `scripts/search.py` as needed during the wave:

```sh
# UX heuristics before implementing a component
python3 .claude/skills/ui-ux-pro-max/scripts/search.py \
  "dashboard accessibility keyboard aria focus" --domain ux

# Chart components (real-time streaming most relevant to Pulse)
python3 .claude/skills/ui-ux-pro-max/scripts/search.py \
  "real-time streaming monitoring anomaly line" --domain chart

# React performance patterns
python3 .claude/skills/ui-ux-pro-max/scripts/search.py \
  "virtualize lists memo suspense lazy bundle" --stack react

# Form components
python3 .claude/skills/ui-ux-pro-max/scripts/search.py \
  "form validation error label progressive" --domain ux

# Animation review
python3 .claude/skills/ui-ux-pro-max/scripts/search.py \
  "animation reduced-motion loading skeleton" --domain ux
```

When search results return hex colour values, font names, or pixel sizes: **discard them**.
Substitute the equivalent from `web/src/styles/global.css`:  
`--color-accent`, `--color-error`, `--chart-1` through `--chart-8`, `--motion-fast`,
`--motion-base`, `--space-1` through `--space-9`, etc.

For Recharts `stroke=` / `fill=` props (SVG presentation attributes that the browser resolves
before CSS vars are applied): use `CHART_COLORS[]` index constants from
`web/src/lib/chartColors.ts`, not `var(--chart-N)`.

### 2.2 Per-wave pre-PR checklist (binding for every wave)

Derived from SKILL.md §§1–3. Mandatory before any wave lands.

**Accessibility — priority 1:**
- Contrast ≥4.5:1 normal text / ≥3:1 large text, verified in BOTH light and dark themes
  against `brandkit/documentation/design-rationale.md §2` WCAG table
- Visible focus rings 2–4px on every interactive element
- `aria-label` on all icon-only controls
- `label[for]` (or `htmlFor`) association on every form input
- Sequential h1→h6 heading hierarchy (no level skipping)
- Color-not-only: every colour-encoded state also has shape/icon/text
- `prefers-reduced-motion` collapse via `--motion-fast: 0ms` / `--motion-base: 0ms`
  (already in `global.css` lines 151–154 — do not remove)
- Escape routes in all modals and multi-step flows

**Touch / Interaction — priority 2:**
- Minimum 44×44pt touch targets (`tokens.json layout.minTouchTarget = 44`)
- ≥8px gap between adjacent interactive targets
- Async operations: disable button + show spinner; re-enable on resolve/reject
- `cursor: pointer` on all clickable non-button elements

**Performance — priority 3:**
- Lists ≥50 items use `@tanstack/react-virtual` (already a `web/package.json` dep — use it)
- Skeleton screens / shimmer for operations expected to take >1s
- Stable list keys (not array index)
- No `useEffect` for derived state (compute during render)
- Dynamic `import()` for page-level components not needed on initial load

**Brandkit compliance — gate (any failure blocks merge):**
- `grep -rE '#[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{3}' web/src/features/<wave>` on changed files:
  only permitted occurrences are inside `CHART_COLORS`, `PROTOCOL_COLORS`, or `STATUS_COLORS`
  constant definitions in `chartColors.ts`
- `grep -rE 'style=\{.*\b[0-9]+px\b' web/src/features/<wave>` on changed files: zero new
  raw pixel literals in JSX style props (use `--space-*` vars or the density tokens)
- All Recharts `stroke=` / `fill=` props on changed components reference `CHART_COLORS[N]`
  constants, not bare string hex
- No CDN references (fonts.googleapis.com, cdn.jsdelivr.net, unpkg.com, etc.)
- No `@import url(fonts.googleapis.com/...)` strings in any source file

**CI gates (each wave — all must be green):**
- `cd web && npm run gen:api && git diff --exit-code` — generated client in sync with
  `contracts/openapi/pulse-api.yaml` (gen:api drift gate, SESSION-30 §Gates verbatim;
  the `diff contracts/` check below does NOT cover generated-client drift)
- `cd web && npm run lint`
- `cd web && npm run build` (zero TS errors)
- `cd web && npx vitest run --coverage` — floors: lines ≥59, branches ≥54, functions ≥45
- Playwright-docker: `dashboard-render.spec`, `auth-gate.spec`, `csp.spec` — light AND dark
- Playwright-docker: `prefs.spec` — density modes (compact/wall/default); reduced-motion
- WCAG table `design-rationale §2` re-verified for all changed components in both themes
- `diff contracts/` empty — zero public API contract changes
- `diff brandkit/design-system/tokens.json` empty — tokens.json untouched by UI waves

---

## 3. Conflict ledger

Conflicts between ui-ux-pro-max skill guidance and brandkit binding values.
Resolution in every case: **token wins**. Adoptable methodology elements are noted separately.

| # | uipro guidance | brandkit binding | Resolution | Citation |
|---|---|---|---|---|
| C1 | colors.csv: industry-specific hex palettes; `--domain color` search for `analytics dashboard` → bg `#020617`, primary `#1E40AF`, accent `#D97706` | tokens.json `color.dark.*` / `color.light.*` define the complete palette. design-rationale §1: "One signal color — Signal Green `#2CE5A7` means exactly one thing: live/healthy/primary action." | **Token wins.** Skip all `--domain color` searches. Discard all hex values from colors.csv. The semantic naming structure (primary, surface, error, on-error) is adoptable as a methodology for naming CSS vars already in global.css | colors.csv rows 6–7 vs tokens.json color.dark.* |
| C2 | typography.csv (74 rows): Google Fonts CDN `@import` strings. Closest matches for Pulse (rows 9 and 31) still reference `@import url(fonts.googleapis.com/css2?family=IBM+Plex+Sans...)` | tokens.json `font.sans = 'IBM Plex Sans'`, `font.mono = 'IBM Plex Mono'`. design-rationale §1: "Both OFL-licensed and self-hostable — a hard requirement for a no-phone-home product (no font CDNs in production)." web/package.json: `@fontsource/ibm-plex-sans`, `@fontsource/ibm-plex-mono`. global.css lines 7–11: `@import '@fontsource/...'` (bundled by Vite, never fetched at runtime) | **Token wins.** IBM Plex Sans and IBM Plex Mono only, via @fontsource. The scale and weight hierarchy methodology (heading 600–700, body 400, label 500) from uipro is adoptable and already reflected in tokens.json | typography.csv rows 9,31 vs tokens.json font.* and design-rationale §1 |
| C3 | SKILL.md §7 spring physics: "prefer spring/physics-based curves over linear or cubic-bezier". motion.csv No=8 (Stagger List Standard): `back.out(1.4)`; No=3 (Hover Micro-interaction Complex): `elastic.out(1,0.4)` (GSAP, which is not in web/package.json) | tokens.json `motion.note = "Live data updates fade, never slide. No bounce."` design-rationale §5: "Never bounce, never slide charts." global.css lines 62–63: `--motion-fast: 120ms ease-out`, `--motion-base: 200ms ease-out` | **Token wins.** Use `--motion-fast` and `--motion-base` exclusively. Spring/bounce/elastic/back.out easings forbidden. The 150–300ms timing principles from uipro are compatible with the token values. GSAP is not in web/package.json — no GSAP code | motion.csv rows 3,8 vs tokens.json motion.note and design-rationale §5 |
| C4 | charts.csv: streaming `current pulse: #00FF00 (dark)` / `#0080FF (light)`; anomaly marker `#FF0000`; trend line `#0080FF` | tokens.json `color.dataviz` = 8-color array. Semantic: healthy = `#2CE5A7`, warning = `#FFB224`, critical = `#FF5C68`. global.css: `--chart-1` through `--chart-8` | **Token wins.** Use brandkit dataviz array for multi-series; semantic tokens for status. Streaming "current pulse" → `CHART_COLORS[0]` (`#2CE5A7`, which is also the signal/healthy colour); anomaly markers → `--color-error`; trend → `CHART_COLORS[1]`. Chart TYPE selection and accessibility rules from charts.csv are fully adoptable | charts.csv rows 1,10,23 vs tokens.json color.dataviz |
| C5 | styles.csv: style-specific radius values 8–24px depending on aesthetic row | tokens.json `radius.control = 8`, `radius.card = 12`, `radius.pill = 999`. design-rationale §3: "Radii: 12 card / 8 control / full pill. Nothing else." | **Token wins.** 8 for controls, 12 for cards, 999 for pills — and nothing else. Style-specific ranges from styles.csv irrelevant. uipro principle of CONSISTENT radii reinforces the token rule | styles.csv rows 2,9,19 vs tokens.json radius.* |
| C6 | styles.csv: Glassmorphism `backdrop-filter: blur(15px)`; Neumorphism `box-shadow: dual inset`; various per-card shadow recommendations | tokens.json `elevation.note = "Elevation by tone + border, not shadow. Shadows only on overlays."` Sole defined shadow: `elevation.overlay = "0 24px 64px rgba(0,0,0,0.5)"` (modals/drawers only) | **Token wins.** No backdrop-filter/blur on cards. No per-card box-shadow. Elevation is tone-step (`--color-surface` → `--color-raised`) plus border (`--color-border`). The single overlay shadow token is valid for modals | styles.csv rows 2,3,28 vs tokens.json elevation.note |

| C7 | Wave 0 WCAG findings (3 items — verify:a11y-brandkit, S31 D-093) | (a) Light-theme CTA: `--color-on-signal` (#FFFFFF) on `--color-accent` (#0BA678) = 3.12:1 at 13px — below 4.5:1 AA for normal text. Pre-existing at baseline 2f53414. Fix requires `tokens.json color.light.accent` → `#087A59` (5.33:1 on white). (b) Description paragraph default color `--color-muted` failed AA in both themes (dark 3.50:1, light 4.36:1) for 14px body copy. (c) Inactive tab color `--color-muted` failed AA in both themes for 13px normal text. | **(b) and (c) fixed in Wave 0 (S31)**: default `descriptionColor` in `TierGate.tsx` changed to `var(--color-secondary)` (8.18:1 dark / 7.00:1 light — PASS); inactive tab color in `Tabs.tsx` changed to `var(--color-secondary)`. **(a) WAIVER — light-theme CTA contrast**: the `background: var(--color-accent)` + `color: var(--color-on-signal)` CTA is pre-existing from baseline 2f53414 (ReportsPage, AnomaliesPage, ProbesPage all had it verbatim). brandkit/ is frozen for UI waves; the fix requires `tokens.json color.light.accent` → `#087A59`. **NO WAIVER EXISTS — OPERATOR RULING REQUIRED (gap G3, filed in `docs/operator-expected.md` at S31 close).** Wave 0 neither introduced nor fixed this failure; it ships unchanged from baseline. `brandkit/` is the operator's to change (D-071), so a session may not self-approve the token edit. Until the operator rules, the light-theme CTA remains a known AA failure and the first token-update wave carries the fix. That wave must update `color.light.accent` in `tokens.json` and cascade the change through `global.css [data-theme=light] --color-accent`. Until then, the CTA is AA-passing in dark theme (#FFFFFF on #2CE5A7 = 8.53:1) and fails only in light theme. | verify:a11y-brandkit S31/D-093 |

### 3.1 Genuine brandkit GAPS — filed for operator (2 open items)

These are cases where the brandkit specification is incomplete or future-spec and the skill's guidance fills a real gap. The operator or designer must rule before the relevant wave proceeds.

**G1 — Mobile input font-size (affects any wave with form inputs):**  
uipro SKILL.md §2: "Minimum 16px body text on mobile (avoids iOS auto-zoom)."  
brandkit: `tokens.json type.body.size = 14`. design-rationale §3: "Tables: 40px rows, 13px table text." Product designed for desktop ops/NOC screens.  
Gap: iOS auto-zooms form inputs with `font-size < 16px`. If AlertRuleForm, ProbeForm, SettingsPage
form inputs, or OnboardingWizard inputs are reachable on mobile, `font-size: 16px` is needed on
`input`/`select`/`textarea` elements specifically while body text stays at 14px.  
**Operator question:** Is any mobile viewport supported for form-bearing pages (Alerts, Probes, Settings)? If yes, apply 16px font-size on input elements only. If no, no change required.

**G2 — Icon library (affects any wave adding icons):**  
uipro SKILL.md §Common Rules Icons: "Default: Phosphor (`@phosphor-icons/react`). Fallback: Heroicons."  
brandkit design-rationale §5 (future spec): "Icon set — commission or curate the full Lucide subset at 1.75px stroke; publish as `icons/ui/` sprite." This is forward-looking; no icon library is currently pinned in tokens.json or web/package.json.  
Gap: Lucide sprint is unscheduled. Until it lands, a library must be chosen and kept consistent within and across waves.  
**Operator question:** Adopt Phosphor (uipro recommendation) as the bridge library until the Lucide sprint, or prefer Lucide directly (aligning with the future brandkit direction)? Either way, one library must be locked before any wave that adds icons; mixing is forbidden. Stroke consistency (1.75px per future brandkit spec) applies regardless of which library is chosen.

**G4 — Touch-target minimum (NEW, S33/D-095; affects every remaining wave):**
brandkit `tokens.json layout.minTouchTarget = 44`. uipro SKILL.md §2 asks for 44×44pt.
The pages' existing controls are ~28px tall (padding `4px 10px` / `6px 12px` at 11–12px text).

The 44 figure is **WCAG 2.1 SC 2.5.5 — Level AAA**. The Level **AA** requirement is
**WCAG 2.2 SC 2.5.8 Target Size (Minimum) = 24×24 CSS px**, which the current controls
already satisfy. So enforcing 44 is *exceeding* AA, not *reaching* it — and it is not free:
it visibly retextures every button on every page, and it contradicts brandkit's own
desktop-density spec in design-rationale §3 ("Tables: 40px rows, 13px table text" — a
product designed for NOC/ops screens). It is also entangled with **G1**: if no mobile
viewport is supported, the touch-target argument largely evaporates.

Wave 2 **deferred** it rather than ship a silent visual change under a refactor chartered as
pixel-neutral.
**Operator question:** Enforce `minTouchTarget = 44` across the UI (accepting that every
button gets taller and the desktop density loosens), or keep the current compact desktop
density and record 24×24 (WCAG 2.2 AA) as the binding floor? This blocks any wave that would
enforce 44pt.

**G5 — The brandkit WCAG table itself has a wrong number (NEW, S33/D-095):**
`brandkit/documentation/design-rationale.md` §2 is **BINDING** (CLAUDE.md). Its row
*"Muted #5C6F80 on #0A0E14 | ~4.6:1 | AA — labels/captions only, never body copy"* is
**incorrect**. Recomputed from the WCAG 2.x sRGB relative-luminance formula: the true ratio
is **3.72:1**.

That matters because 3.72 is **below the 4.5:1 AA bar for normal text** — it only clears the
3:1 large-text bar. The table's own guidance ("labels/captions only") was written against a
number that is too high, and labels/captions at 11–12px *are* normal text. On the actual
surfaces the app uses, `--color-muted` measures **3.44:1 dark / 4.36:1 light** — failing AA
everywhere it was used for text.

This is why Wave 0 and Wave 2 replaced `--color-muted` with `--color-secondary` (8.03:1 dark
/ 7.00:1 light) wherever it carried text. Those fixes were correct; the table is what is wrong.
**Operator action:** correct the ratio in the WCAG table (and, if desired, restate what
`--color-muted` may legitimately be used for — on current values, only large text or
non-text UI). brandkit is yours (D-071); no session will edit it.

**G6 — `Badge` info variant fails AA in light theme (NEW, S33/D-095):**
`global.css` deliberately does **not** override `--color-info` (`#58A6FF`) in the light theme
(there is an explicit comment saying so). In light theme the info Badge therefore renders
`#58A6FF` text on a composited `#EEF6FF` background = **2.32:1**, far below AA.
Reached today from AlertsPage; any page may use it.
**Operator action:** add a `color.light.info` token (something around `#1B5EAD` reaches AA on
that background). This is a `tokens.json` change — **operator-gated**; Wave 2 did not invent
a value.

---

## 4. Wave plan

**Shared surface goes FIRST (Wave 0).** Justification: `TierGate` and `Tabs` components
extracted in Wave 0 are consumed verbatim by five subsequent page waves. Doing extraction
last would require each wave to work against the triplicated/duplicated inline pattern, then
be retroactively refactored. Doing it first also makes the `--space-*` adoption pattern
concrete in a single low-risk extraction before any page wave authors px→token sweeps.

---

### Wave 0 — Shared Surface [S] ✅ DONE — S31 (D-093, 2026-07-14)

**Pages / surface:** No feature page changes. Shared component layer only.

**Files:**
- `web/src/components/TierGate.tsx` — new (extracted from Reports/Anomalies/Probes)
- `web/src/components/Tabs.tsx` — new (extracted from inline pattern in Analytics/Alerts/Reports/Settings)
- `web/src/lib/chartColors.ts` — VERIFIED: `CHART_COLORS[7]` is `'#7C93AD'` at line 19 — already complete; no change needed
- `web/src/features/reports/ReportsPage.tsx`, `web/src/features/anomalies/AnomaliesPage.tsx`, `web/src/features/probes/ProbesPage.tsx` — replace inline TierUpsell with `<TierGate>` import ✅
- `web/src/components/__tests__/TierGate.test.tsx`, `web/src/components/__tests__/Tabs.test.tsx` — new unit tests (44/44 pass)
- `web/src/styles/global.css` — 4 new CSS custom properties (`--radius-control`, `--radius-card`, `--radius-pill`, `--min-touch`) + shared focus-ring block

**What changed:**
- TierGate: pure extraction of the triplicated TierUpsell pattern. Props interface from the
  existing pattern; no logic change, no new API call.
- Tabs: pure extraction of the repeated inline tab-button pattern. Props: `tabs: {id, label}[]`,
  `activeTab`, `onTabChange`. Keyboard nav (ArrowLeft/Right/Home/End), roving tabIndex,
  `role="tablist"` / `role="tab"` / `aria-selected`. No logic change.
- chartColors.ts: `CHART_COLORS[7]` confirmed present (`'#7C93AD'`). No edit made.

**Plan corrections (S31 D-093):**
- ~~"extracted from inline pattern in Analytics/QoE/Alerts/Reports/Fleet/Settings"~~ — **CORRECTED**.
  Live inventory: **4 pages** carry the identical inline tab pattern: Analytics, Alerts, Reports,
  Settings. QoE has NO tab pattern (QoE/Probes/Anomalies = N/A). Fleet uses a segmented-control
  widget with different visual treatment — it is NEVER a `<Tabs>` candidate; it needs a
  separate `<SegmentedControl>` component. Page tab conversions remain deferred to their
  chartered waves (Analytics → Wave 2; Alerts/Settings → Wave 4; Reports → Wave 5).
- ~~"verify `CHART_COLORS[7]` is `'#7C93AD'` (complete the index)"~~ — **CLARIFIED**:
  the index was already complete at baseline. "Complete the index" was misleading. Verified-only.
- **C7 WCAG finding (new, S31):** three items found during Wave 0 a11y pass:
  (a) Light-theme CTA `--color-on-signal` on `--color-accent` = 3.12:1 at 13px — WAIVER
  granted (pre-existing at 2f53414; fix requires `tokens.json color.light.accent` → `#087A59`;
  deferred to first token-update wave); (b) `--color-muted` at 14px body in TierGate description
  — FIXED (changed to `var(--color-secondary)`, 8.18:1 dark / 7.00:1 light); (c) `--color-muted`
  inactive tab text — FIXED (Tabs.tsx uses `var(--color-secondary)`).
- **F5 live finding (S31/D-093):** AMS BroadcastDTO returns `publishType="RTMP"` for
  SRT-ingested streams (live-observed during TC-I-05-SRT-20260714T022945Z). Pulse mirrors AMS
  verbatim; SRT ingest is counted as RTMP in protocol breakdown. publishType for SRT was
  "unknown at S29 authoring" — now KNOWN and recorded. No code change; Pulse behavior is correct
  (mirrors AMS); operator documentation updated in `docs/AMS-INTEGRATION.md`.

**What must NOT change:**
- Zero public API changes. Zero contract changes.
- Tier-gate entitlement logic (only the render is moving; `LicenseContext` remains unchanged).
- No style behaviour changes — extraction is pixel-for-pixel equivalent.
- All 404 existing unit tests must still pass after extraction.

**Result:** 44/44 new component tests pass. 131/131 page-suite tests pass (Reports,
Anomalies, Probes + TenantsTab). TypeScript: zero errors.

**Acceptance gates:** full per-wave checklist (§2.2). Additional extraction regression: run
`npx vitest run` before and after; diffs must not introduce new failures.

---

### Wave 1 — LiveOverview + QoE [M] — ✅ DONE — S32 (D-094, 2026-07-14)

> **PLAN CORRECTED AGAINST REALITY (S32).** The px counts below were wrong.
> LiveDashboard has **33 px occurrences, 19 with an exact `--space-*` match** (not "13 px");
> QoePage's "5 px" was likewise understated. **Binding rule learned and now applied to every
> remaining wave: substitute ONLY where the token EQUALS the literal.** The `--space-*` scale is
> 4/8/12/16/24/32/48/64/96 — a literal with no exact match (6px, 20px, 36px, 160px, 180px, 260px,
> 520px, and every typography size) MUST be left alone. Snapping 13px to `var(--space-3)` (12px) is
> a silent 1px regression; these waves may not change pixels.
>
> **Also landed:** the hex fallbacks in `var(--color-warning, #FFB224)` / `var(--color-error,
> #FF5C68)` were not just redundant but **stale** — the light-theme token values (`#B45309` /
> `#DC2626`) differ from the fallback hex, so they would have rendered the wrong colour if ever
> reached. Dropped.
>
> **Gate lesson (binding for every later wave):** run the specs of the components the wave TOUCHES,
> not only the §2.2 default Playwright set. `streams-virtualization.spec` is not in the default set
> and it caught a real regression (see D-094).

**Pages:** LiveOverview (scout size M), QoE (scout size S). Combined: M.

**Files:**
- `web/src/features/live/LiveDashboard.tsx`, `StatCard.tsx`, `StreamsTable.tsx`, `ProtocolDonut.tsx`, `useLiveDashboard.ts`
- `web/src/features/qoe/QoePage.tsx`
- `web/src/features/live/__tests__/`, `web/src/features/qoe/__tests__/`

**What changes (method passes applied):**
- ProtocolDonut: `#7C93AD` Cell fallback → `CHART_COLORS[7]` (scout: 1 residual hardcoded hex)
- LiveDashboard: px → `--space-*` tokens — **exact matches only** (S32 truth: 33 px occurrences,
  19 exact-match; the rest left alone by design)
- QoePage: Recharts `stroke` props `#58A6FF` → `CHART_COLORS[1]`, `#FFB224` → `CHART_COLORS[4]`
- QoePage: drop hex fallbacks from `var(--color-warning, #FFB224)` pattern — the tokens are
  confirmed stable; fallback hex is redundant and a future drift risk
- QoePage: px → `--space-*` tokens — **exact matches only** (the "5 px" count was wrong; see the
  correction note at the top of this wave)
- uipro §1 accessibility pass: verify aria-labels on ProtocolDonut legend, StreamsTable column
  headers, StatCard metric values (aria-label describing the metric name + unit)
- uipro §2 touch pass: verify StatCard and StreamsTable row touch targets ≥44pt
- uipro §3 performance pass: StreamsTable is already virtualized (scout confirmed); verify
  skeleton screens are in place for `useLiveDashboard` loading state
- Chart type audit: streaming area/line chart for live bitrate confirmed as correct (charts.csv
  row 23 real-time streaming); `CHART_COLORS[0]` as "current pulse" colour per C4 resolution

**What must NOT change:**
- `useLiveDashboard` hook return type (contract for downstream consumers)
- `LiveSocket` WebSocket event shapes
- StreamsTable column definitions and sort behaviour
- Virtualization logic (already correct — must not regress)

**Acceptance gates:** full per-wave checklist (§2.2).

---

### Wave 2 — Analytics + Fleet [M] — ✅ DONE — S33 (D-095, 2026-07-14)

> **PLAN CORRECTED AGAINST REALITY (S33).** Three items below were wrong:
>
> 1. **"Fleet: `var(--color-*, #hex)` fallbacks — leave alone"** — WRONG. Re-derived from
>    `global.css`: `--color-warning` and `--color-success` are defined in **both** themes,
>    so the fallback is **unreachable**, and the light values (`#B45309` / `#0BA678`)
>    **differ** from the fallback hex — it would have painted the wrong colour if ever
>    reached. **Dropped**, exactly as Wave 1 did on QoE.
> 2. **"`width: 32` → `var(--space-6)`"** — WRONG, and it is the px→token trap wearing a
>    different hat. **Width is a dimension, not spacing.** The `--space-*` scale may only
>    replace spacing properties. Caught by the adversarial verifier; the scout had it wrong.
> 3. **"inline stat-card grids replaced with `<StatCard>`"** — INCOMPLETE. A 1:1 swap is
>    **not pixel-neutral**: the default `<StatCard>` is density-token-driven
>    (`--card-padding` 24px, `--metric-size` 40px) while the Analytics cards are fixed
>    14px/24px. Shipped as **`<StatCard size="compact">`**, carrying the Analytics geometry
>    verbatim. Whether Analytics *should* adopt the density-responsive look is a **design
>    decision filed for the operator**, not one a refactor may make.
>
> **★ TOUCH TARGETS ARE NOT A FREE WIN (binding for every remaining wave).** The drafted
> spec wanted `minHeight: 44` on every button. brandkit's `layout.minTouchTarget = 44` is
> **WCAG 2.1 SC 2.5.5 = AAA**; the **AA** requirement is **WCAG 2.2 SC 2.5.8 = 24×24**,
> which the existing ~28px controls already meet. Enforcing 44 **visibly retextures every
> button**, contradicts brandkit's own desktop-density spec ("Tables: 40px rows"), and is
> **coupled to the unanswered G1**. Filed as **G4** — a pixel-neutral wave may not make it.
>
> **★ A className is a CONTRACT with the stylesheet.** S32 shipped
> `className="filter-input"` with no matching rule (see D-095). `styles/__tests__/
> focus-rings.test.ts` now pins both halves for every CSS-only class. **Any wave adding a
> bare className for styling must add it to that map.**

**Pages:** Analytics (scout M), Fleet (scout M). Combined: M+.

**What shipped (S33):**
- Analytics: 3 Recharts strokes → `CHART_COLORS[1]`/`[0]`/`[4]` (same hex); 5 px → `--space-*`;
  `<StatCard size="compact">` adoption; `role=tabpanel` wiring on all three panels (Wave 0
  deferred panel-side ARIA to the page wave — this is that wave); `accessibilityLayer`;
  `scope="col"`; `--color-muted` eliminated.
- Fleet: 2× memory-healthy `#58A6FF` → `CHART_COLORS[1]` (still dataviz blue, **never**
  `statusColors.healthy`); stale var() fallbacks dropped; 8 px → `--space-*`;
  `<SegmentedControl>` extracted; sr-only tier on LoadBar; `--color-muted` eliminated.
- Shared: `Badge` muted variant + `StatCard` labels → `--color-secondary` (both were AA
  failures reached from Wave 2's surface).
- NEW e2e `analytics.spec.ts` + `fleet.spec.ts` — neither page had a spec.

**Result:** web 548/548 / 35 files; coverage 67.93/63.37/57.11 vs floors 59/54/45;
Playwright 16/16; contracts/ + brandkit/ byte-untouched; zero bare hex, zero `--color-muted`
on both pages. 12 tautological palette tests deleted and replaced with render-level pins;
4 mutations RED-proven.

---

### Wave 2 — original plan (superseded by the above)

**Pages:** Analytics (scout M), Fleet (scout M). Combined: M+.

**Files:**
- `web/src/features/analytics/AnalyticsPage.tsx`, `DateRangePicker.tsx`
- `web/src/features/fleet/FleetPage.tsx`
- Corresponding `__tests__/` files

**What changes:**
- Analytics: 3 chart `stroke` props (`#58A6FF` → `CHART_COLORS[1]`, `#2CE5A7` → `CHART_COLORS[0]`,
  `#FFB224` → `CHART_COLORS[4]`); 18 px → `--space-*` tokens; inline stat-card grids replaced
  with `<StatCard>` (the shared component LiveDashboard uses correctly); tab pattern → `<Tabs>`
  (Wave 0 prerequisite)
- Fleet: 2 × `#58A6FF` for normal-memory LoadBar → `CHART_COLORS[1]` (intentional design choice
  confirmed — dataviz[1], not a health signal; CHART_COLORS index makes the intent explicit);
  10 px → `--space-*` tokens; tab pattern → `<Tabs>` if applicable
- uipro §1 pass: `DateRangePicker` input labels, `useStatusColors` hook light/dark contrast
  verification (both themes)
- uipro §3 pass: Analytics data fetch — verify Promise.all for independent async ops

**What must NOT change:**
- DateRangePicker props interface (used by QoE and Reports — changes would cascade)
- Analytics query parameters (public API, no contract change)
- Fleet `cpuStatus`/`memStatus` pure functions (exported for testability)
- `useStatusColors` hook interface

**Acceptance gates:** full per-wave checklist (§2.2). DateRangePicker regression: both Analytics
and QoE `__tests__` must pass.

---

### Wave 3 — Ingest + Anomalies [M] — ✅ DONE — S33 (D-095, 2026-07-14)

> **★★ THE PLAN'S OPEN COLOUR QUESTION WAS A FALSE DICHOTOMY.** It asked whether Ingest's
> `#FF5C68` was an error event (→ `--color-error`) or a plain dataviz series (→
> `CHART_COLORS[3]`). **Neither guess was safe: `#FF5C68` IS NOT IN `CHART_COLORS` AT ALL.**
> It strokes the **Packet Loss** line, and `CHART_COLORS[3]` is `#F06BB2` — **pink**.
> Resolution: it is `--color-error`, routed through `useStatusColors().critical` so it is
> theme-correct. Dark renders identically; **light theme is FIXED** (it had been hard-coding
> the dark red instead of `#DC2626`).
> Bitrate/FPS/Jitter → `CHART_COLORS[1]`/`[0]`/`[4]` (same hex). ReferenceLines → JS values.
> Two tautology suites deleted (they tested helpers defined *inside the test file*).
> `rgba(224,82,82,0.15)` drop-chip tint LEFT ALONE + reported: its base hex isn't even a
> brandkit colour and its alpha matches no token — retinting it silently was not this wave's
> call.

### (superseded) Wave 3 — original plan

**Pages:** Ingest (scout M), Anomalies (scout M). Combined: M.

**Files:**
- `web/src/features/ingest/IngestPage.tsx`
- `web/src/features/anomalies/AnomaliesPage.tsx`
- Corresponding `__tests__/` files

**What changes:**
- Ingest: 4 chart stroke hex → `CHART_COLORS[0]` (`#2CE5A7`), `[1]` (`#58A6FF`), `[3]`
  (`#F06BB2` — note: scout lists `#FF5C68` which is critical/error; verify which series uses
  it and substitute `--color-error` if it encodes a drop/error event, `CHART_COLORS[3]` if it
  is a plain dataviz series); `[4]` (`#FFB224`); drop-event panel inline `rgba()` → `var(--color-error-bg)`;
  8 px → `--space-*` tokens
- Ingest: `<TierGate>` consumption where applicable (Wave 0 prerequisite)
- Anomalies: 19 px → `--space-*` tokens; `<TierGate>` consumption (Wave 0 prerequisite);
  sigma sensitivity selector: uipro §1 pass (aria-label, keyboard navigation)
- uipro §1 pass on Ingest StreamDetail panel (aria roles on status indicators)

**What must NOT change:**
- Ingest `StreamDetail` panel internal state (master/detail pattern)
- Anomaly severity threshold logic (sigma computation)
- Anomaly table sort/filter behaviour

**Acceptance gates:** full per-wave checklist (§2.2). Ingest chart series colour mapping must be
documented in a code comment (which series is which channel) once the `#FF5C68` vs `#F06BB2`
question above is resolved.

---

### Wave 4 — Alerts + Settings [M] — ✅ DONE — S33 (D-095, 2026-07-14)

> **★★ SettingsPage's hand-rolled tab bar was a KEYBOARD TRAP, not just a false promise.**
> It had `role="tab"` + a **roving `tabIndex`** but **no key handler** — so every inactive tab
> was `tabIndex=-1` (out of the tab order) with no Arrow handler to reach it. **Five of six
> Settings tabs were unreachable by keyboard.** Replaced with the shared `<Tabs>`; a `wrap`
> prop was added to `<Tabs>` (the only reason the local copy existed).
> **★★ Both alert forms announced every error TWICE** — each message was mirrored into a
> separate `sr-only aria-live` div *and* rendered inline. Removed: **the inline message IS the
> live region** (`role="alert"`), and it is what `aria-describedby` points at. One error, one
> node. Two tests had *pinned the duplicate* and were replaced.
> OnboardingWizard checkmark → inline `<svg>` (**G2 unanswered → no icon dependency added**).

### (superseded) Wave 4 — original plan

**Pages:** Alerts (scout M), Settings (scout M, 668 lines + OnboardingWizard 363 lines).
Combined: M.

**Files:**
- `web/src/features/alerts/AlertsPage.tsx`, `AlertRuleForm.tsx`, `AlertChannelForm.tsx`
- `web/src/features/settings/SettingsPage.tsx`, `OnboardingWizard.tsx`
- Corresponding `__tests__/` files (4 alert test files + 2 settings test files)

**What changes:**
- Alerts: 13 px → `--space-*` tokens; tab pattern → `<Tabs>` (Wave 0); inline form modals:
  assess extraction to shared `<Modal>` wrapper (document decision; defer if scope exceeds M)
- Alerts: uipro §8 form/feedback pass on `AlertRuleForm` and `AlertChannelForm` — error
  messages placed below fields; `aria-live` on error regions; auto-focus first invalid field
  on submit error; confirmation dialog before destructive rule deletion
- Settings: `#58A6FF` × 2 in tokens-tab inline `color:` style → `var(--color-info)`;
  28 px → `--space-*` tokens; `&#10003;` checkmark in OnboardingWizard → SVG check icon
  (consistent with G2 icon library ruling when available; use inline SVG if library not yet chosen)
- Settings: uipro §9 navigation pass on OnboardingWizard multi-step flow — focus management
  between steps; escape route; state-preserving back navigation
- uipro §8 pass on Settings form fields (label association, required markers)

**What must NOT change:**
- `AlertRuleForm` validation schema (logic only; render changes allowed)
- `AlertChannelForm` channel type enum and API payload shape
- Settings tab route IDs
- OnboardingWizard step sequence and completion state

**Acceptance gates:** full per-wave checklist (§2.2). Alert test suite (53 tests across 4 files)
must stay green.

---

### Wave 5 — Reports + Probes [L] — ✅ DONE — S33 (D-095, 2026-07-14)

> **★★★ A TEST FORCED PRODUCTION CODE TO GET WORSE — the sharpest lesson of the wave.**
> The implementer wrote a file-wide assertion banning **all** `stroke="var(--color-…)"`. To
> satisfy its own test it swapped the TierGate's **plain `<svg>` icon** to a `CHART_COLORS[0]`
> literal — **wrong colour in light theme** (`--color-accent` is `#0BA678` there) — and swapped
> **CartesianGrid** off `--color-border` onto a far lighter neutral, diverging from every other
> chart page. Both reverted.
> **BINDING CLARIFICATION of RULE 3 (Recharts):** the rule is *"no `var()` in a Recharts
> DATA-SERIES prop"*, **not** *"no `var()` anywhere"*. `var()` is correct and theme-aware on
> plain SVG elements and on structural chart chrome (CartesianGrid, axis ticks). Scope the
> gate to `<Line>`/`<Area>`/`<Bar>`; a broader one will make the product worse.
> Probes strokes → `CHART_COLORS[4]`/`[1]`/`[2]`/`[0]` (same hex); ProbeForm a11y.
> ReportsPage needed no change; its single `--color-muted` is a dotted `borderBottom`
> (non-text → 3:1 applies → passes both themes), correctly left alone.

### (superseded) Wave 5 — original plan

**Pages:** Reports (scout L, 1085 lines), Probes (scout L, 1457 lines). Combined: L.
Wave 5 MAY need to split into 5a (Reports) + 5b (Probes) at the S34/S35 boundary if a single
session budget is insufficient; gate that decision at the start of the wave.

**Files:**
- `web/src/features/reports/ReportsPage.tsx`
- `web/src/features/probes/ProbesPage.tsx`
- Corresponding `__tests__/` files (ReportsPage + TenantsTab + ProbesPage — 75 unit tests)

**What changes:**
- Reports: `#FF5C68` inline `color:` style → `var(--color-error)` (scout: 1 hardcoded hex);
  32 px → `--space-*` tokens (scout: highest px count among pages); `<TierGate>` consumption
  (Wave 0); tab pattern → `<Tabs>` (Wave 0)
- Probes: 4 chart stroke hex → `CHART_COLORS[4]` (`#FFB224`), `[1]` (`#58A6FF`),
  `[2]` (`#A78BFA`), `[0]` (`#2CE5A7`); 44 px → `--space-*` tokens (scout: highest px
  count in codebase); `<TierGate>` consumption (Wave 0)
- Probes: ProbeForm validation logic — uipro §8 pass (error placement, aria-live, auto-focus);
  assess extraction to shared `useFormValidation` hook if logic is reusable with AlertRuleForm
- Probes: per-probe results timeline chart — uipro §10 real-time/historical chart pass;
  ReferenceLine `stroke` props → CHART_COLORS constants; skeleton state for chart loading
- uipro §1 pass on all changed components (contrast in both themes for 75 tests' worth of
  component variants)

**What must NOT change:**
- Schedule cron CRUD API
- Probe result query API shapes
- Tier-gate entitlement logic
- ProbeForm validation rules (error messages and field rules are UX concerns; the validation
  logic that drives them is product spec)

**Acceptance gates:** full per-wave checklist (§2.2). Probes test suite (75 tests) must stay
green.

---

## 5. Wave 1 recommendation

**Wave 0 (Shared Surface) is [S] and is recommended as the first wave to execute (S31).**

S30 budget is consumed by this scoping work order (D-092). Wave 0 does NOT fit S30.

Wave 0 is the correct first wave because it creates the extracted components (`TierGate`,
`Tabs`) that every subsequent page wave depends on. Executing any page wave first would require
inline duplication to be left in place, then cleaned up retroactively.

Wave 0 size justification: pure extraction with no logic changes; three source files already
contain the verbatim pattern (TierUpsell triplicated); `Tabs` pattern is identical across
six pages. No new API calls, no contract changes, no new dependencies. Component-level unit
tests for the two new components are the only new test surface. [S] is an honest estimate.

---

## 6. Sequencing note

§2.19 waves sit behind the operator-gated §2.18 marketplace tail. Five items remain in
`docs/operator-expected.md` (GHCR-public flip; final-assessment DRAFT review; Ant Media
marketplace contact rows 7–11; AMS license re-apply). Wave 0 unblocks when those gates clear
or the operator explicitly unblocks §2.19 ahead of them.

Does NOT touch `sdk/beacon-js` (no UI) or `server/` (no UI). Does NOT introduce new public API
endpoints or modify `contracts/`. Brand adoption ledger (`ROADMAP-V2.md §2.15`) is
cross-updated at close-out (Wave 5 completion), not per-wave.
