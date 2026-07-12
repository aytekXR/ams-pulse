# SESSION-26 — backlog batch (display gap + baseline guard + F10 tail candidates) + CI promotions window (ROADMAP-V2, planned at S25 close)

> Written by SESSION-25 close (D-087, 2026-07-13). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2/§3 + `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 + the final-assessment §5 roadmap
and REVISE this plan if a higher-leverage move exists. This file is a
starting point, not a contract — "we always find a spot to shine." (S25's
example: the backlog review + scouts turned a planned metric-add into
finding and fixing BUG-011, a structurally-dead node_down.) Record any
revision in the D-088 open block. Carry this header into SESSION-27.md.

## Mission

Exit = (a) the highest-leverage backlog batch built — candidates, smallest
first: **FleetNodes degraded-status display gap [XS]** (query.FleetNodes
sets status="degraded" only on CPUPCT>90, ignoring ConsecAPIErrors>=3 — a
node firing the rung-2 ALERT shows "up" on the Fleet page; §2.16 note);
**standalone zero-mean baseline guard [S]** (pre-existing D-074-era: cpu_pct/
mem_pct/disk_pct baselines form at mean=0 on standalone AMS where those are
never reported — same instant-false-alarm class the S25 presence guard
prevents for ams_api_latency_ms; add the guard + decide migration/cleanup
for existing zero-mean rows); **F10 tail [M]** (probe-stats UI surface +
RTMP AMF0 connect phase); **BUG-001 [low]** (BroadcastStatistics dead code);
(b) operator intake applied or re-surfaced; (c) CI promotions if run date
≥ 2026-07-23 (else skip carry ×15 — csp-e2e candidate opens 07-23, web-e2e
~07-25); (d) standing re-checks + AMS observation at open. PR-first,
≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — standing decisions** (caddy-vhost merge;
   final-assessment DRAFT review; optional prod rollout — now carries
   D-082..D-087: BUG-002..011 fixes + recording billing + anomaly history +
   the early-warning ladder). If answered → act; else re-surface. Do NOT
   revert `deploy/config/Caddyfile.prod` on disk.
2. **Concurrent-session hazard (D-062, recurrent).** Foreign work → inspect
   → preserve on a branch → reset; never revert working files.
3. **AMS post-expiry state.** FOUR byte-identical sweeps post-lapse; still
   no antmedia restart (StartedAt 2026-07-12T06:52Z). At open:
   `bash qa/realams/harness/expiry-sweep.sh s26open` + diff vs
   `S21-sweep-preexpiry-20260712T014135Z/stable.txt`; check
   `sg docker -c 'docker inspect antmedia --format "{{.State.StartedAt}}"'`.
   If restarted AND features 403: observe + report, never restart/fix AMS.
4. **pulse-realams runs the S25 build** (loopback :18090, rebuilt at S25
   close; `ams_api_latency_ms` baseline live). `down -v` sanctioned for
   realams ONLY, never prod.
5. **S25 merge evidence:** if the S25 PR merged post-push, append the
   PR/merge line to decisions.md D-087 (S26's PR carries it).

## WO-A [XS+S] — early-warning polish batch (if chosen)

FleetNodes status: degraded when ConsecAPIErrors>=3 OR CPUPCT>90 (align
with wave2 evalNodeUpDown; FleetNode schema uncontracted — verify no CR).
Standalone baseline guard: skip cpu_pct/mem_pct/disk_pct observations when
the node has never reported them (decide the presence signal honestly —
NormalizeSystemStats emits none on AMS 3.x standalone; distinguish
"reported 0" from "never reported" — may need an emitted-keys check in the
aggregator, NOT a value==0 heuristic for metrics where 0 is valid; write
the ruling down). Existing zero-mean rows: decide delete-on-boot vs leave
(document). TDD + mutation-verified per §8.

## WO-B [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff
proof; web-e2e earliest ~07-25; else skip carry ×15.

## WO-C [XS] — standing re-checks

Protection drift, dependabot, prod health read-only. Prod untouched unless
a rollout is operator-approved.

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 unexpected
  SKIP — 3 env-gated infra skips are characterized in D-085..D-087,
  coverage ≥ floor 70.2 (S25 actual 75.9), gofmt-on-emptiness, vet,
  contract-drift clean).
- Never trust a tree a dead/stalled agent left (D-082/D-086) — re-derive
  RED proofs in pristine worktrees. A pin that can t.Skip is not a pin
  (D-086). **A replacement pin must have ITS red re-derived too — S25's M4
  remediation was itself vacuous on the first draft (D-087).**
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-088** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-27; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-27.md` (carry the standing directive header).
