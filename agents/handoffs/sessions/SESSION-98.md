# SESSION-98 — planned at S97 close (D-161) — §2.7 CI-PROMOTIONS (date-gate UNLOCKED 2026-07-23), then the marketplace-wait

> Written by SESSION-97 close (2026-07-22/23). Repo `/home/aytek/repo/ams-pulse` on VPS (**this host IS prod**; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-131-g6b5bd38** (unchanged — S97 was docs-only, no roll).
> S97 shipped the complete marketplace docs pack (D-161); the submission is now gated ONLY on operator externals
> (docs/marketplace/submission-package.md §Blocking items). The one date-gated autonomous item — **§2.7 — unlocks
> 2026-07-23** and is THIS session's arc.

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / Future-roadmap §A–E and take the next-highest-leverage non-gated move when one exists; wait at
low frequency otherwise. Ultracode on. No backticks in Workflow prompt prose. `gofmt` only via docker; Go tests only via
docker. This host IS prod — never restart AMS, never `docker compose down -v`, never `git checkout <path>` (D-096).
**Workflow-subagent gotcha (D-161):** subagents cannot read the session scratchpad — share context via repo paths.

## THE FIRST THING TO DO AT OPEN: the two-minute gate
1. `date +%Y-%m-%d` — expected ≥ 2026-07-23 → **Lead A (§2.7) is UNLOCKED — take it.**
1b. `command -v gradle && command -v java` (or `kotlinc`) — if PRESENT → sdk/beacon-kotlin is the NEXT arc after §2.7
   (standing GO D-154, ROADMAP §2.12), no prompt needed.
2. Check `docs/operator-expected.md` top block — the operator may have: approved the docs pack (D-081) / answered the
   support-SLA + pricing + trial boxes / run the load lane (capacity number!) / recorded the demo / answered `[FO-1]` or
   [20] / reported developer-meeting outcomes (→ close A1–A10 rows in submission-process.md and fold answers into
   compatibility.md + listing). Operator input outranks Lead A ordering only if they named a priority.

## Lead A — §2.7 CI-promotions (the arc)
In `.github/workflows/ci.yml`: drop `web-e2e`'s `continue-on-error: true`; run `actionlint` on every workflow file;
verify no other soft job remains that D-100-era notes expected to harden. Then HAND THE OPERATOR the branch-protection
required-status-checks **FULL-LIST PUT** (gh api command, full replacement list adding `e2e`/`csp-e2e`/`web-e2e`/
`docker-build`/`sdk-swift` to the existing contexts) in operator-expected.md — the PUT itself needs repo-admin (only
the operator can run it). CI-config change does NOT roll prod. Pipeline: branch `s98-d162-ci-promotions` → PR → CI →
squash-merge → close docs (D-162, ROADMAP §2.7 DONE-pending-PUT, RESUME → SESSION-99, operator-expected).
**Risk note:** dropping `continue-on-error` makes `web-e2e` a hard PR gate — if it flakes on the promotion PR itself,
fix the flake or re-scope honestly (do NOT re-soften silently).

## Lead B — operator-input-driven (only if provided at open)
Capacity number → `docs/compatibility.md` load-validation row + listing; G-27 meeting answers → close the questions in
compatibility.md/submission-process.md; `[FO-1]` ruling → build the chosen firing-orphan resolution (adversarial review
MANDATORY — live critical-alert path; must NOT touch `stream_offline`); D-081 approval → strip DRAFT-INTERNAL headers
across docs/marketplace/* + licensing-public.md in one commit.

## After the arc → SESSION-99 = the marketplace-wait
With §2.7 done, the non-gated autonomous backlog is EMPTY again. Everything remaining is operator-external (submission
sequence, load lane, meeting) or tooling-blocked (Android/iOS Phase 2) or a product call ([FO-1], [20]). SESSION-99 =
low-frequency wait; do NOT manufacture an arc; do NOT re-sweep (S89/S91/S92 ×3 + S95 delta + S96/S97 arcs). Re-arm the
loop at max interval and stop in one line.

## Environment gotchas (carried, unchanged from SESSION-97.md)
Go/docker invocations, prod-deploy 5-overlay `DC` set, 5-check smoke, do-not-commit `Caddyfile.prod`, mutation-copy
restore via `cp` — see `sessions/SESSION-97.md` §Environment gotchas (all still accurate; S97 changed no runtime).
