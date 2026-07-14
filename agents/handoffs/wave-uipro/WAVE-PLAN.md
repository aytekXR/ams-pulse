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

---

## 4. Wave plan

**Shared surface goes FIRST (Wave 0).** Justification: `TierGate` and `Tabs` components
extracted in Wave 0 are consumed verbatim by five subsequent page waves. Doing extraction
last would require each wave to work against the triplicated/duplicated inline pattern, then
be retroactively refactored. Doing it first also makes the `--space-*` adoption pattern
concrete in a single low-risk extraction before any page wave authors px→token sweeps.

---

### Wave 0 — Shared Surface [S]

**Pages / surface:** No feature page changes. Shared component layer only.

**Files:**
- `web/src/components/TierGate.tsx` — new (extracted from Reports/Anomalies/Probes)
- `web/src/components/Tabs.tsx` — new (extracted from inline pattern in Analytics/QoE/Alerts/Reports/Fleet/Settings)
- `web/src/lib/chartColors.ts` — verify `CHART_COLORS[7]` is `'#7C93AD'` (complete the index; no new hex, only confirm)
- `web/src/features/reports/ReportsPage.tsx`, `web/src/features/anomalies/AnomaliesPage.tsx`, `web/src/features/probes/ProbesPage.tsx` — replace inline TierUpsell with `<TierGate>` import
- `web/src/__tests__/TierGate.test.tsx`, `web/src/__tests__/Tabs.test.tsx` — new unit tests for extracted components (coverage floors must hold)

**What changes:**
- TierGate: pure extraction of the triplicated TierUpsell pattern. Props interface from the
  existing pattern; no logic change, no new API call.
- Tabs: pure extraction of the repeated inline tab-button pattern. Props: `tabs: {id, label}[]`,
  `activeTab`, `onTabChange`. No logic change.
- chartColors.ts: verify the `CHART_COLORS` constant covers indices 0–7. No new hex.

**What must NOT change:**
- Zero public API changes. Zero contract changes.
- Tier-gate entitlement logic (only the render is moving; `LicenseContext` remains unchanged).
- No style behaviour changes — extraction is pixel-for-pixel equivalent.
- All 404 existing unit tests must still pass after extraction.

**Acceptance gates:** full per-wave checklist (§2.2). Additional extraction regression: run
`npx vitest run` before and after; diffs must not introduce new failures.

---

### Wave 1 — LiveOverview + QoE [M]

**Pages:** LiveOverview (scout size M), QoE (scout size S). Combined: M.

**Files:**
- `web/src/features/live/LiveDashboard.tsx`, `StatCard.tsx`, `StreamsTable.tsx`, `ProtocolDonut.tsx`, `useLiveDashboard.ts`
- `web/src/features/qoe/QoePage.tsx`
- `web/src/features/live/__tests__/`, `web/src/features/qoe/__tests__/`

**What changes (method passes applied):**
- ProtocolDonut: `#7C93AD` Cell fallback → `CHART_COLORS[7]` (scout: 1 residual hardcoded hex)
- LiveDashboard: 13 hardcoded px values → `--space-*` tokens (scout: 13 px)
- QoePage: Recharts `stroke` props `#58A6FF` → `CHART_COLORS[1]`, `#FFB224` → `CHART_COLORS[4]`
- QoePage: drop hex fallbacks from `var(--color-warning, #FFB224)` pattern — the tokens are
  confirmed stable; fallback hex is redundant and a future drift risk
- QoePage: 5 hardcoded px → `--space-*` tokens (scout: 5 px)
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

### Wave 2 — Analytics + Fleet [M]

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

### Wave 3 — Ingest + Anomalies [M]

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

### Wave 4 — Alerts + Settings [M]

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

### Wave 5 — Reports + Probes [L]

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
