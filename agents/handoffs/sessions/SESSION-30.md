# SESSION-30 — operator-intake gate + §2.19 uipro scoping (planned at S29 close, D-091)

> Written by SESSION-29 close (D-091, 2026-07-14). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2.19 + §2.18 + `RESUME-PROMPT.md` before
> dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.19 is the new operator-directed
track; §2.18 item 6 is the operator-gated remainder) + the final-assessment §5
roadmap and REVISE this plan if a higher-leverage move exists. This file is a
starting point, not a contract. Record any revision in the D-092 open block.
Carry this header into SESSION-31.md.

## Mission

Exit = (a) **operator intake applied** — SIX standing items
(operator-expected.md ⚡) PLUS two new asks: AMS license landed? → the open
sweep shows a NON-null AMS diff (expected signal, not incident) → record
delta + **run `TC-I-05-SRT-packet-loss.sh` for real** + re-validate the
Enterprise surface; trial key minted? → embed in listing draft; GHCR
flipped public? → verify anonymous `docker pull` + install.sh default-image
leg; final-assessment reviewed? → execute edits / mark approved; Ant Media
contact? → fold listing requirements into docs/marketplace/; Pro MaxNodes
ruling? → one-line change + clear NEEDS-RECONCILE; **PDF disposition**
(`docs/ant-media-marketplace-opportunity-report.md.pdf`, untracked/unread)
→ commit/keep-local/fold per answer; **uipro-vs-brandkit confirmation** →
if "uipro overrules brandkit", re-rule §2.19 before scoping. (b) **§2.19
scoping WO [primary]**: `uipro init` in-repo (inspect what it installs
BEFORE committing — third-party skill content gets vendored-code review),
inventory the skill vs `brandkit/design-system/tokens.json` + the binding
WCAG table, produce the page-by-page wave plan with per-wave gates; first
wave IF the scoping stays [S] and gates allow. (c) CI promotions if run
date ≥ 2026-07-23 (else skip carry ×19). (d) standing re-checks + AMS
observation at open. PR-first, ≤2 pushes.

## S30 carries (from D-091; pick by leverage after intake)

1. **§2.19 uipro scoping WO [S–M]** — primary (see Mission (b)).
2. **SRT live validation [S, gated]** — fires automatically from intake
   the moment the license lands; scenario committed and SKIP-honest.
3. **Browser-accept of the trial banner [operator-assisted]** — realams
   :18090 runs v0.4.0; ssh tunnel walk-through if the operator is present.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** —
   re-surface only; do not build unprompted (§2.6 network-trust model).
5. **Marketplace upload prep [gated]** — the moment operator items 2–5
   land: embed trial key, remaining screenshots, flip docs/marketplace/
   out of DRAFT-INTERNAL (needs final-assessment approval, D-081 gate).

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — six items + two new asks** (see
   Mission (a)). If answered → act; else re-surface.
2. **Concurrent-session hazard (D-062, 3rd occurrence at S29).** Foreign
   work → inspect → preserve → never revert working files. The S29
   variant: the operator committed DIRECTLY onto local main (adopted at
   S29 close — check it reached origin; if origin push was blocked, the
   commit rides S29's PR instead, verify which happened via
   `git log origin/main`). The session shell may lack the docker group —
   `sg docker -c "…"`.
3. **AMS post-expiry state.** At open:
   `bash qa/realams/harness/expiry-sweep.sh s30open`
   — **NO PULSE_TOKEN prefix** (the S29 finding: any non-empty
   PULSE_TOKEN suppresses the realams token auto-extraction and
   produces a false parse-err overview line). Diff vs
   `S21-sweep-preexpiry-20260712T014135Z/stable.txt`. Known truth since
   S29: REST surface byte-identical BUT SRT ingest license-rejects
   (feature-level enforcement, no restart needed). A non-null AMS diff
   ⇒ probably the new license: record delta, run TC-I-05-SRT,
   re-validate — never restart/fix AMS.
4. **Prod runs v0.3.0-34-g58a9c84 since S27** (rollback tag pre-d089
   stands). Read-only health check at open; **next rollout carries
   D-089..D-091** (trial lifecycle + baked migrations + web banner +
   AMF0 probe depth + probe-stats UI + fleet-status CR).
5. **S29 merge evidence:** append the PR/merge line to decisions.md
   D-091 (S30's PR carries it).

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (CI-faithful: golang:1.25 docker w/ pulse-gomod
  + pulse-gobuildcache volumes + safe.directory /repo, 0 FAIL / 0
  unexpected SKIP, coverage ≥ floor 70.2 (S29 actual 76.0),
  gofmt-on-emptiness, vet, contract-drift clean). Integration suite when
  store/query/api change. Web: gen:api drift + build + LINT + vitest
  (gates 59/54/45; S29 actual 407 tests, 64.13/62.13/56.12).
- §2.19 waves additionally: Playwright-docker light+dark, WCAG table
  conformance (brandkit rationale §2 BINDING), tokens.json-only values.
- Never trust a tree a dead/stalled agent left (D-082/D-086/D-091 —
  S29 lived it: adopt+gate, re-derive RED in pristine copies; beware
  `cp -a` rc≠0 on root-owned CH debris short-circuiting `&&` chains).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- docs/marketplace/ stays DRAFT-INTERNAL until the operator approves the
  final assessment (D-081 external gate). Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-092** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-31; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-31.md` (carry the standing directive header).
