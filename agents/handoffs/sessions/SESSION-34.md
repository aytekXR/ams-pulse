# SESSION-34 — operator-intake gate + post-§2.19 (planned at S33 close, D-095)

> Written by SESSION-33 close (D-095, 2026-07-14). Paste-ready prompt for the next session.
> Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read `RESUME-PROMPT.md` +
> `ROADMAP-V2.md` §2 + the final-assessment §5 roadmap before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 and the final-assessment §5 roadmap and REVISE this
plan if a higher-leverage move exists. This file is a starting point, not a contract. Record any
revision in the D-096 open block. Carry this header into SESSION-35.md.

## ⛔ FIRST: is S33 merged?

**S33 (branch `s33-uipro-wave2`, PR #47) was pushed but COULD NOT MERGE.** Branch protection
requires 9 status checks and `gh pr merge --admin` is refused ("7 of 9 required status checks
have not succeeded"). The operator directed "skip ci runs" — CI can be skipped as a *wait*, but
not *at merge time*. The operator was asked to let the checks run, relax protection, or merge
from the UI.

**At open:** `git log --oneline origin/main -3` and `gh pr list --state open`.
- If **merged** → branch S34 off the new main and proceed.
- If **still open** → check `gh pr checks 47`. If green, merge. If the operator has not acted,
  **surface it as the top blocker** and do NOT stack new work on an unmerged branch.

**★★ STANDING RULE (D-095): a session claiming "DONE" is NOT evidence that it landed.** S33
opened to find S32's PR still open AND its branch missing a file its own gates had run against.
Verify `origin/main` at every open.

## Mission

Exit = (a) **operator intake applied.** The queue is now:
  - **⛔ the S33 merge** (branch protection — needs the operator or the checks).
  - **⏰ LICENSE RENEWAL — expires 2026-07-27T13:45Z.** From ~07-25 this is the TOP item: a
    lapse **+** the next AMS restart = total ingest death (both arms proven, D-092/D-093).
  - **SIX design gaps G1–G6** — ALL are `tokens.json`/brandkit edits, so **ONLY the operator may
    authorise them (D-071).** **G5 is the highest-value: a WRONG RATIO IN A BINDING TABLE**
    (design-rationale §2 claims muted = ~4.6:1 AA; it is **3.72:1**, below AA for normal text)
    that every future design decision reads. G3/G5/G6 are one-value changes — apply them the
    moment the operator rules, then re-run the WCAG pass.
  - **Two design questions:** the Analytics StatCard size, and the off-brandkit
    `rgba(224,82,82,…)` drop-chip tint on Ingest (a red that is in no token).
  - **Six marketplace items** (GHCR flip, trial-key mint, assessment review, Ant Media contact,
    MaxNodes ruling, matbu vhost).
(b) **§2.19 IS COMPLETE — Waves 0–5 all landed (S31→S33). Do NOT plan another UI wave.** The
    only remaining UI work is operator-gated (G1–G6).
(c) **Pick the next highest-leverage work** — candidates below.
(d) CI promotions if run date ≥ 2026-07-23 (else skip carry ×23).
(e) standing re-checks + AMS observation at open. PR-first, ≤2 pushes.

## S34 candidates (pick by leverage after intake)

1. **G3 + G5 + G6 token fixes [XS each, operator-gated]** — instant, high value, the moment the
   operator rules.
2. **e2e coverage for the six pages that still have NONE** — Ingest, Anomalies, Alerts, Settings,
   Reports, Probes. S33 wrote `analytics.spec.ts` + `fleet.spec.ts` from scratch; the other six
   have unit tests but **no browser spec**. This is **real residual risk**: S32's regression was
   caught *only* by a non-default spec, and Waves 3/4/5 changed all six of these pages.
   **Strongest technical candidate.**
3. **Prod rollout** — prod still runs `v0.3.0-34-g58a9c84` (since S27). A rollout now carries
   **D-089..D-095**. The UI waves are web-only (no runtime behaviour change) but the bundle is
   substantially different. Runbook: `deploy/runbooks/real-ams-go-live.md`.
4. **Marketplace tail** — unblocks the moment the operator's items land.
5. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** — re-surface only.

## ⚠️ Binding lessons from S33 — carry these into every wave

1. **A className is a CONTRACT with the stylesheet.** `web/src/styles/__tests__/focus-rings.test.ts`
   pins both halves (rule exists ⟺ a component uses it). **Any new bare styling className must be
   added to that map**, or the guard is silently incomplete.
2. **px → token: EXACT matches only** (scale 4/8/12/16/24/32/48/64/96). **And `width`/`height`/
   `minWidth` are DIMENSIONS, not spacing — never `--space-*`.** Radii have their own tokens.
3. **hex → `CHART_COLORS[N]` must be the SAME hex.** **`#FF5C68` is NOT in `CHART_COLORS`** — it
   is `--color-error`. `CHART_COLORS[3]` is **pink** (`#F06BB2`).
4. **Recharts RULE 3 is SCOPED:** no `var()` in a **data-series** prop (`<Line>`/`<Area>`/`<Bar>`
   `stroke`/`fill`). **`var()` IS correct** on plain `<svg>` elements and on structural chart
   chrome (CartesianGrid, axis ticks). A broader gate makes the product worse — S33 caught an
   agent breaking working code to satisfy exactly such a test.
5. **A test that never renders the component cannot fail for it.** ~16 deleted across S32/S33.
   Also beware tests that can't fail for subtler reasons (an `[aria-hidden].length > 0`
   assertion satisfied by a *different*, always-present element).
6. **Verify a mutation LANDED before trusting a GREEN.** `perl -0pi` without `/g` hit a doc
   comment instead of the JSX and reported a false green.
7. **Never add ARIA the code cannot honour.** `role="tab"` + roving tabIndex with **no key
   handler** does not merely under-deliver — it makes the tabs **keyboard-unreachable**.
8. **`--color-muted` may not carry text** (3.44:1 dark / 4.36:1 light). Fine for non-text
   (borders: the 3:1 bar applies).

## Gates (ORCH, before any commit)

- Web: `npm run gen:api && git diff --exit-code` (drift) + lint + build + `npx vitest run --coverage`
  (floors 59/54/45; **S33 census: 599 tests / 35 files**) + Playwright-docker
  (`mcr.microsoft.com/playwright:v1.61.1-noble`, `--network host`, mount `web/` at `/work`;
  **S33 census: 22/22 full suite**) + WCAG re-check on changed components
  (**design-rationale §2 is BINDING — but see G5: one of its rows is WRONG. Recompute; do not
  trust it**) + zero contract changes + `brandkit/` byte-untouched.
- **The §2.2 hex grep scans whole files — do NOT put hex literals in comments** in `src/features/`.
- **Don't overlap gate runs with heavy jobs** (a vitest run flaked at host load 19.8).
- Any Go change: FULL §8 (CI-faithful golang:1.25 docker, 0 FAIL / 0 unexpected SKIP, coverage
  ≥ floor 70.2, gofmt, vet, contract-drift). Not expected.
- docs/marketplace/ stays DRAFT-INTERNAL (D-081 external gate).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — and VERIFY the merge landed.** ≤2 pushes.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-096** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-35; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-35.md` (carry the standing directive header).
