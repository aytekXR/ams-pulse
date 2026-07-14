# SESSION-31 — operator-intake gate + §2.19 Wave 0 (planned at S30 close, D-092)

> Written by SESSION-30 close (D-092, 2026-07-14). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/wave-uipro/WAVE-PLAN.md` + `ROADMAP-V2.md` §2.19/§2.18 +
> `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.19 has its wave plan now; §2.18
item 6 is the operator-gated remainder) + the final-assessment §5 roadmap and
REVISE this plan if a higher-leverage move exists. This file is a starting
point, not a contract. Record any revision in the D-093 open block. Carry
this header into SESSION-32.md.

## Mission

Exit = (a) **operator intake applied** — FIVE standing marketplace items
(operator-expected.md ⚡ item 5) PLUS: **AMS license landed?** → THE trigger
(his AMS is ingest-DEAD since the 07-13 restart): re-sweep shows delta OR a
teststream publish is suddenly ACCEPTED → record delta + restart teststream
(runbook §5 command) + run `TC-I-05-SRT-packet-loss.sh` for real +
re-validate the Enterprise surface; **matbu vhost ruling?** → execute option
(a)/(b)/(c) per answer; **G1/G2 design gaps answered?** → fold into
WAVE-PLAN conflict ledger; **uipro-vs-brandkit confirmation** → if "uipro
overrules brandkit", re-rule §2.19 BEFORE Wave 0; PDF: "drop the pdf" →
remove from tree. (b) **§2.19 Wave 0 [S] [primary]** per WAVE-PLAN §4 W0:
extract shared `TierGate` (triplicated verbatim in Reports/Anomalies/
Probes) + shared `Tabs` (inline pattern ×6 pages) + CHART_COLORS[7]
residual verify — NO design-value changes, pure extraction; full web gates
(see below). (c) CI promotions if run date ≥ 2026-07-23 (else skip carry
×20). (d) standing re-checks + AMS observation at open. PR-first, ≤2 pushes.

## S31 carries (from D-092; pick by leverage after intake)

1. **§2.19 Wave 0 [S]** — primary (Mission (b)); Wave 1 (Live+QoE [M])
   only if Wave 0 lands early and gates are green.
2. **License-landing chain [gated]** — fires from intake the moment the
   operator says "AMS license applied" (or the sweep/teststream shows it).
3. **Marketplace upload prep [gated]** — operator items 2–5.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** —
   re-surface only.
5. **Browser-accept of the trial banner [operator-assisted]** — realams
   :18090 runs v0.4.0.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` ⚡ — five standing + matbu + G1/G2 +
   uipro confirmation.** If answered → act; else re-surface.
2. **Concurrent-session hazard (D-062, 4th occurrence S29, operator
   pre-session index change S30).** Foreign work → inspect → preserve →
   never revert working files. The on-disk `Caddyfile.prod` matbu block
   stays UNCOMMITTED until the operator rules (bcrypt hash, public
   repo). Check `git status` + `.git/index` mtime at open — the S30
   pattern was the operator staging files minutes before session start.
   The session shell may lack the docker group — `sg docker -c "…"`.
3. **AMS state at open.** `bash qa/realams/harness/expiry-sweep.sh
   s31open` — NO PULSE_TOKEN prefix (S29 gotcha). Known truth since
   S30: AMS is INGEST-DEAD (all new RTMP+SRT publishes refused
   "License is suspended…" since the 22:21Z 07-13 restart; REST
   byte-identical Enterprise; teststream CANNOT return). The sweep
   diff vs `S21-sweep-preexpiry-20260712T014135Z/stable.txt` will show
   the teststream-down rows — that part is EXPECTED, not a delta. A
   REST delta OR an accepted publish ⇒ the license landed → run the
   Mission (a) chain. NEVER restart/fix AMS. VPS load note: check
   `uptime` before Playwright/gates — concurrent operator sessions
   pushed load to 20 on 07-13 (AMS's own 75% CPU guard then refuses
   publishes for the OTHER reason — don't confuse the two rejections;
   evidence pattern in D-092).
4. **Prod runs v0.3.0-34-g58a9c84 since S27.** Read-only health check
   at open; **next rollout carries D-089..D-092** (docs-only D-092
   adds nothing runtime — rollout value unchanged since S29).
5. **S30 merge evidence:** append the PR/merge line to decisions.md
   D-092 (S31's PR carries it).
6. **uipro skill presence:** `.claude/skills/ui-ux-pro-max/` is
   local-only + gitignored (license blocker, D-092). If missing,
   bootstrap per WAVE-PLAN §1.1b. NEVER commit it.

## Gates (ORCH, before any commit)

- Wave 0 (web change): `cd web && npm run gen:api && git diff
  --exit-code` (gen:api drift) + lint + build + `npx vitest run
  --coverage` (floors 59/54/45; S30 census 404 tests/30 files) +
  Playwright-docker light+dark (`dashboard-render`, `auth-gate`,
  `csp`) + `prefs.spec` density/reduced-motion + WCAG re-check on
  changed components (design-rationale §2 BINDING) + zero contract
  changes + tokens.json untouched + no new bare hex/px in changed
  files (WAVE-PLAN §2.2 checklist is BINDING and includes all of
  this — read it verbatim).
- Any Go change: FULL §8 (CI-faithful golang:1.25 docker w/
  pulse-gomod + pulse-gobuildcache volumes + safe.directory, 0 FAIL /
  0 unexpected SKIP, coverage ≥ floor 70.2, gofmt-on-emptiness, vet,
  contract-drift). Not expected this session.
- Never trust a tree a dead/stalled agent left (D-082/D-086/D-091).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- docs/marketplace/ stays DRAFT-INTERNAL (D-081 external gate).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-093** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-32; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-32.md` (carry the standing directive header).
