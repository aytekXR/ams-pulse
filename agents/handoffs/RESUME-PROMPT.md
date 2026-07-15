# Pulse — Resume / handoff prompt (SINGLE source of truth)

> **This is the one handoff doc.** It supersedes the previous separate "next-session" prompt (merged 2026-06-29,
> D-037); don't recreate a second handoff file — update THIS one + `decisions.md` each session.
> Pulse = self-hosted analytics/QoE/alerting for Ant Media Server. Repo: `/home/aytek/repo/ams-pulse`
> on VPS `161.97.172.146`. Full decision log: `agents/handoffs/decisions.md` (D-001…D-057 + session notes, binding).
> **Plan of record: `agents/handoffs/ROADMAP.md`** (D-057; superseded `PRODUCTION-READINESS.md`,
> deleted by operator directive D-069). Session prompts: `agents/handoffs/sessions/`. AMS operator guide:
> `docs/AMS-INTEGRATION.md`. Go-live runbook + rollback: `deploy/runbooks/real-ams-go-live.md`.
> Operator creds/keys (gitignored, never commit): `oguz-testing.md`.

---

## ▶ START HERE (next session — execute `sessions/SESSION-43.md`)

**Session 2026-07-15 result: D-104 — S42 DONE (PR #81, audit OIDC first-login provisioning — every user-creation path is now recorded, in prod).**

**★ S42 CONFIRMED its plan (fourth non-pivot running).** Closed the one mutating path the S40 audit trail
deliberately left out: `oidc.go` provisions a user on **first SSO login** OUTSIDE the audited
`handleCreateUser` path, so that creation was never recorded (S40's own doc comment named it the top Phase-2
follow-up). New `oidcHandler.auditProvision` writes a `user.provision` `audit_log` entry with a **different
actor model** — no bearer token exists pre-session, so the SSO subject provisions itself
(`actor_user_id == object_id`, `actor_token_id` empty, `actor_name = "oidc:<sub>"`, `detail = {role,via,groups}`).
Placed **only in the create branch** (after the re-fetch that populates `user.ID`) → audited once per user,
never in the concurrent-UNIQUE-race branch (the winning login audits it). Same best-effort contract as
`s.audit` (cancel-detached, 5 s-bounded, log-on-failure). `audit.go` scope comment updated (now covered);
OpenAPI `action` desc += `provision`; `schema.d.ts` regenerated. Gated (Go 24/24 · vet · gofmt · web
tsc+build+vitest 650; new test **mutation-proven RED**; **adversarial review → no real defects**). CI all
required green. Merged, rolled to prod **`v0.4.0-25-g6a0226d`** (smoke: healthz all-ok, webhook 200, limits
`512M/0.5cpu`, logs clean). The path is dormant until OIDC is configured (off in prod), so the version stamp
is the proof of deploy. Full evidence: `decisions.md` D-104.

**⚠ CARRIED operator item (from S40, still unresolved):** the **AMS trial expiry is documented inconsistently** —
`deploy/runbooks/self-hosted-ams.md` says **2026-07-12**, the ledger says **2026-07-27** (live-verified
S37–S39). If it's 07-12 it has ALREADY lapsed. Cannot re-verify live (AMS creds operator-only; AMS enforces
the licence only on restart, so live-ingesting doesn't disambiguate). **Operator must confirm.** See
`operator-expected.md`.

**Next goal candidate (SESSION-43):** the audit trail now covers every user-creation path. Remaining bounded
tail: **admin-scope gating of the `GET /admin/audit-log` read** (currently auth-gated but not
admin-scope-gated — a read-only token can read the whole trail; product ruling first), the **two S34 e2e gaps**
[S], the dead **`PULSE_LICENSE_OFFLINE_FILE`** path [XS]. Team-management UI stays **blocked on the operator's
model ruling**; §2.7 CI promotions unlock **2026-07-23** (check the date at open — today is 07-15).
**Re-verify against the ledger + re-read the standing clause first.**

---

### Prior session (for context): D-103 — S41 DONE (PR #79, audit-log web UI — the trail is visible in the SPA, in prod).

**★ S41 CONFIRMED its plan.** Completed the self-contained half of "audit trail Phase 2": a read-only
**Audit Log** page (`/audit-log`) surfacing the S40 `GET /admin/audit-log` endpoint. `AuditLogPage.tsx`
mirrors `AnomaliesPage` (table + cursor "Load more" + Refresh; **no tier gate** — core admin feature);
router + left-nav wired. **Web-only — zero Go/contract change.** Gated (tsc · 650 vitest incl. 10 new · build
· 3 Playwright e2e in the docker image); CI all-green (no `csp-e2e` flake). Merged, rolled to prod
**`v0.4.0-23-ga44691b`** — UI proven served (AuditLogPage strings in the live JS bundle). Full evidence:
`decisions.md` D-103.

---

### Prior session (for context): D-102 — S40 DONE (PR #77, audit trail — actor on every admin/config write, in prod).

**★ S40 CONFIRMED its plan.** Built the compliance foundation the S36–S39 arc was
missing: an append-only **`audit_log`** recording "who changed what, when" for every mutating admin/config
API call (gates SOC 2 / ISO 27001 buyers). `s.audit(...)` is threaded into **24 handlers** (create/update/
delete of alert rules & channels, users, tokens, probes, report schedules, AMS sources, tenants + licence
activation); the actor comes from the bearer token already in `ctxTokenKey` (no new middleware); `detail` is
a non-sensitive summary only. `GET /admin/audit-log` reads it back (keyset, newest-first). Migration 0004
(SQLite idempotent + PG embed); OpenAPI + `schema.d.ts`. **Documented out-of-scope, not silent:** the two
`/test` fires, `/auth/oidc/logout`, and OIDC auto-provisioning (different actor model). **The adversarial
review found + I fixed one real defect** — two update handlers audited *after* the post-update re-fetch
guards, so a committed mutation could go unrecorded on a failed re-read (moved the audit before the
re-fetch). CI caught a PG migration-parity gap (fixed); the `csp-e2e` flake recurred (required `web-e2e`
green). Gated, merged, rolled to prod **`v0.4.0-21-g0b7decc`** — migration 0004 proven live (WAL-aware copy:
`audit_log` present, 10 columns). Full evidence: `decisions.md` D-102.

---

### Prior session (for context): D-101 — S39 DONE (PR #75, out-of-band licence-expiry alerting, in prod).

**★ S39 CONFIRMED its plan.** Built a **`license_expiry`** alert metric (faithful `cert_expiry` mirror) that
warns through the operator's channels when the Pulse key nears expiry — closing the D-098 UI-banner-only gap.
`serve.go` adapts `license.Manager.ExpiresAt()`; free/perpetual keys are skipped, expired keys fire. The
adversarial review (clean) still moved the work: it flagged that the unit tests called the setter directly,
so I added a **`wireAlertLicenseExpiry` seam + mutation-proven wiring pin** proving `serve.go` wires the
checker into the real evaluator. Merged, rolled to prod `v0.4.0-19-g38111c9`. Operator action: none for the
build (rule + channel still operator-created). Full evidence: `decisions.md` D-101.

---

### Prior session (for context): D-100 — S38 DONE (PR #73, /admin/users correctness, in prod).

**★★ S38 DISCARDED ITS OWN PLAN — right again.** SESSION-38.md named the **team-management UI**
(`/admin/users` CRUD exists, no page — the top D-098 funnel gap). Verify-at-open found the feature is
**advisory, not real**: the stored per-user `role` **does not govern SSO sessions** (OIDC re-maps role from
IdP groups on every login and never reads the stored value) and **there is no password login** (SSO
auto-provisions users). So a UI role-edit would change nothing. S38 instead **fixed the API's real
correctness bugs** — `handleUpdateUser` blanked the username on a role-only edit, returned 200 for a missing
id, and echoed a fabricated `created_at:0`; create accepted any role and 500'd on duplicates — and
**deferred the UI + password-login to an operator product ruling** (operator-expected item 10). Full-replace
matching the contract, role allowlist, duplicate→409, 409 documented in the spec. Gated (Go 24/24 · web
tsc+vitest · schema.d.ts in sync; every guard mutation-proven RED; adversarial review → 3 findings all
fixed), merged, rolled to prod. Full evidence: `decisions.md` D-100.

---

### Prior session (for context): D-099 — S37 DONE (PR #71, tier-entitlement enforcement, in prod).

**★★ S37 DISCARDED ITS OWN PLAN — right again (three sessions running).** SESSION-37.md named
"§2.16 AMS early-warning," but a `grep` of ROADMAP-V2 at open proved it had **already shipped S25/S26
(D-087/D-088)** — the "deferred twice" note was a planning error propagated across handoffs. So S37
became a **tier-entitlement enforcement audit** — generalizing the D-098 bug class (*capability stored
but never checked*) across every paid feature. **Six gaps fixed:** SSO/OIDC now Enterprise-gated
(closing the D-098 "unenforced revenue" funnel row); white-label report headers, alert-channel type
(update + test-fire), the report scheduler's timer, and retention on five analytics/probe read paths.
The close-out **adversarial review caught two gaps in my own sweep** — an unclamped probe-results read
and an *untested* callback gate (deletable with zero failures, the S36 vacuous-test trap) — both fixed
and mutation-proven. Gated (Go 24/24 · gofmt · web tsc+vitest; every guard mutation-proven RED),
merged, rolled to prod. Full evidence: `decisions.md` D-099.

**Next goal candidate: team-management UI** (`/admin/users` CRUD exists, no page — highest-value
sell-readiness gap) or out-of-band licence-expiry alerting — **but re-verify against the ROADMAP
ledger and re-read the standing-directive clause first** (S37's lesson: the named goal was stale).

---

### Prior session (for context): D-097 — S35 DONE (PR #51 `425b04b`, prod `v0.4.0-11-g425b04b`).

**★★ S35 DISCARDED ITS OWN PLAN — and that was the right call.** S34 had planned "close two e2e
gaps, then build §2.16". The operator instead asked *"have you finished all development? is
installation and generating license keys ready?"* — so S35 **executed** the docs rather than
reading them. **The answer was no, on both counts.**

42 agents ran every documented command against a live system; every finding then faced an
adversarial verifier whose job was to kill it (**36 raw → 33 confirmed, 3 refuted**).
**3 blockers, 10 majors.** Everything a session was permitted to fix is fixed, merged and **live
in prod**.

### What was actually broken (evidence: D-097)

- **`GET /api/v1/reports/export` did not exist** — yet the Reports page shipped **Export CSV** and
  **Export PDF** buttons wired to it. A paying Business customer clicked Export and got a **404**.
  *A missing feature, not a doc bug.* **CSV implemented; PDF removed rather than left broken.**
- **The analytics Export CSV button had never worked either** — it authenticated with `?token=` in
  the URL, which `bearerAuthMiddleware` **deliberately ignores**, so it answered **401**.
- **`docs/licensing.md` documented an activation API that does not exist** (`POST
  /api/v1/license/activate` vs the real **`PUT /api/v1/admin/license`**) — under a heading titled
  *"Verify activation."* Verifying a key you had just **sold** returned 404.
- **`make up` / `docker compose up -d` — install.md's own primary command — always failed.**
- **The README Quick Start silently monitored a MOCK AMS** — real address set, no data, no error.

### Two things previous sessions told you that were WRONG

- **"No customer can install Pulse" was overstated.** The **clone-and-build path works** (verified
  from a clean clone; it never touches GHCR). Only the **one-command quickstart** is dead. Two
  paths, two fates — never flatten them again.
- **The vendor key ceremony is DONE** (S16/D-077), not open. Redoing it would **invalidate every
  outstanding key**, including the enterprise licence in prod.

### ★★ THE PRODUCT STILL IS NOT SHIPPABLE — AND IT IS STILL NOT ENGINEERING

> **GHCR is private.** Anonymous pull → **401**, so the one-command quickstart is dead for every
> user. **One click.** No session can do it.
> **The AMS licence expires 2026-07-27T13:45Z.** From ~07-25 it outranks GHCR: a lapse **plus**
> the next `antmedia` restart = **total ingest death** (D-092/D-093).

### At open — VERIFY, do not assume (D-095)

- `origin/main` should be **`425b04b`**; prod should print **`v0.4.0-11-g425b04b`**.
- **Cheapest real check in the repo** — prove the S35 fix survived:
  ```sh
  curl -s -o /dev/null -w "%{http_code}\n" --resolve beyondkaira.com:443:161.97.172.146 \
    "https://beyondkaira.com/api/v1/reports/export?from=1&to=2&format=csv"
  # 401 = route exists (correct).  404 = the rollout did not survive; re-run the runbook.
  ```

### S36 goal

**§2.16 AMS operational early-warning** — already OPERATOR-APPROVED (D-086 addendum), the largest
unblocked feature left, and now first in line because S35 cleared the ship blockers ahead of it.
Full candidate list, gates, and the binding lessons: **`sessions/SESSION-36.md`**.

### Two lessons that cost real time — do not repeat them

1. **RUN the doc; do not read it.** Every S35 blocker had passed prior review.
2. **A gate you cannot point at in the repo is not a gate.** S35's own prompt asserted a
   "hex-literal grep"; an agent reported it RED with 35 matches; **no such gate exists**. Trusting
   it would have mangled nine files. (Same trap: `git diff --exit-code` reads RED against this
   repo's permanently-dirty tree — scope drift checks to `schema.d.ts`.)

---

## ▶ prior session context (S33 Wave 2, superseded by the above)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-34.md`)

**Session 2026-07-14 result: D-095 — S33 DONE (★★ caught S32 SHIPPING A TREE IT NEVER
COMMITTED — PR #46 was still open and its branch was missing the CSS rule its own gates had
run against; ★ §2.19 Wave 2 landed: Analytics + Fleet + shared SegmentedControl; ★★ deleted
12 tests that could never fail; ★ THREE new operator gaps — including the brandkit's own WCAG
table being WRONG).**

- **OPERATOR INTAKE: no new signals; all standing items OPEN; NONE blocked Wave 2.** No
  operator commits or file drops since S32. GHCR anonymous pull still **401** (private). No
  trial-key / assessment-review / Ant-Media-contact / MaxNodes / matbu signals. G1/G2
  unanswered (Wave 2 adds no forms, no icons). **G3 unanswered** (a `tokens.json` change a
  session may NOT self-approve). ⏰ **License renewal due 2026-07-27T13:45Z** (13 d at open).
  AMS healthy at open (`s33open` sweep **byte-identical** to baseline; teststream
  broadcasting, `publishers=1`, 0 poll errors). **CI promotions skip carry ×22.**
- **★★ THE HEADLINE — S32 GATED A TREE THAT DOES NOT EXIST IN GIT.** At S33 open, **PR #46
  was still OPEN** (S32's docs said "DONE"; `origin/main` was still at S31). And its branch
  was **missing a line**: S32 committed `QoePage.tsx` with `className="filter-input"` and a
  comment promising *"focus ring provided by `.filter-input:focus-visible` in global.css"* —
  but **never staged that global.css rule**. Working-tree mtime (04:42Z) **predates** the
  commit (05:24Z): the gates ran green against a file that was never committed. Merging as-is
  would have shipped two filter inputs with **no branded focus ring**, behind a comment and
  two tests promising one.
- **★★ WHY THE TESTS COULDN'T CATCH IT — a whole blind spot, now closed.** They asserted
  `toHaveClass("filter-input")`, which is true **whether or not any rule matches the class**.
  Nothing in TypeScript ties a bare className to a stylesheet. **A className with no rule is
  a false a11y promise; a rule with no user is dead CSS.** New `styles/__tests__/
  focus-rings.test.ts` pins **both halves** for every CSS-only class, parsing the real
  stylesheet. **RED-proven against S32's actual commit.** Fixed on the branch that caused it;
  PR #46 re-gated 15/15 and **merged**. **STANDING RULE: verify the previous session actually
  merged — "DONE" in a doc is not evidence that it landed.**
- **★ §2.19 WAVE 2 DONE (Analytics + Fleet).** Chart hex → `CHART_COLORS[N]` (same hex);
  Fleet's memory-healthy bar stays **dataviz blue, never `statusColors.healthy`**; 18 px →
  `--space-*` **exact matches only**; **`<SegmentedControl>` extracted** (`role=radiogroup`,
  **NOT `tablist`** — a tablist promises tabpanels that don't exist, the same false-promise
  class as S32's `aria-sort`); **`<StatCard size="compact">`** (a 1:1 swap was NOT
  pixel-neutral: padding 14→24px, value 24→40px). `--color-muted` eliminated from both pages
  + shared `Badge`/`StatCard` (it fails AA at every size these pages use).
- **★ THE PLAN WAS WRONG IN THREE PLACES, all corrected against reality:** Fleet's
  `var(--color-warning, #hex)` fallbacks were **dead AND stale** (drop them, don't keep them);
  **`width: 32` is a DIMENSION, not spacing** — never `--space-*`; the StatCard swap needed a
  variant. **The px→token trap now has a cousin: width/height/minWidth are not spacing.**
- **★★ 12 TAUTOLOGICAL TESTS DELETED.** Each asserted `STATUS_COLORS[cpuStatus(85)] ===
  '#FF5C68'` — two values the test file imported and composed **itself**, never rendering the
  component. **One was worse than vacuous:** it pinned healthy memory as GREEN while the
  component deliberately paints it **BLUE** — asserting a value the component never uses, with
  **no render-level pin on the real behaviour at all**. Replaced with DOM-reading pins in both
  themes. **4 mutations RED-proven** — and **one of my own mutations produced a FALSE GREEN**
  (`perl` without `/g` hit a doc comment, not the JSX). **Verify a mutation LANDED before
  trusting a green** (D-091 class, 2nd occurrence).
- **★ THREE NEW OPERATOR GAPS (all verified, not asserted):**
  **G4 — touch targets:** brandkit's `minTouchTarget=44` is **WCAG AAA**; the **AA** bar is
  **24×24**, which today's ~28px buttons already pass. Enforcing 44 would visibly retheme every
  button and fights brandkit's own desktop-density spec. **Deferred, not skipped** — a
  pixel-neutral wave may not make that call. Coupled to G1.
  **G5 — the brandkit WCAG table is WRONG:** design-rationale §2 (BINDING) claims muted =
  **~4.6:1 AA**; recomputed it is **3.72:1** — *below* AA for normal text. **The table's own
  guidance is unsafe, and every future wave reads that table.** This retroactively justifies
  the muted→secondary sweeps.
  **G6 — light-theme info Badge = 2.32:1** (`--color-info` is intentionally not overridden for
  light). Needs a `color.light.info` token. All three are brandkit/`tokens.json` → **operator's
  to authorise (D-071).**
- **★ NEW e2e: `analytics.spec.ts` + `fleet.spec.ts`** — **neither page had one.** The Analytics
  spec pins the **real Recharts SVG stroke attributes** (jsdom structurally cannot) and they are
  byte-identical to the pre-refactor hex.
- **Gates:** web **548/548** (S32: 515) / 35 files; coverage **67.93/63.37/57.11** vs floors
  59/54/45; lint + build clean; gen:api in sync; **Playwright 16/16** (default four + the two
  new specs); `contracts/` + `brandkit/` byte-untouched; zero bare hex + zero `--color-muted`
  on both pages. No Go changes. Workflow: 10 agents, 0 errors.
- **S34 carries:** §2.19 **Wave 3 (Ingest + Anomalies) [M]** primary — **★ it carries an
  unresolved colour question the plan could not settle: an Ingest series uses `#FF5C68`, and
  whether it means "error" (→ `var(--color-error)`) or is a plain dataviz series (→
  `CHART_COLORS[3]` = `#F06BB2`, a DIFFERENT hex) must be read from the code, not guessed**;
  **G3+G5+G6 token fixes [XS, operator-gated]**; **G4 ruling**; license renewal before 07-27;
  marketplace tail (operator items).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-34.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan; operator intake
FIRST — six standing items + **G1…G6** + the StatCard look question; **VERIFY S33 ACTUALLY
MERGED before trusting it**; AMS re-sweep at open WITHOUT any PULSE_TOKEN prefix;
`ams-teststream` does NOT auto-restart across a reboot; **SRT publishes use the PLAIN
streamid**; CI promotions if ≥07-23 else skip carry ×23). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST.

---

## ▶ prior session context (S32, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-33.md`)

**Session 2026-07-14 result: D-094 — S32 DONE (★ §2.19 Wave 1 landed: LiveOverview
+ QoE on brandkit tokens + real ARIA; ★★ the verify net caught the build shipping a
FALSE ARIA PROMISE and three TAUTOLOGY tests; ★★ e2e caught a real regression the
default gate list would have missed).**

- **OPERATOR INTAKE: no new signals; all standing items OPEN.** No operator commits
  or file drops since S31. GHCR anonymous pull still **401** (private). No trial-key
  / assessment-review / Ant-Media-contact / MaxNodes / matbu signals. **G1, G2 and
  G3 unanswered** — none blocks a wave (Wave 1 adds no forms, no icons; **G3 is a
  `tokens.json` change a session may NOT self-approve**). ⏰ **License renewal due
  2026-07-27T13:45Z.** AMS healthy at open (`s32open` sweep byte-identical to the
  pre-expiry baseline; teststream broadcasting, `publishers=1`, 0 poll errors).
- **★ §2.19 WAVE 1 DONE:** chart hex → `CHART_COLORS[N]` (same hex — ProtocolDonut
  `[7]`, QoE `[1]`/`[4]`); **stale hex fallbacks dropped** from
  `var(--color-warning, #FFB224)` / `var(--color-error, #FF5C68)` (both vars exist in
  BOTH themes, and the light values `#B45309`/`#DC2626` DIFFER from the fallback hex —
  they would have rendered the wrong colour if ever reached); a11y — StatCard
  accessible names, donut aria-labels, `role=grid/rowgroup/row/columnheader` on
  StreamsTable. Virtualization/columns/sort untouched.
- **★★ THE px→TOKEN TRAP (binding for every remaining wave):** the `--space-*` scale
  is 4/8/12/16/24/32/48/64/96. **Substitute ONLY where the token EQUALS the literal.**
  Every non-matching literal (6/20/36/160/180/260/520px + all typography sizes) is
  LEFT ALONE. **Snapping 13px → `var(--space-3)` (12px) is a silent 1px regression** —
  these waves may not change pixels. Verifier re-derived all 26 substitutions: all EQUAL.
- **★★ THE BUILD SHIPPED A FALSE ARIA PROMISE — caught pre-merge.** `aria-sort="none"`
  was added to the Viewers/Bitrate headers, which have **no sort handler**. That lies
  to assistive tech. Removed; tests now pin its ABSENCE.
- **★★ THREE TAUTOLOGY TESTS — caught pre-merge.** Each asserted its own expression,
  never the component (the ProtocolDonut one evaluated
  `PROTOCOL_COLORS[key] ?? CHART_COLORS[7]` **in the test body** — swapping the
  component's fallback to `[0]` left all 18 tests GREEN). All rewritten to render the
  component; each **RED-proven** under sabotage. **A test that never touches the
  component cannot fail for the component.**
- **★★ e2e CAUGHT A REAL REGRESSION THE DEFAULT GATE LIST WOULD HAVE MISSED.**
  `streams-virtualization.spec` is NOT in the §2.2 default Playwright set; it was run
  because Wave 1 touches StreamsTable. Moving `role="grid"` to the header-owning
  container (correct ARIA 1.2) meant the spec was setting `scrollTop` on the OUTER
  `overflow:hidden` wrapper — **a silent no-op; the virtualizer never advanced.** Users
  unaffected; the test's handle was wrong. Fixed with an explicit
  `data-testid="streams-scroll"`. **STANDING RULE: run the specs of the components a
  wave TOUCHES, not just the default gate list.**
- **Gates:** web **515/515** (S31: 452) / 33 files; coverage **67.42/62.77/56.29** vs
  floors 59/54/45; lint + build clean; gen:api in sync; **Playwright 10/10**;
  `contracts/` + `brandkit/` + `package.json` byte-untouched; zero new hex/px in source.
  No Go changes. **Honest note:** one `vitest --coverage` run reported 2 failures while
  Playwright + a build ran concurrently (host load **19.8**) — two clean re-runs
  returned 515/515. **Don't overlap gate runs with heavy jobs on this box.**
  CI promotions **skip carry ×21**. Workflow: 8 agents, 0 errors.
- **S33 carries:** §2.19 **Wave 2 (Analytics + Fleet) [M]** primary; **`<SegmentedControl>`
  extraction** (Fleet's cards/table toggle is NOT a `<Tabs>` candidate — S31 finding, and
  Wave 2 is the wave that touches Fleet); **G3 token fix [XS, operator-gated]**; license
  renewal intake before 07-27; marketplace tail (operator items).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-33.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan; operator intake
FIRST — six standing items + G1/G2/**G3**; AMS re-sweep at open WITHOUT any PULSE_TOKEN
prefix; `ams-teststream` does NOT auto-restart across a reboot; **SRT publishes use the
PLAIN streamid**; CI promotions if ≥07-23 else skip carry ×22). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST.

---

## ▶ prior session context (S31, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (execute `sessions/SESSION-32.md`)

**Session 2026-07-14 result: D-093 — S31 DONE (★★ SRT INGEST LIVE-VALIDATED,
FIRST EVER — TC-I-05-SRT PASS 2/2, blocked-scenario list now EMPTY; ★★ the
scenario's streamid format had been WRONG since S29, hidden behind two honest
SKIPs; ★ §2.19 Wave 0 landed — shared TierGate + Tabs; ★ NEW operator gap G3).**

- **★ DEAD-SESSION TREE AT OPEN (4th occurrence of the class).** The branch
  `s31-uipro-wave0` already existed carrying S30's addendum commit `f703634`
  **plus an uncommitted partial Wave 0 from a crashed earlier S31 run** (the VPS
  rebooted 02:02Z mid-session). Per D-082/D-086/D-091 it was **re-audited from
  scratch, not trusted** — and the audit paid: it found a vacuous icon test, a
  vacuous active-underline pin, and two WCAG contrast failures. **Never trust a
  tree you did not gate.**
- **★★ SRT INGEST LIVE-VALIDATED — TC-I-05-SRT PASS (2/2)**, 02:29:45Z, evidence
  `qa/realams/evidence/TC-I-05-SRT-20260714T022945Z/`: accepted in **2 s**,
  `bitrate=1,148,432 bps`, `packetLostRatio=0.0`. **Blocked-scenario list EMPTY**
  (was [SRT ingest, RTMP ingest (new), any fresh-publish scenario]).
- **★★ WHY IT NEVER RAN — the fixture was broken, and two gates hid it.** S29
  wrote the streamid in SRT Access-Control form (`#!::h=LiveApp/<id>,m=publish`).
  **AMS EE 3.0.3 splits the streamid on `/` and treats the left side as the app
  scope WITHOUT stripping the ACF prefix** → `ERROR SRTAdaptor - There is no
  scope for incoming stream id. Parsed scope: #!::h=LiveApp`. Both ACF spellings
  (`h=`, `r=`) were probed live and both fail; **the plain
  `streamid=<App>/<streamId>` form ingests cleanly**. The license refusal (S29)
  and then the CPU admission guard (S30) refused the connection *before the
  parser was ever reached*. **LESSON: a SKIP that never reaches the code under
  test proves nothing about it.** A second defect surfaced on the first real run:
  the scenario asserted `bitrate>0` five seconds in, but AMS reports bitrate from
  a **rolling window** (legitimately 0 for ~10 s) — it was **failing a healthy
  stream**. Both fixed; resource-guard SKIP arm adopted (a busy box is not a
  broken product).
- **★★ ENFORCEMENT MODEL CLOSED (both arms).** The VPS rebooted 02:02Z and
  `antmedia` restarted — the **FIRST restart since the S30 license was applied** —
  and **ingest came straight back** (teststream re-accepted immediately, zero
  refusal lines). ⇒ **a VALID license survives a restart cleanly**; D-092's
  ingest-death needs a **LAPSED license AND a restart**. ⏰ **Key expires
  2026-07-27T13:45Z** — renewal is the top intake item from ~07-25.
- **★ SRT IS ATTRIBUTED AS RTMP (LIM-23, honest disclosure — not a defect):** AMS
  returns `publishType: "RTMP"` for SRT ingest; Pulse copies it verbatim
  (`amsclient/client.go:88`), so SRT counts as RTMP in the protocol breakdown.
- **★ §2.19 WAVE 0 DONE:** shared `TierGate` (was triplicated verbatim) + `Tabs`
  (was copy-pasted ×3; now `role=tablist/tab`, `aria-selected`, roving tabindex,
  Arrow/Home/End — **none of the inline copies had any ARIA or keyboard nav**).
  Adopted in Analytics/Alerts/Reports; **Settings diverges → Wave 4; Fleet's
  cards/table toggle is a SEGMENTED CONTROL, never a `<Tabs>` candidate.** Two
  **deliberate WCAG fixes** (mandated by the BINDING §2.2 gate — an extraction may
  not ship a component that fails contrast): muted → secondary (3.50:1 → 8.18:1).
  **Plan corrected against reality:** the tab pattern is on **4** pages, not the 6
  the plan assumed; `CHART_COLORS[7]` already existed (verify-only).
- **★ NEW OPERATOR GAP G3:** the light-theme "Upgrade License" CTA fails WCAG AA
  (**3.12:1**, white on `#0BA678`). **Pre-existing; Wave 0 neither caused nor fixed
  it.** The fix is `tokens.json color.light.accent` → `#087A59` (5.33:1) and
  **brandkit is the operator's to change (D-071) — NO waiver has been granted.**
  (An agent's draft claimed one was "operator-approved"; that was FALSE and was
  corrected in three places. Sessions do not self-approve operator decisions.)
- **Operator intake: all standing items re-verified live, still OPEN** — GHCR
  anonymous pull still 401/403 (private); no trial-key / assessment-review /
  Ant-Media-contact / MaxNodes signals; matbu vhost ruling pending (on-disk
  Caddyfile untouched, still the only uncommitted file); G1/G2 unanswered (Wave 0
  needed neither). 10th sweep byte-identical. uipro-vs-brandkit assumption STANDS.
- **Gates:** web **452/452** (was 404) / 32 files; coverage 67.17/62.05/56.21 vs
  floors 59/54/45; lint + build clean; gen:api in sync; Playwright
  dashboard-render/auth-gate/csp/prefs green; contracts/ + brandkit/ byte-untouched;
  zero new hex/px (hex debt REDUCED); bash -n + shellcheck clean. No Go changes.
  CI promotions skip carry ×20 (07-14 < 07-23). 13 agents, 0 errors.
- **S32 carries:** §2.19 **Wave 1 (LiveOverview + QoE) [M]** primary; **G3 token
  fix [XS, operator-gated]**; license renewal intake before 07-27; marketplace tail
  (operator items); optional `<SegmentedControl>` extraction when a wave touches Fleet.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-32.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan; operator
intake FIRST — six standing items + G1/G2/**G3**; AMS re-sweep at open WITHOUT any
PULSE_TOKEN prefix; teststream does NOT auto-restart across a reboot →
`docker start ams-teststream`; **SRT publishes must use the PLAIN streamid**;
CI promotions if ≥07-23 else skip carry ×21). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST.

---

## ▶ prior session context (S30, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (execute `sessions/SESSION-31.md`)

**Session 2026-07-13/14 result: D-092 — S30 DONE (★★ AMS INGEST-DEAD
FINDING: the S22 restart-enforcement hypothesis CONFIRMED — the first
post-lapse AMS restart (crash 22:14Z + docker auto-restart) extended
license enforcement to ALL new RTMP ingest while REST stays
byte-identical Enterprise; ★ §2.19 SCOPING WO COMPLETE: 6-wave plan +
vendored-skill DO_NOT_COMMIT ruling; ★ PDF disposition CLOSED by
operator staging).**
- **★ OPERATOR INTAKE (mission (a)): all items OPEN, re-verified live**
  — 9th byte-identical REST sweep (license NOT landed; sweep run bare
  per the S29 PULSE_TOKEN gotcha); GHCR anonymous still denied; no
  key/review/contact/MaxNodes signals. **PDF CLOSED:** the operator
  staged it pre-session (01:29:54 index mtime) = "commit to docs/";
  content READ for the first time (dockerized poppler — the host has
  none): it is a RENDERING of the already-public docs/prd-report.md
  (heading-diff clean) → committed as staged, zero new exposure,
  "drop the pdf" reverses. His two Caddyfile .bak deletions = his own
  cleanup, closed. **matbu vhost ruling still pending** (on-disk
  Caddyfile block stays uncommitted — bcrypt hash, public repo, D-062
  4th). uipro-vs-brandkit assumption STANDS unconfirmed.
- **★★ AMS INGEST-DEAD (the headline):** antmedia crashed ~22:14Z
  under VPS load 20 (concurrent operator sessions: hayati flutter
  tests + evrak pilot + 3 claude), docker auto-restarted 22:21:31Z =
  FIRST post-lapse process restart (boot history in-log: 07-12 06:53Z
  pre-lapse → 07-13 22:21Z). Post-restart: REST byte-identical BUT
  every new RTMP publish refused — `AcceptOnlyStreamsInDataStore -
  License is suspended and not accepting connection` (both apps,
  fresh ids; the first teststream-retry rejection was the CPU guard
  at load 20 — red herring, correctly separated). Teststream CANNOT
  return until the license lands; blocked-scenario list now [SRT
  ingest, RTMP ingest (new), any fresh-publish scenario]. Docs
  corrected: AMS-INTEGRATION "RTMP unaffected" de-staled + S30 note;
  validation-environment §9 row. Evidence:
  `qa/realams/evidence/S30-rtmp-license-block-20260713T2353Z/`.
  operator-expected item 1 ESCALATED (his AMS looks healthy on REST
  but accepts no new streams).
- **★ §2.19 SCOPING WO (mission (b)) DONE** — workflow 6 agents (3
  scouts + author + 2 adversarial verifiers), 0 errors. `uipro init
  --ai claude --offline` ran (CLI v2.11.0; 143 files / 7 skills).
  **Vendored verdict DO_NOT_COMMIT + independent commit-gate REJECTED:
  core ui-ux-pro-max has NO license grant (public-repo blocker);
  design/ makes live Gemini calls; CDN-font content ×74; ui-styling
  pushes shadcn/Tailwind (stack absent). RULING: .claude/skills/
  local-only + GITIGNORED, bootstrap in WAVE-PLAN §1.1b.** Plan:
  `agents/handoffs/wave-uipro/WAVE-PLAN.md` — method (search.py
  targeted queries, values ALWAYS discarded for tokens), conflict
  ledger C1–C6 all token-wins + operator gaps G1 (mobile ≥16px input
  font vs 14px body token) / G2 (icon library ruling), 6 waves: W0
  Shared Surface [S] (TierGate triplicated ×3 pages + Tabs pattern ×6
  → extract), W1 Live+QoE, W2 Analytics+Fleet, W3 Ingest+Anomalies,
  W4 Alerts+Settings [M each], W5 Reports+Probes [L]. Ground truth:
  404 web tests / 30 files; 21 residual hex (all Recharts stroke= →
  CHART_COLORS[], never var()); ~200 px literals. **planVerify
  PARTIAL → must-fix remediated same-session** (drafted plan dropped
  the gen:api drift gate — re-added verbatim; C3 citation fixed).
  **Wave 0 → S31** (the plan's own honest call; W0 = real web change
  needing full gates on a contended box).
- **Gates:** docs-only session (no Go/web/contract changes) — no code
  gates due; PR CI runs the full suite. Prod healthy at open (healthz
  all-ok, 0 poll errlines); pulse-realams healthy; AMS never touched
  (publish probes = the sanctioned S22 class). CI promotions skip
  carry ×19. Workflow: 6 agents, 0 errors.
- **★★★ LATE-SESSION (post-#44-merge): THE LICENSE LANDED AND WAS
  APPLIED — INGEST RESTORED.** Operator pasted the key (stored ONLY in
  gitignored oguz-testing.md; expires **2026-07-27T13:45Z**). Applied
  via REST POST /rest/v2/server-settings (PUT → 405, the S18 pattern;
  success:true + readback) — enforcement did NOT lift while running →
  `docker restart antmedia` (operator-sanctioned) → **teststream
  ACCEPTED, count=1, HLS flowing, realams publishers=1; s30postlicense
  sweep BYTE-IDENTICAL to the pre-expiry baseline** except 6 prod
  poll-errlines all inside the 00:44:28–:55Z restart window
  (self-healed). TC-I-05-SRT: license gate CLEARED (handshake reached
  the ACF callback, first time ever) but rejected by AMS's
  high-resource guard (load 14) — re-run needs a low-load window;
  scenario-gap filed (resource-rejection mislabels as FAIL, add SKIP
  arm, XS). Evidence + full detail: D-092 addendum (rides S31 PR).
- **S31 carries:** §2.19 Wave 0 [S] primary (TierGate + Tabs
  extraction per WAVE-PLAN §4 W0; full web gates incl. gen:api drift +
  Playwright light/dark); **TC-I-05-SRT re-run in a low-load window
  [license gate cleared — resource window only] + TC-I-05 SKIP-arm fix
  [XS]**; **license renewal intake before 2026-07-27** (post-lapse
  restart = ingest death, proven); Enterprise-surface re-validation
  tail if any; matbu ruling; G1/G2 asks; marketplace tail the moment
  operator items land; CI promotions if ≥07-23 else skip carry ×20.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-31.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST — five standing items + matbu ruling + G1/G2 +
uipro/brandkit confirmation; AMS re-sweep at open WITHOUT any
PULSE_TOKEN prefix, observe-only — a non-null diff OR a teststream
publish suddenly ACCEPTED = the license landed → run the full
re-validation chain; NEVER restart/fix AMS). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST.

---

## ▶ prior session context (S29, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-30.md`)

**Session 2026-07-13/14 result: D-091 — S29 DONE (★ F10 TAIL COMPLETE:
RTMP AMF0 connect live-validated + probe-stats UI columns; ★ FIRST
post-expiry enforcement delta found — SRT ingest license-gated and
rejecting; known-limitations 18→22; NEW operator directive → §2.19
uipro UI/UX refactor).**
- **★ OPERATOR INTAKE (mission (a)): ALL SIX items OPEN, re-verified
  live** — 8th byte-identical sweep (license NOT applied; sweep gotcha:
  the `PULSE_TOKEN=<any>` prefix SUPPRESSES token auto-extract →
  parse-err; run the sweep WITHOUT it); GHCR anonymous pull 401; no
  key/review/contact/MaxNodes signals. **Caddy-vhost decision CLOSED by
  operator action:** `80df0ab "bedirhan site"` committed by the
  operator onto local main (his authorship; on-disk prod file
  byte-identical; carried to origin at S29 close). **NEW operator-side
  artifact:** `docs/ant-media-marketplace-opportunity-report.md.pdf`
  (8pp, untracked, content unread — no PDF tooling on host; operator
  asked what to do with it).
- **★ F10 TAIL DONE (W1+W2, live-validated):** probe_rtmp.go AMF0
  connect round-trip (hand-rolled AMF0 + chunk demuxer honoring
  SetChunkSize; app_accepted/app_rejected/rtmp_connect_timeout;
  no-app URL keeps legacy path; description-only contract CR; live vs
  real AMS: app_accepted, 281-byte wire fixture committed — RTMP
  connect WORKS under the suspended license); ProbesPage Signaling
  badge + Connect ms columns (the S15-noted UI gap; 407 vitest).
  V1 must-fix remediated: SetChunkSize handler test added, mutation
  re-proven RED in a pristine copy (the live fixture never
  renegotiates — fixture-only coverage was a hole).
- **★ SRT (W3, license-BLOCKED honestly):** AMS EE SRTAdaptor rejects
  every SRT publish "License is suspended" (RTMP unaffected) — the
  FIRST feature-level post-lapse enforcement observed (8 REST sweeps
  were byte-identical because the REST surface doesn't change).
  TC-I-05-SRT-packet-loss.sh committed (OBSERVATION framing,
  license-gate SKIP 77 keyed to OUR streamid after V3 must-fix; ran
  once → honest SKIP w/ evidence). Runs for real the moment the
  operator's license lands. DG-18 variant note in AMS-INTEGRATION
  (RTMP TCP-masking / SRT post-ARQ); blocked-scenario list EMPTY→
  [SRT ingest].
- **★ Docs honesty (W4):** known-limitations +LIM-19 (Kafka consumer
  NEVER live-validated, AV-15 — disclosure-critical for the LIM-01
  workaround path), +LIM-20 (plaintext-only), +LIM-21 (at-least-once +
  first-start FirstOffset replay), +LIM-22 (first-viewer z-spike
  intentional, Enterprise-only); LIM-01/04 stale topic name fixed
  (ams-instance-stats→ams-server-events, code-derived caveat). All
  citations independently re-verified (V4 CONFIRMED_OK).
- **★ NEW OPERATOR DIRECTIVE (2026-07-14): ROADMAP §2.19 — full UI/UX
  refactor via uipro** ("UI/UX Pro Max" skill; CLI v2.11.0 global, NOT
  yet `uipro init`-ed in-repo). Ruling: uipro = method, brandkit
  tokens stay authoritative (D-071) unless operator overrules
  (confirmation asked in operator-expected). S30 primary = §2.19
  scoping WO. Ledger: §2.17.4 stamped DONE-S26 (3rd drift find).
- **Incidents (all recorded):** workflow died on the account spend
  limit mid-batch (4 authors) → operator raised it → resumed with
  partials adopted+gated (dead-workflow rule held); ORCH false-green
  near-miss during a mutation re-proof (`cp -a` rc≠0 on root-owned CH
  debris short-circuited `&&` — tested the unmutated copy; caught by
  RED-expectation mismatch).
- **Gates:** 24/24 `-race` 0 FAIL; coverage **76.0** (floor 70.2); web
  407/407 + lint + build; regen idempotent ×3; no new skips. CI
  promotions skip carry ×18. 12 agents (4 scouts + 4 authors + 4
  verifiers), 0 errors post-resume.
- **S30 carries:** §2.19 uipro scoping WO [primary]; SRT live run +
  Enterprise re-validation the moment the license lands; browser-accept
  trial banner (realams :18090, operator-assisted); D-V2-1 (operator
  decision); marketplace upload prep (operator-gated); PDF disposition
  (operator answer).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-30.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST — six items + PDF disposition + uipro/brandkit
confirmation; CI promotions if run date ≥07-23 [csp-e2e candidate,
web-e2e ~07-25] else skip carry ×19; AMS re-sweep at open WITHOUT any
PULSE_TOKEN prefix, observe-only — a non-null AMS diff = the operator's
license landed → run TC-I-05-SRT + Enterprise re-validation).
**PR-first, ≤2 pushes.** Check `docs/operator-expected.md` FIRST.

---

## ▶ prior session context (S28, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-29.md`)

**Session 2026-07-13 result: D-090 — S28 DONE (★ operator-intake gate +
marketplace tail: all 5 operator items re-verified still-open; DG-15 kafka
doc + AMS-INTEGRATION 4-tier de-stale + 3 listing PNGs + §2.17.2/.3 done +
realams on v0.4.0).**
- **★ OPERATOR INTAKE (mission (a)): ALL FIVE items OPEN, re-verified
  live** — 7th byte-identical sweep (license NOT applied); GHCR anonymous
  pull 401 (still private); no key/review/contact signals. v0.4.0 release
  confirm-only check PASSED (run success; Release page live 16:04Z;
  signed multi-arch image on ghcr). **NEW operator item 6: Pro
  MaxNodes=10 vs PRD §7.11 "1–2" — pricing decision, enforcement already
  built (server.go:1632/license.go:118); listing-draft stays
  NEEDS-RECONCILE until ruled.**
- **★ Docs (W1/W2, adversarially verified):** NEW `docs/kafka-integration.md`
  (DG-15; code-authoritative topic `ams-server-events` — corrects the
  assessment docs' `ams-instance-stats`; AV-15-BLOCKED admonition;
  plaintext-only; **first-start FirstOffset history-replay** — 2 V2
  catches fixed vs source incl. healthz lag>10000 degradation);
  AMS-INTEGRATION.md 4-tier remediation (~30 fixes: BUG-002-era claims,
  §4.4 falsely-missing Caddy route, webhook port :8091→:8092, B6/A2/A7
  shipped-status, ~19 line cites, DG-05 §3.7 stub). DG-05+DG-15 marked
  AUTHORED.
- **★ Marketplace assets (W3):** `qa/marketplace/render-screenshots.mjs`
  — hermetic brandkit render (render-COPY font patch; **brandkit dc.html
  Google-Fonts-CDN violation filed for designer**, brandkit untouched);
  SS1/SS2/SS4 @1282×802 verified IBM-Plex-true; SS3/SS5/SS6
  operator-manual (no source screens); PNGs gitignored.
- **★ Code honesty (W4, RED-proof re-derived independently):**
  `alert.SupportedAnomalyMetrics()` canonical accessor; parity +
  validator tests fail-fast on set drift (§2.17.2); **deliberate
  contract CR: "down" dropped from NodeHealth/FleetNode status enums**
  (§2.17.3 Option B — structurally unreachable since D-087 eviction;
  node_down ALERT untouched; FleetPage dead "Down" tile removed).
- **Rulings/ledger (W5):** §2.17.1 RULED KEEP+documented (first-viewer
  z-spike is a real signal; anomaly guide §new); **ROADMAP §2.5 found
  ALREADY FIXED since S10/D-068, stamped** (2nd ledger-drift find).
- **Gates:** 24/24 `-race` 0 FAIL; coverage **76.1** (floor 70.2); web
  388/388 + lint + build; regen idempotent; contracts valid; realams
  rebuilt on `167f48d` healthy 10s (fresh token — orphan gotcha cleared).
  CI promotions skip carry ×17. 14 agents, 0 errors.
- **S29 carries:** F10 tail [M] (probe-stats UI + RTMP AMF0), D-V2-1
  unsigned-webhook (operator), SRT-loss validation [test-only vs live
  AMS], browser-accept trial banner (realams :18090 now runs it),
  marketplace upload prep the moment operator items land.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-29.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST — now SIX items incl. the NEW MaxNodes pricing
ruling; CI promotions if run date ≥07-23 [csp-e2e candidate, web-e2e
~07-25] else skip carry ×18; AMS re-sweep at open, observe-only — a
non-null diff = the operator's license landed, expected signal not
incident). **PR-first, ≤2 pushes.** Check `docs/operator-expected.md`
FIRST (6 items).

---

## ▶ prior session context (S27, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-28.md`)

**Session 2026-07-13 result: D-089 — S27 DONE (★ OPERATOR MARKETPLACE
DIRECTIVE executed end-to-end: prod rollout D-082..D-088 LIVE +
trial-license lifecycle live-proven + one-command install + marketplace
docs pack + v0.4.0).**
- **★ OPERATOR DIRECTIVE (the S27 prompt): "rollout quick … marketplace
  asap … installation easy … trial license key … ams license today."**
  Interpretation rulings in D-089; ROADMAP-V2 **§2.18** is the new
  top-priority backlog section; the planned F10/§2.17/§2.5 batch DEFERRED.
- **★ PROD ROLLOUT EXECUTED** (the standing offer, operator-triggered):
  `v0.3.0-34-g58a9c84`, runbook path (backup + `pre-d089` rollback tag +
  stamped build + smoke green); CH 0009+0010 applied; boot self-proofs:
  zero-mean sweep count=3 + first prod VoD billing event. Webhook still
  fail-closed (signed 200 / unsigned 401).
- **★ Trial lifecycle (RULE-1, NO contract CR):** lazy expiry in every
  Manager reader — mid-run lapse ⇒ free entitlements + valid=false +
  expiresAt RETAINED (three states distinguishable in the existing
  LicenseInfo shape); boot-expired-key same honest state; injected clock;
  once-only warn. **LIVE-PROVEN: 3-min pro key on a running server
  degraded without restart; /analytics/audience 200→403.** V1: 7/7
  mutations RED in pristine copies. licensegen grew -expires-minutes.
- **★ One-command install (RULE-2):** migrations BAKED into the image
  (`/usr/share/pulse/migrations` + ENV; config.go:210 honors it) + NEW
  `deploy/quickstart/` (compose pinned `\${PULSE_IMAGE:-ghcr.io/aytekxr/
  ams-pulse:0.4.0}`, 6-var .env.example incl. the vendor PUBKEY,
  install.sh curl|bash-able). **V2 live clean-install vs the real AMS:
  healthy ~60s, baked-path migrations, token printed, re-run honest.**
  ⚠ Customers can't pull until the operator flips GHCR public (item 5).
- **★ Web trial surface (RULE-3):** LicenseContext + TrialBanner
  (amber ≤14d dismissable / red expired non-dismissable), tier badge
  revived; 388 vitest (was 366), 66.83/61.95/56.12 vs 59/54/45.
- **★ Marketplace pack (RULE-4):** NEW docs/compatibility.md +
  docs/known-limitations.md (18) + docs/marketplace/ (DRAFT-INTERNAL);
  checklist rows 16/17→PASS, 4/12 refreshed; scores recounted
  **66.7 strict / 84.5 weighted** (verifier re-derived independently).
  V3 PARTIAL → 4 must-fix (incl. a doc claiming the REMOVED Speed
  fallback exists) all remediated same-session.
- **Gates:** 24/24 `-race` 0 FAIL; coverage **76.0 → 76.1** (floor 70.2);
  gofmt/vet/qa clean; contracts byte-untouched; web green; **v0.4.0
  tagged at close (LOAD-BEARING — the quickstart pins its image).**
- **Operator queue (operator-expected.md ⚡, 5 items):** AMS license
  (promised, NOT landed by close — sweep s27open = 6th byte-identical
  null delta), official trial-key mint (vault privkey), final-assessment
  review (now gates upload), Ant Media marketplace contact, **GHCR
  public flip (NEW, critical-path)**. CI promotions skip carry ×16.
- **S28 carries:** AMS-INTEGRATION.md §4.5 stale (recording_gb claim,
  BUG-002-era), kafka-integration.md (DG-15), realams fresh rebuild
  (orphaned token — down -v sanctioned), listing PNG exports, Pro
  MaxNodes=10 vs PRD 1–2 reconcile, deferred F10 tail / §2.17 / §2.5.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-28.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST — AMS license landed? trial key minted? GHCR
flipped? assessment reviewed? — then the highest-leverage batch; CI
promotions if run date ≥07-23 [csp-e2e candidate, web-e2e ~07-25] else
skip carry ×17; AMS re-sweep at open, observe-only — realams token still
orphaned unless rebuilt). **PR-first, ≤2 pushes.** Check
`docs/operator-expected.md` FIRST (5 items incl. the NEW GHCR flip).

---

## ▶ prior session context (S26, superseded by the above — original START HERE follows)

## (superseded) ▶ START HERE (next session — execute `sessions/SESSION-27.md`)

**Session 2026-07-13 result: D-088 — S26 DONE (★ early-warning polish batch:
node-degraded predicate unified across alert+display; standalone zero-mean
baseline poison fixed cause-and-symptom, live-validated; BUG-001 deleted —
0 open bugs).**
- **★ WO-A1 (FleetNodes degraded display): worse than filed, fixed
  structurally** — THREE drifted copies of one predicate (wave2 alert had
  CPUPCT>90||MemPCT>90||ConsecAPIErrors>=3; FleetNodes checked ONLY CPU;
  LiveOverview missed the ConsecAPIErrors arm) ⇒ a node firing the rung-2
  node_degraded ALERT showed "up" on the Fleet page. Now ONE predicate
  `domain.LiveNodeStats.Degraded()` used by all three (drift structurally
  impossible); wave2_d087_test.go untouched+green. No contract CR (enum
  already [up,degraded,down]); no web change (badge already renders).
- **★ WO-A2/A3 (zero-mean baseline poison): cause AND symptom fixed** —
  live census had cpu/mem/disk_pct mean=0 stddev=0 baselines at realams
  n=733→797, prod n=8813 (first real report ⇒ z vs 1e-9 ⇒ instant false
  alarm). Cause: presence flags (CPUPCTReported/... json:"-", set in
  aggregator ok-blocks; value==0 heuristic RULED OUT — disk 0% is a valid
  cluster reading, pinned by the M7 anti-heuristic mutation) guard all 3
  eval sites. Symptom: `DeleteZeroMeanNodeBaselines` boot sweep
  (Detector.Run after WarmHysteresis, optional BaselineSweeper interface —
  V2 verified prod wiring satisfies it). **LIVE: rebuilt realams WITHOUT
  down -v so the poison survived into boot → log `purged zero-mean
  baselines on startup count=3` → census 3→0 while api_latency n kept
  growing (801→803) and viewers/bitrate rows survived — guard holds vs the
  real AMS.** viewers zero-mean = DIFFERENT class (real measurement) →
  §2.17 product ruling, S27 candidate.
- **Stretch:** BUG-001 dead code DELETED (**0 open bugs**; TC-V-09
  inverted to pin absence, PASS 3/3); §2.4 dependabot policy found ALREADY
  DELIVERED (S9) — ledger corrected; ROADMAP-V2 **§2.17** seeded (4
  follow-ups; PG sweep coverage addressed same-session).
- **Verify:** V1 CONFIRMED_OK **12/12 mutations RED in pristine copies**;
  V2 CONFIRMED_OK (prod sweep wiring ACTIVE, JSON shape unchanged, rebind
  PG-safe); V3 PARTIAL → all 3 must-fix (doc staleness) remediated
  same-session + PG parity test added (explicit PASS vs postgres:16).
- **Gates:** -race 24/24 0 FAIL (api SKIP census 0; 3 env-gated infra
  skips byte-unchanged since 2d311d9); coverage **75.9 → 76.0** (floor
  70.2); gofmt/vet clean; full `-tags integration` green vs CH 24.8 +
  postgres:16 (CI-faithful); contracts/ + web/ byte-untouched.
- **AMS post-expiry (s26open): byte-identical 5th null delta; still no
  antmedia restart.** CI promotions skip carry ×15 (07-13 < 07-23).
  Prod untouched; **a rollout now carries D-082..D-088** (BUG-001..011
  all fixed + recording billing + anomaly history + early warning +
  degraded display + zero-mean guard). **⚠ S27 gotcha:** realams container
  logs reset by the rebuild → env.sh token extraction orphaned
  (`realams-token-log-extract` memory; SESSION-27.md §3 carries it).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-27.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST; CI promotions if run date ≥07-23 [csp-e2e candidate,
web-e2e ~07-25] else skip carry ×16; candidates: F10 tail [M] + §2.17
viewer_count ruling [S] / parity-map [XS] / status-down reachability [XS–S]
+ §2.5 O(N²) [M]; AMS re-sweep at open, observe-only — token gotcha noted).
**PR-first, ≤2 pushes.** Check `docs/operator-expected.md` FIRST
(caddy-vhost? final-assessment review? prod rollout now carries
D-082..D-088).

---

## ▶ prior session context (S25, superseded by the above)

**Session 2026-07-12/13 result: D-087 — S25 DONE (★ AMS early-warning ladder
built + live-validated; BUG-011 fixed — node_down could never fire; F9
beacon metrics honestly GATED on sparsity).**
- **★ WO-D (expanded primary): the 3-rung ladder for the ant-media#7926
  class** (AMS gradually freezes; OS metrics normal): rung 1
  `ams_api_latency_ms` poller-RTT anomaly metric (FIRST live node-scoped
  metric on standalone — AMS 3.x reports no cpu/mem; key-absent-on-failure;
  skip-when-0 at all 3 eval sites; budget 5×0.086=0.432<1.0) → rung 2 API
  error-streak ≥3 → node_degraded (~15 s; was dead on standalone) → rung 3
  **BUG-011 FIXED: EvictStaleNodes was NEVER WIRED — node_down could never
  fire in ANY deployment** (explains the S19 matrix downgrade); now
  `wireNodeEviction` at pinned 3×PollInterval. Load-bearing ruling: failure
  events never refresh LastSeenAt (in-place streak update) so rung 2 can't
  starve rung 3 — both pinned red-first. **LIVE: realams rebuilt on the S25
  tree — baseline `ams_api_latency_ms|{"node_id":"beyondkaira-ams"}|
  mean=3.177ms` vs the real AMS in ~4 min.** Contract: text-only CR
  (rule_type descriptions, stale since D-074) + gen:api regen.
- **★ WO-A (F9 rebuffer_ratio/error_rate) HONESTLY GATED:** prod
  beacon_events = 2 smoke rows (realams 0); zero-variance baselines ⇒ first
  real event = instant false alarm; hourly rollup bucket accumulates ⇒
  non-independent Welford samples (needs sub-hour windowing + real
  traffic). Gate documented (§2.14/matrix F9/assessment); scores stay
  65.2/83.0; the rebuffer_ratio exclusion pin untouched.
- **Verify:** V2+V3 CONFIRMED_OK; V1 PARTIAL → remediated same-session:
  M4 GREEN_BAD (hardcoded-0 streak emission) fixed twice over — the first
  replacement pin was ITSELF vacuous (index-0 buffer rescan) and only the
  mutation re-derivation caught it (final RED: 'consec=3, want 1'); M8: 3×
  eviction multiplier extracted + pinned. 8 mutations + 2 re-derived.
  Latent AnomalyBaselineForMetric bug = dead code, TODO(D-087)-pinned.
- **Gates:** 24/24 `-race` 0 FAIL (3 env-gated infra skips; D-028 class 0);
  coverage **75.5 → 75.9** (floor 70.2); gofmt/vet/integration green; web
  366 tests, gates met. Backlog seeded: FleetNodes status ignores
  ConsecAPIErrors (§2.16 note, XS) + pre-existing zero-mean cpu/mem/disk
  standalone baselines (no presence guard, D-074-era) — both S26+.
- **AMS post-expiry (s25open): byte-identical 4th null delta; still no
  post-lapse antmedia restart.** CI promotions skip carry ×14 (ran
  07-12/13 < 07-23). Prod untouched; **a rollout now carries D-082..D-087**
  (BUG-002..011 + recording billing + anomaly history + early warning).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-26.md` and execute it**
(★ standing directive at its top: review the backlog + REVISE the plan;
operator intake FIRST; CI promotions if run date ≥07-23 [csp-e2e candidate,
web-e2e ~07-25] else skip carry ×15; candidates: FleetNodes degraded-status
display gap [XS] + standalone zero-mean baseline guard [S] + F10 tail
[probe-stats UI + RTMP AMF0] + BUG-001 [low]; AMS re-sweep at open,
observe-only). **PR-first, ≤2 pushes.** Check `docs/operator-expected.md`
FIRST (caddy-vhost? final-assessment review? prod rollout now carries
D-082..D-087).

---

## ▶ prior session context (S24, superseded by the above)

**Session 2026-07-12 result: D-086 — S24 DONE (★ BUG-008 FULLY FIXED —
ADR-0009 flag-event store built + Accepted; conformance debt now 2, both
tenant).**
- **★ WO-A: BUG-008 Group B built end-to-end:** CH migration **0010**
  `anomaly_flag_events` + UpdateBaselines-tick write path (shared
  `detectFlagsLocked`, detected_at = tick time, inserts outside d.mu,
  at-most-once) + `WarmHysteresis` restart dedup + `QueryFlagHistory`
  (base64 keyset cursor) + `/anomalies` routes ?from/?to on RAW presence
  (400 FLAG_STORE_NOT_CONFIGURED / BAD_REQUEST; parseTimeParam never
  parseTimeRange) + `flagHistoryBridge` wiring. **Registry: 37 probes /
  2 known-violations (only BUG-009 ?tenant ×2 — needs the multi-tenancy
  data model), minProbes 35.** Contract untouched.
- **★ Bug found DURING build (ADR §6 was wrong as written):** clickhouse-go
  sends time.Time params second-precision → keyset cursor duplicated
  page-boundary rows at DateTime64(3); fixed via toUnixTimestamp64Milli
  (ADR Amendment g); the reverted form now fails as an infinite cursor loop
  (structural pin). **A1 author stalled + auto-retried mid-build** — the
  retry gated its predecessor's tree per D-082; the verify phase re-derived
  ALL missing REDs: **9/9 mutations RED + 2 re-derived** after V1/V2
  must-fix remediation (t.Skip→t.Fatal pin; same-second pagination fixture).
  V3 CONFIRMED_OK (ADR items 1–15 cited; -race ×3; blast radius zero).
- **WO-B ruling:** no P2 Makefile list (auto-discovery suffices;
  PULSE_HAS_VOD_POLL stays an explicit attestation). TC-REC-01 re-run vs
  realams: **3/3 PASS, recording_gb stable after ~3 h of poll cycles** —
  the BUG-002 seen-set holds live (no double-billing drift).
- **Gates:** 24/24 Go pkgs `-race` 0 FAIL (skip census = the 3 pre-existing
  env-gated infra tests; D-028 class 0); coverage **76.0 → 75.5** (floor
  70.2; honest dilution — ~190 new CH-store lines are integration-covered);
  gofmt/vet/contract-drift clean; full integration green (10 migrations
  idempotent). ADR-0009 **Accepted** (amendments a–h).
- **AMS post-expiry (s24open): byte-identical 3rd null delta; still no
  post-lapse antmedia restart** (StartedAt 06:52Z < lapse 12:09Z) — the
  boot-time-enforcement hypothesis stays untested; observe-only. CI
  promotions skip carry ×13 (07-12 < 07-23 — the gate opens ~07-23).
  Prod untouched; **a rollout now carries D-082..D-086.**

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-25.md` and execute it**
(★ STANDING OPERATOR DIRECTIVE at its top: review the backlog + REVISE the
plan before dispatching — carry that header into every future SESSION-NN;
operator intake FIRST; CI promotions if run date ≥07-23 else skip carry ×14;
primary = F9 beacon-QoE anomaly metrics + NEW WO-D `ams_api_latency_ms`
early-warning [operator-approved, ROADMAP §2.16, upstream ant-media#7926];
AMS re-sweep at open, observe-only). **PR-first, ≤2 pushes.** Check
`docs/operator-expected.md` FIRST (caddy-vhost? final-assessment review?
prod rollout now carries D-082..D-086 = BUG-002..010 + anomaly history).

---

## ▶ prior session context (S23, superseded by the above)

**Session 2026-07-12 result: D-085 — S23 DONE (★ BUG-002 FIXED end-to-end,
live-validated + BUG-008 ADR-0009 authored + assessment 65.2/83.0).**
- **★ BUG-002 FIXED (WO-A, the recording/billing gap — was the last FAIL row
  in the marketplace checklist):** amsclient `ListVods(Paged)` (DTO pinned by
  a VERBATIM live-AMS fixture) + `restpoller.pollVods` (every 12th tick,
  tick-0 backfill, persistent seen-set on the stable AMS `vodId` — the live
  probe at open resolved ALL 5 design-note OQs in one read-only call) +
  `mv_recording_1d` (CH 0009) + `vod_poll_state` (meta 0003, 4 copies incl.
  the Postgres embed chain). **LIVE-VALIDATED: TC-REC-01 3/3 PASS vs real
  AMS — recording_gb=0.003126, 0.02% reconciliation** with the S17 fixture
  VoD. Traps pre-empted: the poll Deduplicator would silently drop
  same-window VoD events (bypassed + regression-pinned); `streamName` is the
  FILE name (`streamId` is the stream); `duration` is ms. At-most-once
  (mark-then-emit): undercount-on-crash preferred over double-BILLING.
- **WO-B: ADR-0009 (anomaly flag-event store) authored, Proposed** — CH
  `anomaly_flag_events`, migration **0010** (0009 taken by BUG-002), write
  path in the UpdateBaselines tick, hysteresis warm-up on restart, separate
  `FlagHistoryQuerier` interface. **Build DEFERRED** (Effort L vs the
  build-only-if-Small gate) → S24 primary IF approved; the 2 `/anomalies`
  known-violations stay pinned.
- **WO-C: assessment refreshed** (S20–S22 fixes + BUG-002 landed):
  completeness **60.6/79.9 → 65.2 strict / 83.0 weighted** (recounted
  mechanically 43/12/7/3/1); marketplace "No P0 open bugs" FAIL→PASS; only
  BUG-001 (low) open; docs stay DRAFT (operator review still gates external
  use).
- **Verification:** 4 scouts + 9 build agents, 0 errors; 3 adversarial
  verifiers, 0 must-fix; 5 mutation proofs (4 RED; the uncaught Postgres
  embed-chain hole got an ORCH remediation guard test + PG-parity 0003 fix).
- **Gates:** 24/24 Go pkgs `-race`, 0 FAIL (3 SKIPs = pre-existing env-gated
  infra, byte-unchanged since 2d311d9; D-028 api-skip class = 0); coverage
  **75.9% → 76.0%** (floor 70.2); gofmt/vet/contract-drift clean (no OpenAPI
  change).
- **AMS post-expiry (s23open sweep): byte-identical AGAIN; no post-lapse
  antmedia restart yet** (StartedAt 06:52Z < lapse 12:09Z) — the boot-time
  enforcement hypothesis stays untested; observe-only. **pulse-realams now
  runs the S23 build** (stack reset `down -v` + rebuilt; loopback :18090).
  Prod untouched. CI promotions skip carry ×12 (07-12 < 07-23 — the gate
  opens within ~11 days of S23).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-24.md` and execute it**
(BUG-008 ADR-0009 build [primary, IF approved — else next ROADMAP item] +
CI promotions if ≥07-23 else skip carry ×13 + AMS re-sweep at open,
observe-only). **PR-first, ≤2 pushes.** Check `docs/operator-expected.md`
FIRST (caddy-vhost? final-assessment review? prod rollout now carries
D-082..D-085 = all BUG-002..010 fixes).

---

## ▶ prior session context (S22, superseded by the above)

**Session 2026-07-12 result: D-084 — S22 DONE (post-expiry sweep NULL delta +
conformance debt 27→4 fixed TDD + two panic fixes).**
- **★ THE EXPIRY ANSWER (WO-A): the AMS trial lapsed 12:09Z and NOTHING
  observable changed.** S22 opened 05:23Z (pre-gate) → HELD OPEN per spec (no
  4th re-gate), clock monitor fired 12:10:03Z, sweep 12:11Z. The only diff vs
  the pre-expiry baseline was the teststream being down — it crashed at
  07:10Z, **5 h BEFORE the lapse** (ffmpeg, S14 class). Restarted as a live
  probe: **AMS accepted the RTMP publish post-lapse**; re-sweep BYTE-IDENTICAL
  to baseline (null delta stated explicitly). Blocked-scenario list EMPTY.
  Standing hypothesis (untested BY DESIGN): enforcement may bite at AMS
  **process restart** — S23 re-sweeps at open, observe-only, NEVER restart
  the antmedia container to test.
- **WO-C: conformance debt 27→4 known-violations, all TDD + mutation-verified.**
  BUG-006 FIXED (keyset limit+cursor through 8 list endpoints + store layer;
  `limit<=0` preserves internal callers); BUG-007 FIXED (cursor threading +
  REAL probes, not exempts); BUG-009 PARTIAL (LiveStreams cursor + required
  stability sort; tenant ×2 → F6, no tenant data model); BUG-010 FIXED (the
  ONE contract CR: audience `format` json|csv + text/csv; regen idempotent);
  BUG-008 PARTIAL (app/stream/limit/cursor fixed handler-side; **from/to are
  architecturally unfixable without a persistent flag-event store** — S23
  designs the ADR; triage: `docs/assessment/bugs/BUG-008-triage-s22.md`).
  Registry census 29/8/49 → **35 probe / 4 KV / 47 exempt = 86**; minProbes
  8→33.
- **★ The verify net caught TWO PANICS pre-ship:** stale-cursor `items[10:2]`
  OOB in LiveStreams + `?limit=-1` → `hist[:-1]` → HTTP 500 on alerts/history.
  Both red-first, both fixed. 5/5 remediation spot-mutations RED.
- **Gates:** 24/24 Go pkgs `-race` ok, **0 FAIL / 0 SKIP**; coverage
  **74.9% → 75.9%** (floor 70.2); gofmt/vet/build clean; contract-drift clean
  except the deliberate CR; web 360/360 (63.15/61.40/54.85 vs 59/54/45).
- WO-B: no operator answers (caddy-vhost + final-assessment re-surfaced).
  WO-D (BUG-002 build) did NOT fire → **S23 primary**. CI promotions skip
  carry ×11 (07-12 < 07-23). Workflows: 16 agents, 0 errors. Prod + AMS
  read-only except the sanctioned teststream restart.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-23.md` and execute it**
(BUG-002 VoD REST-poll build [primary] + BUG-008 flag-event-store ADR +
assessment refresh + CI promotions if ≥07-23 else skip carry ×12). No clock
gate. **PR-first, ≤2 pushes.** Check `docs/operator-expected.md` FIRST
(caddy-vhost merge? final-assessment review?) + the AMS post-expiry re-sweep
at open (restart hypothesis, observe-only).

---

## ▶ prior session context (S21, superseded by the above)

**Session 2026-07-12 result: D-083 — S21 DONE (BUG-005 fixed + the parameter-conformance
class fix landed; post-expiry sweep re-gated BY OPERATOR DIRECTION).**
- **BUG-005 FIXED** (`fix(api)` `2e9d026`, TDD): `/qoe/ingest` honors `interval`
  (new `parseBucketInterval`: hour→3600, day→86400; absent/invalid→0 ⇒ the 60 s
  query-layer default is KEPT — deliberate, documented deviation from the spec
  default `day`; PRD F4 15 s visibility depends on it). Contract UNCHANGED.
- **★ THE CLASS FIX LANDED:** `server/internal/api/param_conformance_test.go`
  enumerates **all 85 declared query params** from `pulse-api.yaml` and FAILS on
  any without an explicit registry entry (probe / exempt / known-violation) — a
  declared-but-ignored param can no longer land silently. 11 live probes / 47
  honest exempts / **27 known-violations pinned**. Anti-vacuity: enumeration
  floor 85, minProbes 8, spec-load t.Fatal. Mutation-verified (fix-revert,
  registry-hole, probe-break all go RED in a pristine copy).
- **★ SWEEP YIELD — the class was 28/85, not 1:** **BUG-006** (limit+cursor dead
  on 8 list endpoints), **BUG-007** (cursor-only gaps ×2), **BUG-008**
  (`/anomalies` drops ALL six declared filters), **BUG-009** (verifier catch one
  layer DEEPER: `query.LiveOverview/LiveStreams` accept `tenant` and never use
  it; `LiveStreams` stubs `cursor` — audits must follow the value to its
  observable effect), **BUG-010** (reverse direction: audience `?format=csv`
  implemented but undeclared). All filed under `docs/assessment/bugs/`; fixing
  them is S22+ backlog — pinned, not silent.
- **Gates:** 24/24 Go pkgs `-race` ok, **0 FAIL / 0 SKIP** (D-028 asserted);
  coverage **74.8% → 74.9%** (floor 70.2); gofmt/vet/build/contract-drift clean.
  Shared test helpers now `t.Fatalf` (not Skip) on missing meta DDL.
- **Date fact + the re-gate:** S21 opened 01:30Z — 9 min after S20's merge,
  STILL pre-expiry (lapse 12:09Z). Planned to HOLD past the lapse, but the
  **operator directed (03:33Z): close and continue in a new session** → sweep
  re-gated to S22 (3rd re-gate, 1st operator-directed) **at zero cost**: the
  sweep tool is committed (`qa/realams/harness/expiry-sweep.sh`, validated —
  output byte-identical to the baseline run) and the pre-expiry diff base is on
  disk (`qa/realams/evidence/S21-sweep-preexpiry-20260712T014135Z/stable.txt`,
  baseline re-confirmed ×3: Enterprise 3.0.3, build 20260504_1443, 4 apps).
- Workflow: 8 agents, 0 errors. No concurrent-session incident this time. The
  caddy-vhost decision + final-assessment review still pending (non-blocking).
  CI promotions skip carry ×10 (07-12 < 07-23). Prod + AMS untouched.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-22.md` and execute it**
(**verify the clock ≥2026-07-12T12:10Z first** — if earlier, WAIT, do not re-gate;
then `bash qa/realams/harness/expiry-sweep.sh postexpiry` + diff vs the S21
baseline → **D-084** delta + blocked-scenario list; then operator intake; then
the conformance-debt fixes BUG-006/007/009 + BUG-010 contract CR (BUG-008 needs
a ComputeFlags redesign — assess first); BUG-002 VoD poll build if light; CI
promotions if ≥07-23 else skip carry ×11). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST (caddy-vhost merge? final-assessment review?).

---

## ▶ prior session context (S20, superseded by the above)

**Session 2026-07-12 result: D-082 — S20 DONE (both P0 code bugs fixed; sweep re-gated).**
- **BUG-004 FIXED** (`fix(api)`): `/qoe/ingest` now honors the `from`/`to`/`app`/
  `stream`/`node` params it had been declaring and silently discarding. Contract
  UNCHANGED (gen:api + diff clean). **★ Prod impact found while fixing:** the web
  Ingest page sends `from=now-15min&to=now` on every load — the REAL dashboard was
  being served all-time era-mixed buckets, not just tests. Residual carved out as
  **BUG-005** (`interval` declared-but-ignored — same class).
- **BUG-003 FIXED** (`fix(prober)`): **the filed hypothesis was WRONG** (no
  "immediate run on create" goroutine exists). Real mechanism: the 60 s refresh loop
  cancel+respawned EVERY probe's scheduler on EVERY tick even when unchanged, and
  the respawn fires immediately (prod `MaxJitterFraction`=0) → duplicate 0–1 ms
  apart every **60 s** (= the refresh period, matching the evidence). It also
  silently reset every probe's phase. Fix = skip respawn on unchanged config +
  FakeClock-drivable refresh. All 3 filed fix suggestions REJECTED as symptom-hiding.
- **★ WORKFLOW PARTIALLY DIED on the weekly subagent limit** — the BUG-003 author
  wrote code+tests then died BEFORE gating. **ORCH gated everything inline and
  re-derived the missing RED proof** in a pristine copy (pre-fix `spawnProbe` → the
  pin fails with the bug's exact signature: 5 fires where 4 are expected). A pin
  whose red was never observed is not a pin. **If a workflow dies mid-phase, never
  trust the tree it left — gate it yourself.**
- **Gates:** 24/24 Go packages `-race` ok, **0 FAIL / 0 SKIP** (api skips asserted 0,
  D-028); coverage **74.5% → 74.8%** (floor 70.2); gofmt/vet/build/contract-drift clean.
- **BUG-002 design note** landed + **corrects final-assessment §5**: the VoD-poll fix
  needs **two additive migrations**, not the "no schema change" the draft claimed.
- **Date fact:** S20 ran 22:32Z–03:0xZ, i.e. STILL PRE-EXPIRY (lapse 07-12T12:09Z) →
  **the post-expiry sweep is re-gated to S21 open** (2nd re-gate; it is finally real
  next session). Baseline re-confirmed unchanged: Enterprise 3.0.3, build
  20260504_1443, 4 apps. CI promotions skip carry ×9. Prod + AMS untouched.
- **⚠️ CONCURRENT-SESSION INCIDENT (2nd occurrence):** a foreign commit (`2d3f539`,
  operator's `~/repo/bedo` session) landed a `bedirhandemirel` Caddy vhost ON the S20
  branch. Inspected (CLEAN — no secrets), **preserved on branch `caddy-bedirhan-vhost`**,
  reset out of the S20 branch, and the on-disk `Caddyfile.prod` was **NOT reverted**
  (prod Caddy mounts it — reverting would down the site). **`origin/main` now LACKS a
  vhost that live prod HAS** → a Caddy reload from a clean main checkout drops that
  site. Operator must decide whether to merge it (operator-expected.md item 1).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-21.md` and execute it**
(post-expiry AMS sweep FIRST — the trial lapsed 2026-07-12T12:09Z and S21 is the first
session that actually runs after it; then operator-review intake, BUG-005 + the
remaining P0s, CI promotions if ≥07-23 else skip carry ×10). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` FIRST (caddy-vhost merge? final-assessment review?).

---

## ▶ prior session context (S19, superseded by the above)

**Session 2026-07-11(d) result: D-081 — S19 DONE (D-078 Phases 7+8 + Phase-6 top-3).**
- **Phase 7 LANDED: `docs/assessment/prd-validation-matrix.md`** — F1–F10
  (1 FULLY = F10 probes / 9 PARTIALLY); 66 sub-rows: 40 FULLY / 14 PARTIALLY /
  7 DIFFERENTLY / 4 MISSING / 1 NC; numeric N1–N36: 33/1/2. Every verdict
  evidence-cited; adversarially verified (a FAIL-run evidence citation and a
  missing PRD criterion row were caught & fixed — the net works).
- **Phase 8 LANDED: `docs/assessment/final-assessment.md` DRAFT** — completeness
  **60.6% strict / 79.9% weighted / 91.7% numeric**; 17-row marketplace
  checklist (5 NEEDS-OPERATOR-CONTACT, 1 FAIL = BUG-002); 13-item prioritized
  roadmap (P0: BUG-002 VoD REST poll, D-V2-1 unsigned webhook, BUG-004);
  5 open questions for Ant Media. **★ OPERATOR ACTION PRODUCED: review the
  draft before ANY external use** (operator-expected.md ⚡ TL;DR).
- **Phase-6 top-3 AUTHORED:** DG-04 + DG-11 → AMS-INTEGRATION.md (+56 lines);
  DG-07 → NEW `docs/beacon-sdk.md`. Verifiers killed a fabricated D-V2-1
  "third option", 2 stale `index.iife.js` refs (real file: `index.global.js`),
  and a missing BUG-004 caveat.
- **Date facts:** S19 ran PRE-expiry (open 18:17Z; lapse 07-12T12:09Z) — fresh
  authed baseline: Enterprise Edition 3.0.3 build 20260504_1443. **Post-expiry
  sweep moved to S20 open.** CI promotions skip carry ×8 (07-11 < 07-23).
- Docs-only session; prod + AMS untouched (read-only). Workflow: 14 agents,
  0 errors. Ledger: decisions.md **D-081**.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-20.md` and execute it**
(post-expiry sweep FIRST — the trial lapses 2026-07-12T12:09Z; then
operator-review intake for final-assessment.md + P0 bug fixes BUG-004/BUG-003 +
backlog; CI promotions if ≥07-23 else skip carry ×9). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` (final-assessment review answered?) FIRST.

---

## ▶ prior session context (S18, superseded by the above)

**Session 2026-07-11(c) result: D-080 — S18 DONE (D-078 Phases 3+4 P1 + Phase 6).**
- **P1 vs LIVE AMS: 21 PASS / 3 SKIP / 0 FAIL** (24 new scenario scripts +
  `make validate-p1`). **P0 upgraded to 25 PASS / 1 SKIP** — TC-V-02 fixed
  (detached Playwright viewer died on missing NODE_PATH, invisible under -d).
- **Pulse bugs: BUG-003** (probe scheduler near-duplicate result rows, 0–1 ms
  apart ~every 60 s) + **BUG-004** (/qoe/ingest declares from/to in OpenAPI but
  ignores them). Both in docs/assessment/bugs/ — S19's PRD matrix cites them.
- **★ ENV-LIMIT finding: this VPS's AMS accepts only ~5–7 concurrent RTMP
  streams** ("current system resources not enough") — TC-L-05/TC-S-01 skip with
  a capacity probe; stress validation needs a bigger AMS instance (operator FYI
  filed). AMS-semantics findings: hlsViewerCount = sliding request-window (~9×
  session inflation, expiry lag >90 s); RTMP/TCP masks packet loss
  (packetLostRatio is UDP/SRT/WebRTC-only); app-settings mutate = POST not PUT.
- **Phase 6 DELIVERED:** docs/assessment/documentation-gaps.md (DG-01..18 with
  S19 authoring priorities). WO-C skip carry ×7 (delta green). Prod untouched.
- Shell landmine memory extended (bash \`\${var:-{}}\` stray brace; jq without -r
  quotes booleans) — check memory shell-harness-false-green-patterns before
  writing/reviewing ANY harness bash.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-19.md` and execute it**
(post-license-expiry sweep FIRST — trial lapsed 2026-07-12T12:09Z; then D-078
Phase 7 PRD validation matrix + Phase 8 final-assessment draft + top doc-gap
authoring; CI promotions if ≥07-23 else skip carry ×8). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` for operator answers (AMS-reset confirm,
Kafka, marketplace contact) FIRST.

---

## ▶ prior session context (S17, superseded by the above)

**Session 2026-07-11(b) result: D-079 — S17 DONE (D-078 program Phases 1–2 delivered).**
- **HARNESS LANDED:** `qa/realams/` (7 helpers, 26 P0 scenario scripts, Makefile;
  `make validate-realams-p0`) — reusable, evidence-gitignored, lockout-safe.
- **P0 vs LIVE AMS: 24 PASS / 2 SKIP / 0 FAIL.** Parity headlines: publish→Pulse 4 s,
  stop→Pulse 7 s (PRD ≤10 s); bitrate ÷1000 ±10%; probes live-green (rtt/jitter/loss
  keys present); fleet honest-absent. SKIPs honest: TC-APP-02 (no blocked app exists),
  TC-V-02 (headless WebRTC playback never registered — S18 item).
- **★ False-green caught (D-028 class):** suite run 1 "passed" 17 scenarios in <4 min
  — auth.sh `exit 0` on source killed callers rc=0. Runner now requires a FRESH
  verdict.txt for PASS. Memory: shell-harness-false-green-patterns (jq `//` fires on
  false; `grep -c || echo 0` doubles the zero).
- **Live AMS drift (S17 Corrections in scenario-matrix.md — BINDING over S16 rows):**
  apps 16→4 all-open (operator asked to confirm the reset); applications/info → 405;
  HLS at flat `{id}.m3u8`; implicit RTMP broadcasts DELETED on stop (404, never
  `finished`); versionType="Enterprise Edition"; one S17-created test VoD on
  pulse-test (ground truth for the recording gap).
- Bugs: BUG-001 (BroadcastStatistics dead code) + BUG-002 (recording_gb=0,
  webhook-blocked; fix = VoD REST poll fallback) in `docs/assessment/bugs/`.
- WO-B skip carry ×6 (csp-e2e 30/30 green, candidate at 07-23; web-e2e ~07-25).
  WO-C landed (info-color vars + 21 unit pins → 360 tests; light values escalated to
  `agents/handoffs/proposals/D-079-linkbody-token-proposal.md` — operator sign-off,
  non-blocking). Coverage web 65.94/61.66/54.85. Prod UNTOUCHED; pulse-realams stack
  left running (loopback :18090).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-18.md` and execute it**
(D-078 Phases 3+4 P1 scenarios + Phase 6 documentation-gap list; FIRST a read-only
post-license-expiry AMS sweep — the trial lapsed 2026-07-12T12:09Z, operator-waived;
CI promotions if run date ≥07-23, else skip carry ×7). **PR-first, ≤2 pushes.**
Check `docs/operator-expected.md` (AMS-reset confirmation?) + scenario-matrix
⚠ S17 Corrections FIRST.

---

## ▶ prior session context (S16, superseded by the above)

**Session 2026-07-11 result: D-077 — S16 DONE (+ D-078 opened: new operator program).**
- **S16 LANDED (one PR):** AuthGate fail-open FIX (the web-e2e ×12-red root cause —
  SPA-fallback 200 on unproxied /auth/me "authenticated" the shell; JSON shape-guard +
  /auth vite proxy, TDD); brandkit phase 2 (light theme 15/15 exact tokens, density
  default/compact/wall, motion + reduced-motion, sidebar toggle, status-color sweep,
  StreamsTable 44→40); ProbesPage WebRTC columns (ice_state badge + rtt/jitter/loss,
  absent=dash, 0=valid). Gates: 339/339 vitest, coverage 65.80/61.13/54.85 (all ↑),
  Playwright-docker 15/15 (the gate caught 3 spec bugs — see D-077). Key-hygiene
  closed (backup shredded on operator say-so). WO-A skip carry ×5; csp-e2e promotion
  candidate lands EXACTLY at 07-23; web-e2e clock restarted at S16's merge (~07-25).
- **★ D-078 (NEW OPERATOR DIRECTIVE, primary track from S17):** Pulse × AMS
  **real-validation & product-fit program** — 8 phases from real test env (control AMS:
  broadcasts/viewers/failures; verify effects in Pulse; AMS-API-vs-Pulse-API parity
  checks that FAIL loudly) through PRD matrix to a marketplace-readiness report for
  the Ant Media team. Plan of record: **`docs/assessment/`** (README + capability-map +
  validation-environment + scenario-matrix + session-plan), authored at S16 close.
- **Session continuity proof:** S16's terminal died mid-workflow; a fresh session
  resumed from the persisted script + journal with zero work lost (D-077).
- Operator queue now: 👀 browser-accept of the re-branded UI (standing) + optionals
  (D-V2-1, O7, O11, workflow-scope).

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-17.md` and execute it**
(D-078 program Phases 1–2: build the real-AMS harness + P0 parity scenarios; CI
promotions if the ≥07-23 gate is open, else skip carry ×6; S16 verifier backlog).
**PR-FIRST: all work via branches + PRs; max 2 pushes/session.** Check
`docs/operator-expected.md` + `docs/assessment/session-plan.md` first.

---

## ▶ prior session context (S15, superseded by the above)

**Session 2026-07-10(d) result: D-075 — S15 DONE (pion phase-2b RTP stats).**
- **WO-B phase-2b LANDED + LIVE-EVIDENCED:** probe holds ~2s after `ice_state=connected`
  and reports `rtt_ms`/`jitter_ms`/`loss_pct` (CH **0008** `Nullable(Float32)` — 0.0 is a
  valid measurement, key-absent = not measured; pointers nil on failed/timeout/hold-expiry;
  Success never flips). Mechanism settled by spike: pion v4 `NewAPI` auto-registers default
  interceptors — plain `pc.GetStats()`. mock-ams `-webrtc-ice` sends ~2s deterministic VP8
  RTP post-DTLS (sync.Once, ctx-bounded). e2e asserts the three keys is-not-None (budgets
  unchanged). Store vertical ATOMIC per D-072, proven live vs real CH v26.6.1 incl.
  LossPct=0.0 non-nil pin. **Live vs real AMS 3.0.3: rtt_ms=0.47 jitter_ms=22.33
  loss_pct=0 in 2.2s** (pristine-copy livecheck, idle box).
- **Gate find:** alert async-delivery guard was a contention flake (109.8ms vs 100ms,
  6.5ms idle) — strengthened to discriminate (500ms fake sends ⇒ sync ≥2s vs 1s budget).
- **Verify:** CONFIRMED_OK (correctness, zero findings) + PARTIAL×2 — zero functional
  must-fix; probes.md MUST-FIX (stale "reachability-only stubs" section) + ~19 more
  findings fixed same-session (TimeoutS 4→8, atomic hold-override, OMITTED wording,
  README/ARCH/ADR staleness).
- **Dispositions:** WO-A promotions skip carry ×4 (07-10 < 07-23 — **gate OPEN by S16**);
  WO-C v0.3.0 + WO-F iOS did NOT fire (operator answers still open); WO-D brandkit-2 →
  S16 WO-B; WO-E protection re-check unchanged. Workflows: 4 scouts / 6 impl / 3
  verifiers, 0 errors. Commits `86c9497..cf1417c` + close docs.

*(historical FIRST ACTION superseded by D-076 above — the 4 switches were all answered
2026-07-11 and executed in S15b.)*

**Standing numbers (2026-07-11 post-S15b/D-076):** Go total **74.5%** (floor **70.2**);
web **62.96 / 59.04 / 52.05** (gates 59/54/45, vitest-4); sdk untouched (3.52 KB). Prod
**`pulse v0.3.0` + ENTERPRISE license, healthy** — prod is CURRENT with main for the
first time since GA; QoE/beacon + probes + data API + anomaly detector all live. Watches:
pion ICE-in-CI 120s/5s budget (D-042 — if it flakes READ THE SCHEDULER); AMS
`highResourceUsage` under load (live WebRTC checks on an idle box only); latency-guard
tests must DISCRIMINATE (D-075); PR-first mechanics + 2-push budget (D-076).

---

## ▶ prior session context (S14, superseded by the above)

**Session 2026-07-10(c) result: D-074 — S14 DONE (pion media path + OIDC phase 2 + anomaly
expansion + LimitReader).** All 8 WOs executed or explicitly gated:
- **WO-B pion phase-2a LANDED + LIVE-EVIDENCED:** pion/webrtc **v4.2.16** in BOTH modules
  (CGO=0 pre-verified at open, gates green); probeWebRTC continues past the offer into a
  pion ANSWERER (trickle ICE both ways) → new `ice_state` (connected|failed|timeout, CH
  **0007**, key-absent semantics); ICE outcome NEVER flips Success (bonus-measurement).
  Live vs real AMS 3.0.3: `ice_state=connected` in 0.2s. mock-ams `-webrtc-ice` pion
  offerer (VP8 track); e2e asserts `ice_state=='connected'` at 120s/5s.
  **★ HEADLINE FIX (live-verify pays again):** real AMS sends `notification`
  (subtrackAdded) BEFORE the offer — D-072's first-message-must-be-offer parse FAILED
  against every real AMS with a live stream (CI mock false-green). Fixed (notification-
  skip loop + AMS error `definition` surfaced), pinned by fixture-replay from the live
  capture, mock now mirrors real AMS in both modes. **Phase-2b (RTCP rtt/jitter/loss,
  CH 0008) RE-GATED to S15** per the pre-declared yield — triage in decisions.md.
- **WO-C OIDC phase-2 LANDED:** GET /auth/oidc/status {enabled} + GET /auth/me
  (name/role/auth_method via ctx cookie-vs-bearer flag); AuthGate: pulse_session cookie
  authenticates the SPA, "Sign in with SSO" button when enabled; sign-out also revokes
  the OIDC session; bearer/401 flows byte-unchanged; Playwright auth-oidc.spec.ts.
- **WO-D anomaly expansion LANDED:** +`ingest_bitrate_kbps` (stream) + `disk_pct` (node);
  all 5 whitelist copies atomic; negative tests → rebuffer_ratio; FalseAlarmRate 4-metric
  CONSERVATIVE bound documented (~0.346 < 1.0 PRD); e2e A5b (spike UP, EXIT-trap restore);
  owner ruling: `internal/anomaly` → BE-02 in manifest (D-012 precedent). Beacon QoE +
  viewer_* metrics EXCLUDED w/ reason (U3 gate / sparsity).
- **WO-F LimitReader LANDED:** `segBodyCapBytes=32<<20`, LimitReader(cap+1) at BOTH
  segment sites; over-cap ⇒ Success=true + `segment_too_large` + BitrateKbps=0.
- **WO-A skip carry ×3** (07-10 < 07-23 — the gate OPENS by S15). **WO-E v0.3.0 did NOT
  fire** (unanswered; now carries D-074 too). **WO-G** re-recorded (unchanged). **WO-H**
  gated (mobile-SDK unanswered).
- **Process:** 3 workflows (4 scouts / 7 authors incl. WO-F→WO-B serial chain / 3
  adversarial verifiers → CONFIRMED_OK + PARTIAL×2, zero functional must-fix; 11
  stale-docs findings fixed same-session). Live cross-pair (probe↔mock binary, ICE 16ms).
  Final gate caught a test **budget inversion** (harness wait == probe deadline —
  deterministic, D-042 class; wait must STRICTLY dominate). AMS refuses WebRTC sessions
  (`highResourceUsage`) while workflows saturate the box — run live WebRTC checks idle.
  `ams-teststream` was found crashed (2h), restarted. Captures dir is GITIGNORED —
  shapes pinned via in-repo fixture tests instead.

**▶ FIRST ACTION — open `agents/handoffs/sessions/SESSION-15.md` and execute it** (CI
promotions — date gate OPENS ≥07-23, pion phase-2b, conditional v0.3.0, brandkit light
theme if light, operator-gated iOS SDK). **Check `docs/operator-expected.md` answers
FIRST — 4 switches (all unanswered at S14 close): "ship v0.3.0", CodeQL yes/no,
PR-first yes/no, mobile-SDK need yes/no.** Plan of record: `ROADMAP-V2.md`.

**Standing numbers (2026-07-10 post-S14/D-074):** Go total **74.4%** (floor **70.2**;
prober 72.6, anomaly 81.6, api 76.9, domain 100); web **lines 62.96 / branches 59.04 /
functions 52.05** (gates 59/54/45, vitest-4); sdk untouched (66.06/45.79/70.42; gates
63/43/67; 3.52 KB). Prod **`pulse v0.2.0` + D-067 digests**, healthy — next rollout
(**v0.3.0, operator-gated D-V2-6**) carries D-068 + D-070 + D-072 + D-073 + **D-074**.
Dependabot queue ZERO at S14 open. Operator queue: 4 questions (v0.3.0-ship, CodeQL
~07-23, PR-first, mobile-SDK) + U3 + optionals; **browser-accept of the re-branded UI
happens AFTER v0.3.0 ships.** Watches: CH startup flake (2nd occurrence ⇒ 60→180s ×4
copies); pion ICE-in-CI budgeted ONCE at 120s/5s (D-042 — if it flakes READ THE
SCHEDULER; budget-inversion class documented in D-074); AMS `highResourceUsage` refusals
under load (run live WebRTC checks on an idle box).

---

## ▶ prior session context (2026-07-07(c) — e2e backfill, superseded by ROADMAP)

**Session 2026-07-07(c) result: `pulse-e2e-backfill` is COMPLETE (D-055 + D-056).** Two workflows
(13 + 7 agents), all verifiers green. Verify with `git log --oneline -6`:
- **D-055 `001bcbe`+`3882952`+`a3cb351`** — e2e.yml now asserts A1 alert→history (fires in ~4s), A3
  health_score 100→50 transition (new mock-ams `/control/set_bitrate`; equality assert, never unpublish),
  A2 ephemeral-Pro-license beacon→`/qoe/summary` (`qa/licensegen`, ≤120s bounded poll, real ~10s);
  Playwright skeleton `web/e2e/` (5 specs; CSP spec skipped → Caddy-fronted phase 2) + non-required
  `web-e2e` ci job. ⚠️ Plan correction that MUST survive: normalize.go:79 divides wire bitrate by 1000 —
  mock wire 2000000→health 100, 400000→50. On this VPS run Playwright via
  `mcr.microsoft.com/playwright:v1.61.1-noble` (host lacks chromium libs, no sudo).
- **D-056 `0240a29`** — the e2e's faithful repro EXPOSED two pre-existing bugs, both fixed: (1) beacon
  ingest always-401 post-D-052 (adapter used plain-SHA-256 `GetTokenByHash`; now raw-token
  `LookupIngestToken` → HMAC-aware `meta.LookupToken` + kind + NEW expiry guard, 6 TDD adapter tests);
  (2) mock-ams still served pre-D-029 un-prefixed broadcast paths → every poll 404'd (even the OLD e2e
  overview assert was silently broken; e2e only runs on PRs). ⚠️ **Prod runs the pre-D-056 image** — no live
  impact (beacon is Pro+-gated, U3 pending); ship with the next prod rollout.
Coverage 59.4% → **59.5%**; full -race suite 24 pkgs, 0 FAIL / 0 SKIP. Detail: `decisions.md` D-055/D-056.
Do NOT re-do any of this. E2E-TEST-PLAN.md phase-2 leftovers: caddy-fronted CSP/Playwright job,
delivery_failure e2e, promote web-e2e to required after ~2 weeks green.

~~FIRST ACTION: pulse-test-backfill~~ **SUPERSEDED by D-057** — test backfill is ROADMAP S2/S3
(with CORRECTED per-package numbers; the debt list that stood here was stale). B7 → S5; backup
cycle-2 watch + the D-056-carrying prod rollout → SESSION-01 (WO-5).

### Operator-only actions (surface every session)
- **U3 — activate a Pro+ Pulse license.** Until then QoE/beacon data does NOT flow in prod; rebuffer/error-rate alerts
  run off the HealthScore proxy. (The e2e plan's mock license only covers CI.)
- **U4 — branch protection + a `v*` tag** (repo-admin; also retire the stale `ams-integration` ref).
- **U5 — open `beyondkaira.com` + `pulse.beyondkaira.com`**, confirm no CSP console errors.
- **point AMS at the webhook** — configure the AMS app(s) to POST lifecycle webhooks to
  `https://beyondkaira.com/webhook/ams` with the HMAC secret from `deploy/.env`. **The Pulse side is LIVE as of
  D-054** (smoke-verified: signed → 200, bad-sig → 401); only the AMS-console configuration remains.

**Binding (unchanged, hard-won):** Go ONLY in Docker `golang:1.25`, **mount the repo ROOT** (`-v <repo>:/repo -w /repo/server
-e GOFLAGS=-buildvcs=false`) or ~90 api tests silently `t.Skip` → false green (D-028). Api integration tests need
`-tags integration` + `/tmp/clickhouse` (the unit `-race` gate skips them). **No false-green:** a "flake" that never resolves
with more waiting is a deterministic bug — read the code, don't bump the timeout (D-042); verify adversarially; reproduce CI
faithfully via `gh`. Commit by **explicit path** only, never `git add -A`. `Verify → Commit → Handoff` (§11); update THIS
file + `decisions.md` (new D-0NN) each session. AMS web login is RESOLVED (D-036). The `brier` project is DROPPED (D-046) —
`Caddyfile.prod` is now plain committable Pulse config.

---

## 0. VERIFIED CURRENT STATE (facts, not assumptions)

- **Production is LIVE on a SELF-HOSTED AMS (D-034).** `https://beyondkaira.com` (apex) + subdomains
  `https://pulse.beyondkaira.com` (app) and `https://ams.beyondkaira.com` (AMS panel) — all real Let's Encrypt
  TLS via Caddy. Backend = operator-owned `antmedia` container (AMS Enterprise 3.0.3, `--network host`,
  `http://161.97.172.146:5080`), **NOT** test.antmedia.io. `/healthz` = ok (clickhouse/collector/meta_store);
  `/api/v1/live/overview` → `total_publishers:2` on LiveApp as of 2026-07-07(b) (one is the synthetic 2 Mbps
  `ams-teststream` container — `docker rm -f ams-teststream` once real streams suffice). The mock-ams seeded demo
  is **retired**. [re-verified by authed curl post-D-054 rollout].
- **AMS web-console login RESOLVED (D-036, 2026-06-29).** The AMS console MD5-hashes the password client-side, but
  both admin accounts were REST-provisioned (D-034) with the plaintext password, so the browser's hashed submission
  never matched. Fixed by re-provisioning `aytek@` + `admin@` with `MD5(realpassword)`; both now web-login, Pulse
  (plaintext) unaffected. Brute-force lockout = **2 tries → 5-min block, per-EMAIL not IP**. AMS is the **latest
  stable** (3.0.3 == Docker Hub `latest`); trial license valid to 2026-07-12. Opened the newly-created `pulse-test`
  app's `remoteAllowedCIDR` 127.0.0.1→0.0.0.0/0 (logs clean — every new AMS app defaults to 127.0.0.1). Values in
  `oguz-testing.md`.
- **Branch state (D-058, 2026-07-08): `main` is PROTECTED** (contexts contracts/server/web/sdk/docker-build/
  helm/compose, strict, 1 review, enforce_admins=false — owner direct pushes work; keep it that way while
  sessions push to main). `ams-integration` is DELETED (local+origin). Tag **v0.1.0** exists @ `1a701d6`;
  release pipeline proven (D-058). U4 is fully resolved.
- **Go suite green / coverage 73.2%** as of 2026-07-09 (full `-race` + coverage, **repo-root mount**,
  golang:1.25, after D-052…D-065; was 47.5% on 2026-06-28). Working tree is CLEAN — everything is committed and
  pushed; CI additionally enforces a `gofmt -l` gate, a **70.2%** coverage floor (D-053, ratcheted through
  D-065 = GA achieved−3) and a stamped-version docker-build assert (D-058). **Prod runs
  `v0.1.0-50-g5d77a05` = CURRENT MAIN since 2026-07-09 (D-065 WO-A)** — honest-QoE + B7 live-verified,
  beacon public chain live (403 LICENSE_REQUIRED until U3), rollback tags `pulse-prod-pulse:pre-d064`
  (bc15d43), `:pre-d061` (1a701d6) and `:pre-d058`. **★ GA DECLARED (D-065) — tag choice = operator (O13).**
- **The prod image embeds the web UI** (multi-stage `deploy/docker/pulse.Dockerfile`: `npm ci && npm run build` →
  embedded in the Go binary), so a passing go-live build implies the web build passed.

---

## 1. PENDING USER ACTIONS (only the operator can do these — persist every session)

| # | Action | Why it's blocked / needed |
|---|---|---|
| U1 | ✅ **RESOLVED (D-034).** Self-hosted AMS on this VPS; per-app `remoteAllowedCIDR=0.0.0.0/0` so Pulse polls cleanly (200). No external allow-list dependency. | (was: 8/16 apps 403'd the VPS on test.antmedia.io). |
| U2 | ✅ **RESOLVED (D-039, 2026-06-30).** `ci` workflow is GREEN (de-flaked `TestQuery_QoeSummary_RealStartupP50`, 15s→90s poll); verified via `gh` (run 28429722100, 7/7 jobs). | — |
| U3 | **Activate a Pro+ Pulse license** on `beyondkaira.com` (`PULSE_LICENSE_KEY`, see §5). | QoE/beacon ingest (F3) is gated to Pro+ (`CheckBeaconIngest` 403 on Free). Without it `beacon_events` stays empty; QoE features/alerts can't be exercised in prod. *(This is a Pulse license — separate from the AMS license.)* |
| U4 | ✅ **RESOLVED (D-058, 2026-07-08).** Branch protection live (API 200) + v0.1.0 released (run 28911789088, cosign tlog 2110636506). NEW follow-ups: **O7** make the GHCR package public (or `gh auth refresh -s read:packages`) so pulls + `cosign verify` work; **O8** review the first dependabot PRs. | — |
| U5 | **Open `https://beyondkaira.com` AND `https://pulse.beyondkaira.com` in a browser; confirm the SPA renders with no CSP console errors on each** (Caddy serves both — apex via the catch-all, subdomain via its own block, so they can fail independently). | The agent can't run a real browser; CSP is browser-enforced. Report any violation → instant fix. |
| U6 | ✅ **DONE (2026-06-30).** `gh` is installed + authed (account `aytekXR`, ssh). The CI blind spot is gone — the agent now reads Actions directly (so it can also do U4). | — |

---

## 2. DONE (verified) vs MISSING (backlog) — no "done" without verification

**DONE — verified live or by green test:** real-AMS go-live (D-031); real-AMS wire correctness — bitrate
bps→kbps, FPS-redistribution, QoE fields, `terminated_unexpectedly`, WebRTC single-track (D-029v/D-030);
`maskDSN` password-leak fix (D-031); aggregator honors configured bitrate target (D-031); cookie-session auth +
per-app paths + multi-app keying (D-029); `golang:1.26`→`1.25` (D-032); subdomains + Caddy TLS (D-034/D-035);
AMS web-console login (D-036); `ams-integration` is now contained in `main` (branch divergence resolved).

**MISSING / NOT DONE (actionable backlog — was detailed in `PRODUCTION-READINESS.md`, deleted D-069 — see ROADMAP.md):**
- ✅ **Silently-stubbed features — DONE (D-041):** alert test-fire now delivers (real `Send` via `buildChannelFromRow`,
  contract keys, `200 {accepted,message}`, sanitized error body); 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
  mutex); standalone node card shows real identity (os/cores/java/version — AMS 3.x exposes **no** standalone cpu/mem via
  REST, a documented AMS limit, A9); WebRTC viewer QoE captured **and** surfaced as `viewer_*` on `/live/streams`.
  *(Still open: the `rebuffer_ratio`/`error_rate` alerts proxy from HealthScore, not real beacon data — needs actual
  beacon data → blocked on U3; tracked under QoE/beacon e2e in phase 4 (§4).)*
- ✅ **Webhook path — DONE (D-046 route + D-048 config/test).** Prod rollout + AMS-side webhook URL config pending.
- **Branch cleanup [P2]:** retire the stale `ams-integration` pointer; branch protection + `v*` tag (U4).
- ✅ **Reliability gaps — DONE + DEPLOYED (D-049…D-054):** alert retry + delivery_failure; backups w/ verified
  restore (sidecar live in prod); CH graceful drain; resource limits (bound, inspected); `alert_history`
  auto-prune (cap 1000).
- **Security:** ✅ B3 secrets `_FILE` + opt-in overlay (D-052); ✅ API tokens HMAC-SHA256 w/ legacy back-compat
  (D-052). Remaining [P3]: B7 per-source webhook secret (contract CR).
- **Feature completion (PRD) [P3]:** QoE/beacon e2e (needs U3); Postgres meta backend (HA); SSO/OIDC; mobile SDKs;
  native WebRTC/RTMP/DASH probes; white-label PDF logo.
- **Testing [P0 for prod-readiness]:** `query` + `store/clickhouse` unit still ~0%, no response-body contract
  tests. ✅ e2e deepened (D-055: alert→history, health transition, beacon→QoE) + Playwright skeleton +
  coverage floor (D-053). Remaining breakdown in §6.

---

## 3. IMMEDIATE NEXT STEPS (do in order — each with verification)

- **Step A — `golang:1.26`→`1.25`** ✅ DONE (D-032). Verify: `grep -rn golang:1.26 deploy/ .github/` → empty.
- **Step B — Merge `ams-integration` → `main`** ✅ EFFECTIVELY DONE (2026-06-29): `main` now contains `ams-integration`
  (`git log main..ams-integration` empty). Remaining: **delete the stale `ams-integration` branch** (local + origin
  after a final diff confirms 0 unique commits), drop vestigial `AMS_LOGIN_*` from `deploy/.env.example`, add commented
  `PULSE_AMS_APPLICATIONS=` + `PULSE_INGEST_TARGET_BITRATE_KBPS=`.
- **Step C — Caddy `/webhook/*` route** ✅ DONE (D-046 route + D-048 config + D-054 live smoke: signed POST → 200).
  §3 is now fully retired — current next steps live in ▶ START HERE above.

---

## 4. BACKLOG = WORKFLOW-DRIVEN PHASES (orchestrate EACH phase as a Workflow)

> **D-057: this phase list is superseded by `ROADMAP.md` §3 (sessions S1–S7)** — kept for history.
> Mapping: phase 2 → S2/S3, phase 4 → S5 + post-GA backlog; release/dockerization work (new) = S1;
> e2e/CI hardening = S4; docs/Helm = S6; GA gate = S7.
1. ✅ **`pulse-p1-gaps`** — DONE (D-041): alert test-fire real delivery, 3 license gates enforced (+`/qoe/ingest`, +TOCTOU
   mutex), standalone node honest identity (AMS 3.x has no standalone cpu/mem via REST), WebRTC viewer QoE surfaced as
   `viewer_*`, `PULSE_ALLOWED_WS_ORIGINS` wired. Two adversarial-verify rounds.
2. **`pulse-test-backfill`** — TDD coverage to every level + enforced gate (3 sub-workflows: Go unit, web coverage
   gate, e2e+contract). See §6/§7.
3. ✅ **`pulse-prod-harden`** — DONE + DEPLOYED (D-048…D-054): webhook path, alert retry, backups, CH drain,
   B3 secrets `_FILE`, token HMAC, `alert_history` pruning, resource limits, SecretKey fail-closed. Still open
   from the original list: Trivy/SBOM, request-ID middleware (fold into phase 2/4 as convenient).
4. **`pulse-feature-complete`** — QoE/beacon e2e (after U3), AMS version surfacing, anomaly expansion, native probes,
   white-label PDF, B7 (contract CR), SSO/OIDC, mobile SDKs, backup sidecar, Postgres backend.

---

## 4a. `pulse-p1-gaps` — ✅ EXECUTED & VERIFIED (D-041, 2026-06-30)

> **DONE.** All 4 items below were implemented TDD + closed through **two adversarial-verify rounds**. The verify rounds
> overturned several of the round-1 "green" results (false-positive tests): item 1 read internal keys not contract keys
> (`webhook_url`/`email_to`/`telegram_chat_id`) and leaked secrets in the 502 body; item 3's premise was wrong — real AMS
> 3.x `/rest/v2/system-status` has **no cpu/mem**, so it now reports honest node identity (os/cores/java/`GetVersion`)
> instead; item 2 missed the `/qoe/ingest` gate + had a TOCTOU race (now mutex-guarded); item 4 was dead data (now exposed
> as `viewer_*` on `/live/streams`). The original scouted plan is kept below for provenance. **Do not re-run this workflow.**


Scouted by a read-only fan-out (4 agents); file:line below were read, not guessed. **Treat the approach as the plan,
not verified code — each item is TDD red→green (write the failing test FIRST, watch it fail, implement, watch it pass)
and re-confirmed against the live tree during implementation.** Launch as the `pulse-p1-gaps` workflow: one
disjoint-scope author per item (scopes are non-overlapping → safe to run in parallel), then ORCH gates (full `-race`
repo-root mount, §8) + commits by explicit path, then re-confirm CI green via `gh run watch`.

1. **Alert test-fire actually delivers** · scope `server/internal/api`
   - Now: `handleTestAlertChannel` (`server.go:1234-1243`) returns 202 and **never calls `Send()`**; the ready helper
     `alert.TestFireChannel` (`alert/evaluator.go:652-680`) is unused; no `buildChannelFromRow` exists.
   - Fix: add `buildChannelFromRow(store,row)` (decrypt `ConfigEnc`, switch `row.Type` → `channels.New{Slack,Webhook,
     Telegram,PagerDuty,Email}Channel`) + call `alert.TestFireChannel` in the handler; 200 on delivery, 5xx on failure.
     Channel impls + `Send` signatures in `alert/channels/*.go`.
   - Red test (`api/wave2_test.go`): POST `/alerts/channels/{id}/test` at an `httptest` webhook sink → assert the sink
     RECEIVED a body (fails today). Verify: `go test ./internal/api/... -run TestHandleTestAlertChannel`.

2. **Enforce the 3 license gates** · scope `server/internal/api/server.go` + new `license_gates_test.go`
   - Now: `CheckDataAPI`/`CheckNodeLimit`/`CheckPrometheus` (`license.go:288/250/347`) are **defined but never called** →
     Free tier 200s on `/analytics/{audience,geo,devices}`+`/qoe/summary`, registers unlimited sources, scrapes `/metrics`.
   - Fix: `if err := s.lic.CheckX(); err != nil { writeError(403,"LICENSE_REQUIRED",…); return }` at the top of
     `handleAudienceAnalytics(908)/handleGeoAnalytics(941)/handleDeviceAnalytics(961)/handleQoeSummary(982)` [DataAPI];
     `handleCreateSource(1316)` count `ListAMSSources+1` vs `CheckNodeLimit`; `handleMetrics(672)` `CheckPrometheus`.
     Pattern: `handleReportUsage` (`reports_wave2.go:26-29`).
   - Red test (`api/license_gates_test.go`, pattern `v3b_guard_test.go`): Free-tier request that should 403 (200s today).

3. **Standalone node card (`SystemStats`)** · scope `server/internal/collector` (BE-01)
   - Now: `SystemStats()` (`amsclient/client.go:532-541`, GET `/rest/v2/system-status`) has **0 callers**; for a
     standalone AMS, `ClusterNodes()` 404→nil → 0 `node_stats` → `snap.Nodes` empty → `FleetNodes()`=`[]` → blank card.
   - Fix: in `restpoller.poll()` (`restpoller.go:123-153`), when `ClusterNodes` returns nil, call `SystemStats()` + a new
     `NormalizeSystemStats` (`normalize.go`) → emit a `node_stats` event. `aggregator.onNodeStats` + `query.FleetNodes`
     already consume it (CPU/Mem wired).
   - Red test (`restpoller/standalone_node_stats_test.go`): mock AMS 404 on `/cluster/nodes` + `{cpuUsage,…}` on
     `/system-status` → assert an `EventNodeStats` with `cpu_pct` is emitted.

4. **WebRTC viewer QoE (`EventWebRTCClientStats`)** · scope `collector/aggregator` + `domain/types.go` + `cmd/pulse`
   - Now: aggregator `OnServerEvent` switch (`aggregator.go:115-134`) has **no case** for `EventWebRTCClientStats` → every
     `webrtc_client_stats` event (`restpoller.go:185-195`, `NormalizeWebRTCStats` `normalize.go:163-190`) is dropped;
     `domain.LiveStream` (`types.go:279-299`) has no viewer-QoE fields.
   - Fix: add `ViewerRTTMS/ViewerJitterMS/ViewerLossPct` to `LiveStream` + a `case domain.EventWebRTCClientStats:
     a.onWebRTCClientStats(ev)` handler that writes rtt/jitter/loss into the stream snapshot. **`PULSE_ALLOWED_WS_ORIGINS`:**
     `api Config.AllowedWSOrigins` (`server.go:70`) is consumed but never set — add the field to `EnvConfig` (`config.go`)
     + wire in `serve.go` `apiCfg` (~295-300).
   - Red test (`aggregator/aggregator_test.go`): feed publish-start + `webrtc_client_stats` → assert snapshot has `ViewerRTTMS` etc.

Full per-item detail (current behavior, fix, red test, verify cmd) was captured by the scout — re-scout cheaply with the
same fan-out if stale. Cross-check scopes against `agents/manifest.yaml` single-writer map before launching.

---

## 5. INTEGRATION KEYS (operator provides any subset; agent wires + verifies each on staging first, then prod)

Agent stores in `deploy/.env` (gitignored), wires, and verifies **real** behavior end-to-end. **Never commit keys.**
⚠️ Wire each alongside fixing the **stub the key would otherwise hide** (alert test-fire no-op; the 3 unenforced
license gates) — TDD each.

| Capability | Provide | Unlocks |
|---|---|---|
| **Pulse license** (Pro+/Business/Ent) | `PULSE_LICENSE_KEY` (or signed file + `PULSE_LICENSE_PUBKEY`) | QoE/beacon ingest (U3), anomalies, data API, probes, reports, Prometheus, multi-tenant — today gated to Free |
| **Email alerts** | SMTP host/port/user/pass (or SES/SendGrid key) | email alert delivery |
| **Slack alerts** | Slack incoming-webhook URL | Slack alert delivery |
| **PagerDuty** | routing/integration key | PagerDuty alert delivery |
| **Telegram** | bot token + chat id | Telegram alert delivery |
| **Generic webhook** | target URL + shared secret | webhook alert delivery |
| **S3 report export** | `PULSE_S3_ACCESS_KEY_ID`/`_SECRET_ACCESS_KEY`/`_BUCKET`/`_REGION`(/`_ENDPOINT`) | CSV/PDF report storage |
| **Geo enrichment** | MaxMind license key → GeoLite2-City.mmdb (`PULSE_GEO_MMDB_PATH`) | viewer country/region |
| **Prometheus** | `PULSE_METRICS_TOKEN` (self-generate) | authed `/metrics` |

Implemented alert channels: **email, slack, pagerduty, telegram, webhook**.

---

## 6. TEST & CI HARDENING (so breakage is caught in CI) — orchestrate as workflows, TDD red→green

> ⚠️ **D-057: the per-package numbers below are the 2026-07-07 baseline and several are now WRONG**
> (license 91.5, channels 74.1, config 74.5, meta 61.9, clickhouse unit 61.8, logtail 92.1 as of the
> 2026-07-08 audit). Use **ROADMAP §1/§4** as the current table; S2/S3 own this section's work.

Baseline coverage: total **59.5%** as of 2026-07-08 (was 47.5% on 2026-06-28); ci.yml enforces a 58% floor +
gofmt gate (D-053) — ratchet the floor as coverage climbs.

**ZERO unit coverage (write tests FIRST):**
- `internal/query` **0%** — powers every dashboard chart + API read (highest blast radius). Unit-test with a mock Conn.
- ~~`internal/config` 0%~~ ✅ covered by D-052 (secrets + validation tests); keep extending failure paths.
- `internal/store/clickhouse` **0% unit** (integration covers only ~3/12 query methods) + `.../migrations` **0%**.
- `cmd/pulse` **1.2%** — serve/migrate/diag wiring.

**LOW + critical:** `internal/license` **36.9%** (billing/tier gates = revenue), `store/meta` **29.7%**,
`collector/logtail` **37.5%**, `internal/api` **52.2%**, `alert/channels` **56.8%**.
**STRONG (keep ratcheting):** collector/ingest 85, cluster 89, sessions 81, anomaly 76, amsclient 76, restpoller 72,
alert 72.

**Priority (critical-business-logic-first):**
1. `license` 37→≥85 **and ENFORCE** the 3 gates + alert test-fire real `Send()`.
2. `query` 0→≥70 (mock-Conn unit) — analytics behind every chart.
3. alert firing→delivery (`channels` 57→≥80). ✅ The alert→history e2e gap is CLOSED (D-055, exactly the
   snapshot-present-metric approach: `ingest_bitrate_floor` lt 99999 → firing history row ≤30s). Still open:
   delivery_failure e2e (webhook channel at a dead URL → history row; E2E-TEST-PLAN phase 2) + channels unit depth.
4. `config` 0→≥80 — all env vars + failure paths.
5. `store/clickhouse` + `meta` — unit + expand integration to all query methods.
6. AMS wire **fixture-replay regression** pinning D-029/D-031 (bps→kbps, FPS-redistribution, `terminated_unexpectedly`,
   WebRTC single-track).
7. **De-flake `TestDiscovery_NewNodeVisible`** (`internal/cluster/discovery_test.go:116`, observed D-041): 60ms (3×20ms)
   latency budget is too tight on a CPU-contended/2-vCPU runner (measured 68.8ms once under whole-suite `-race`; 3/3 pass
   unloaded). Loosen the budget like D-039 did — a real future CI-red risk.

**CI gaps to close (`.github/workflows`) — the "see breakage in CI" asks:**
- ✅ **Coverage gate** — DONE (D-053): floor 58, ratchet as totals climb. Per-package regression check still optional.
- ✅ **Playwright browser e2e** — SKELETON DONE (D-055): `web/e2e/` 5 specs (auth gate in-place, dashboard zero
  console errors, 500-row virtualization, 401→gate; CSP spec skipped). Phase 2: caddy-fronted CSP job, promote
  `web-e2e` to required after ~2 weeks green.
- **ADD response-body contract tests** (kin-openapi) in `internal/api`: assert real responses conform to
  `contracts/openapi/pulse-api.yaml` (CI only lints the spec today, never the responses).
- **ADD web coverage threshold** (`vitest --coverage` gate).
- ✅ **e2e.yml DEEPENED** (D-055): alert fires→history, health 100→50 transition, beacon→QoE under an ephemeral
  Pro license. Still open: delivery_failure e2e, real-AMS fixture replay.

---

## 7. TDD ENFORCEMENT (BINDING — bias toward test coverage over implementation speed)

**Every change follows red→green→refactor: write the failing test FIRST, watch it fail, implement, watch it pass.**
For each unit of work produce tests at ALL applicable levels (do not stop at "unit"):

| Level | What it asserts | Where |
|---|---|---|
| **Unit** | pure logic, table-driven, both branches | `*_test.go`, `*.test.ts(x)` |
| **Integration** | real ClickHouse/sqlite via the Go harness (`-tags integration`, `/tmp/clickhouse`) | `*_integration_test.go` |
| **Contract** | HTTP response bodies validated against `contracts/openapi/pulse-api.yaml` (kin-openapi) | `internal/api/*_contract_test.go` |
| **Functional** | a feature's user-visible behavior end-to-end through the API (publish→visible, alert→history) | `e2e.yml` steps + api tests |
| **E2E (browser)** | dashboard render, auth redirect, CSP header, large-table virtualization | `web/e2e/*.spec.ts` (Playwright — NEW) |
| **Regression** | a fixed bug stays fixed (every D-0NN fix gets a pinning test) | co-located with the fix |
| **Edge-case** | empty/zero/max/null/unicode/pagination boundaries | per package |
| **Failure-path** | timeouts, 4xx/5xx, drop-on-full, retry exhaustion, decode errors | per package |

**Coverage gate (must not regress; the three 0.0% packages must reach ≥60%):**
```
sg docker -c 'docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 golang:1.25 sh -c "go test -race -coverprofile=cover.out -covermode=atomic ./... && go tool cover -func=cover.out | grep -E \"^total|0.0%\""'
```
**Prioritize critical business logic first:** (1) license/tier enforcement, (2) alert firing + delivery, (3) ingest
health scoring, (4) AMS wire decode/normalize, (5) the query layer. Report coverage in every handoff.

---

## 8. VERIFICATION WORKFLOW (BINDING — every implementation runs ALL of these before "done")

1. **Build:** `go build ./...` (CGO_ENABLED=0) + `cd web && npm run build`.
2. **Lint:** `cd web && npm run lint`; Go `gofmt -l` (must be empty) + `go vet ./...`.
3. **Type-check:** `cd web && npm run typecheck` (or `tsc --noEmit`).
4. **Test (race):** `go test ./... -race -count=1` **repo-root mount** (D-028: server-only mount silently skips ~90 api
   tests → false green). Confirm **0 FAIL, 0 unexpected SKIP**.
5. **Coverage:** the gate command in §7; attach numbers to the handoff.
6. **Contract drift:** `cd web && npm run gen:api` then `git diff --exit-code` (generated types match spec);
   `redocly lint` + `ajv` on event schemas.
7. **Staging verify:** bring the change up on an **isolated compose project** (NOT pulse-prod) and curl the affected
   endpoints. Never verify on prod first.
8. **Deploy smoke (after a prod change):** `/healthz` ok via `--resolve`; affected endpoint returns expected real
   data; `pulse logs` shows no 401/403/decode/login errors; for migrate, DSN masked (`:xxxxx@`).
9. **Independent/adversarial re-check:** default to "refuted" until reproduced on a fresh build (D-013/017/019). A
   verify harness that silently skips == no verify (D-028).

---

## 9. WORKFLOW SUGGESTIONS (prefer workflows; break large tasks into small verifiable ones)

- **Feature:** `pulse-feature-<name>` — fan out disjoint-scope authors → TDD tests → adversarial verify → ORCH gate →
  ORCH commit by explicit path.
- **Testing:** `pulse-test-backfill` — per-package finder measures coverage, authors the missing unit/edge/failure
  tests TDD-style, re-measures; a completeness critic asks "which exported fn has no test?".
- **Deployment:** `pulse-deploy-<target>` — pre-flight (config -q + login) → isolated staging verify → prod swap →
  post-swap smoke → handoff. (Pattern: `deploy/runbooks/real-ams-go-live.md`.)
- **Monitoring:** `pulse-monitor` — periodic poll of `/healthz` + `/live/overview` + `pulse logs` for AMS wire drift /
  403 storms / decode errors; surface regressions.
- **Rollback:** `pulse-rollback` — re-point pulse to the prior image/overlay (no `-v`), restore the prior state,
  smoke-verify. (Real-AMS rollback steps: runbook §5.)
- **Verification/audit:** `pulse-<x>-audit` — adversarial finders + refute pass (pattern proven in D-029v/D-031/D-032).

---

## 10. ASSUMPTIONS TO ELIMINATE (replace each with a verified fact; bias toward verification)

| # | Assumption (currently unverified or known-false) | How to eliminate |
|---|---|---|
| A1 | ✅ Resolved (2026-06-29): `main` now **contains** `ams-integration` (`main..ams-integration` empty). | Retire the stale `ams-integration` ref + branch protection (U4). |
| A2 | ✅ **VERIFIED GREEN (2026-06-30, D-039)** — `ci` all-green (run 28429722100) after de-flaking the QoE rollup test (15s→90s); readable via `gh` (U6 ✅), no longer an assumption. | Keep green: `gh run watch` after pushes. |
| A3 | ✅ Resolved: test-fire delivers (D-041); delivery retry (D-049); alert-fires→history **e2e in CI** (D-055, fired in ~4s live). Still open: delivery_failure e2e (phase 2). | Keep green via e2e.yml. |
| A4 | "Coverage is adequate." **FALSE** — 3 pkgs 0%, no gate. | `pulse-test-backfill` + coverage gate (§7). |
| A5 | "The 0.0% pkgs are covered by integration tests." Partially — only ~3 of ~12 query methods. | Add unit tests with a mock Conn (§6). |
| A6 | "QoE/beacon works in prod." **CI-VERIFIED under a mock Pro license** (D-055 beacon→rollup→qoe/summary e2e) and the always-401 bug it exposed is FIXED (D-056) — but prod still runs the pre-D-056 image AND has no license. | U3 + next prod rollout (carries D-056), then a live beacon smoke. |
| A7 | "The SPA renders / CSP is correct." **HALF-VERIFIED**: render/zero-console-errors/virtualization/auth now asserted by Playwright (D-055, route-mocked). CSP still unverified (Caddy-served; not reachable from `vite preview`). | U5 manual check + caddy-fronted Playwright CSP job (phase 2). |
| A8 | "Response bodies match the OpenAPI contract." **UNVERIFIED** — only spec-linting. | Response-body contract tests (kin-openapi). |
| A9 | "The real-AMS wire format is fully characterized." Partial — fixtures from one capture. | Watch pulse logs for decode errors; add a fixture-replay contract test; re-capture periodically. |
| A10 | "The teststream represents production load." **FALSE** — 1 low-bitrate publisher, 0 viewers. | Load/perf test (many streams/apps/viewers); VD-04 render-time at scale. |
| A11 | ✅ **RETIRED (D-059):** `TestIntegration_Migrations_IdempotentRun` applies all 4 migrations twice — second `Run` is a nil-error no-op, `schema_migrations` count unchanged. In CI on every push. | — |
| A12 | "ClickHouse shutdown loses no events." **FALSE** — 100ms sleep, not drain. | Drain-on-close + a no-loss test. |
| A13 | ✅ Moot (D-034): self-hosted AMS; `remoteAllowedCIDR=0.0.0.0/0` lets Pulse poll all apps (200). New apps default to 127.0.0.1 — open them. | — |

---

## 11. BINDING FLOWS — every workflow MUST end with these (user directive)

- **Verify** — independent/adversarial re-check of *every* claim against a running stack or fresh build; default to
  "refuted" until reproduced; **repo-root mount** or api tests silently skip (D-028). QA alone is not authoritative
  (D-013/017/019).
- **Commit** — by **EXPLICIT path**, per scope; never `git add -A/-u/.` (parallel agents share the tree — D-008/D-011).
  In a workflow, agents AUTHOR only; ORCH commits centrally (avoids `.git/index.lock` races). Message
  `<scope> D-0NN: <summary>` + evidence. Push when the user directs.
- **Handoff** — update **THIS `RESUME-PROMPT.md`** + `decisions.md` (new D-0NN) every session, then commit + push.

## 12. OPERATING PROTOCOL (binding — learned the hard way)

- **Orchestrate with the Workflow tool.** One phase = one Workflow: ORCH writes the plan + pre-approved CRs to
  `decisions.md`, fans out to disjoint-scope agents, then **independently gates**. Background work is harness-tracked —
  you're re-invoked on completion; don't poll-spin.
- **CodeGraph (operator-installed 2026-07-09, D-061).** Local index `.codegraph/` + CLI `~/.local/bin/codegraph`.
  Scouts/authors query the graph BEFORE grep/file sweeps: `codegraph explore "<question>"`,
  `codegraph node <sym>`, `codegraph callers <sym>` (blast radius). Put this in every agent work order
  (subagents use the CLI via Bash). **Closing protocol: `codegraph sync` after the last commit** (+
  `codegraph status` to confirm; stale lock → `codegraph unlock`).
- **Local compose stacks NEVER run from the real repo** — compose auto-loads `deploy/.env` (prod secrets) from
  the `-f` dir. Use a pristine working-tree copy:
  `git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -C <scratch> -xf -` + unique `-p` name (D-061).
- **Anti-stall (D-016):** NEVER run `pulse serve`/`clickhouse server` in the foreground inside an agent. Use
  `docker compose up -d` (detached) + health polling; CH unit work via the integration harness. `timeout` on builds,
  `-timeout` on `go test`, vitest `run` not watch, `curl -m`. Long local repros: Bash `run_in_background: true`.
- **Single-writer scope map** in `agents/manifest.yaml`. **Contracts frozen (D-004)** — changes only via an
  ORCH-approved CR applied by INT-01 (OpenAPI + event schemas + migrations).
- **⚠️ Workflow/fork agents have Write+commit access** — a reviewer fork once auto-committed during a concurrent ORCH
  edit (D-030 process note). Scope reviewer agents read-only when ORCH is editing the same files.
- **⚠️ Subagents NEVER revert shared-tree files (D-063):** no `git restore` / `git checkout --` /
  `git stash` inside workflow agents — concurrent agents' UNCOMMITTED work shares the tree, and a
  verifier reading `git status` cannot tell foreign work from scope violations. Violations are
  REPORTED; ORCH decides and reverts. ORCH also commits early per scope to shrink the window.
  (A wo6 fixer once destroyed two files of verified work; recovered only via transcript-replay.)

## 13. HARD RULES (CLAUDE.md / ARCHITECTURE §3)

- AMS wire formats ONLY in `server/pkg/amsclient` + `server/internal/collector`; metrics in ClickHouse, config in the
  meta store, never crossed; web UI consumes ONLY generated public-API types; beacon ingest is hostile input.
- `CGO_ENABLED=0` for the shipping build (pure-Go sqlite); single binary `pulse serve|migrate|diag`; React 19 + RR7 +
  Vite + TS strict; recharts; no external fonts/CDNs. `go test -race` needs `CGO_ENABLED=1` + gcc.
- **4 tiers** (free/pro/**business**/enterprise) in the contract enum + `internal/license/license.go` (D-014).
- Deploy fixes live in `deploy/`. Base `docker-compose.yml` stays clean (`expose:`, no host ports); exposure in
  overrides. Prod stack = `base + hardened + prod-tls + real-ams + backup` (5 overlays since D-054 — see §14).

## 14. ENVIRONMENT (VPS)

- **Ubuntu 24.04 VPS `161.97.172.146`**, Docker 29 + Compose v5. **`go` is NOT on PATH** — run Go only in Docker
  (`golang:1.25`). node 20 + npm 10 on PATH. **`gh` IS installed + authed as owner `aytekXR`** (U6, 2026-06-30 —
  the old "`gh` NOT installed" note was stale, corrected D-057).
- **⚠️ For `go test` mount the REPO ROOT** (`-v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e
  GOFLAGS=-buildvcs=false`): a `server/`-only mount makes `metaDDLPath` escape the mount → `t.Skip` →
  skip-counts-as-pass false green (~90 api tests). Confirm **0 SKIP** for api.
- **Docker:** user `aytek` is in `docker` group but stale in non-login shells → prefix `sg docker -c "…"`. `sudo` needs
  a password → ask the user via the `! <cmd>` prompt for privileged ops. For host-root debugging without sudo, run a
  privileged container in the host netns (e.g. `docker run --rm --net=host --cap-add=NET_RAW corfr/tcpdump …`, D-036).
- **Real-AMS prod ops** (run from repo root): `DC="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml
  -f deploy/docker-compose.backup.yml --env-file deploy/.env"` (backup overlay is part of the standing combo
  since D-054 — omitting it on `up -d` would REMOVE the backup sidecar). Status: `sg docker -c "docker compose $DC ps"`. Admin token: in `oguz-testing.md`
  (gitignored) — persisted in the `pulse-prod_pulse-data` volume; **never `down -v` that volume.** TLS check: always
  `--resolve beyondkaira.com:443:161.97.172.146` (VPS DNS is stale). Rollback: runbook §5.
- `deploy/.env`, `*.db*`, `oguz-testing.md`, `web/pulse_secret.key` are gitignored — never commit.
- ~~brier Caddyfile warning~~ RETIRED (D-062 verified): D-046 removed the brier block + `.bak-brier`
  file; `deploy/config/Caddyfile.prod` is clean, tracked, and uses `{$AMS_UPSTREAM}` since D-062.
- ⚠️ **Concurrent-session hazard (learned D-062):** the operator may run a second Claude session in
  this repo. If HEAD moves or the tree dirties mid-session with work you didn't do, STOP and inspect
  before committing/pushing — a foreign unpushed commit once carried a hardcoded live secret (O11).
