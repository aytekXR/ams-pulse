# SESSION-24 — BUG-008 flag-event store build (if approved) + backlog + CI promotions window (ROADMAP-V2, planned at S23 close)

> Written by SESSION-23 close (D-085, 2026-07-12). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/adr/0009-anomaly-flag-event-store.md` before dispatching.

## Mission

Exit = (a) **BUG-008 phase-2 built** per ADR-0009 IF the plan/operator approves
building it (the ADR is Proposed; Effort L — a full-session primary), flipping
the last 2 `/anomalies` known-violations to probes; else pick the next
ROADMAP-V2 item; (b) operator intake applied or re-surfaced; (c) CI promotions
if run date ≥ 2026-07-23 (else skip carry ×13 — the gate FINALLY opens within
~11 days); (d) standing re-checks + AMS post-expiry observation at open.
PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — standing decisions** (caddy-vhost merge;
   final-assessment DRAFT review — the assessment now says 65.2/83.0 with
   marketplace row 3 PASS, worth a re-look; optional prod rollout approval —
   a rollout now carries D-082..D-085: every BUG-002..010 fix + recording_gb
   billing). If answered → act; else re-surface. Do NOT revert
   `deploy/config/Caddyfile.prod` on disk.
2. **Concurrent-session hazard (D-062, recurrent).** HEAD moved / dirty tree
   with foreign work → inspect → preserve on a branch → reset; never revert
   working files.
3. **AMS post-expiry state (D-084/D-085).** Two sweeps post-lapse, both
   byte-identical; NO post-lapse antmedia restart has happened yet
   (StartedAt 2026-07-12T06:52Z = pre-lapse) — the boot-time-enforcement
   hypothesis is STILL untested. At open: re-run
   `bash qa/realams/harness/expiry-sweep.sh s24open` + diff vs
   `S21-sweep-preexpiry-20260712T014135Z/stable.txt`; check
   `sg docker -c 'docker inspect antmedia --format "{{.State.StartedAt}}"'`.
   If restarted AND features 403: observe + report, do NOT restart/fix AMS.
4. **pulse-realams stack now runs the S23 build** (loopback :18090, reset via
   `down -v` at S23 — fresh volumes, fresh admin token in its boot logs).
   TC-REC-01 needs `PULSE_HAS_VOD_POLL=1` against this stack.

## WO-A [L, PRIMARY if approved] — BUG-008 phase 2 per ADR-0009

ADR: `docs/adr/0009-anomaly-flag-event-store.md` (Proposed). Scope: CH
migration **0010** (`anomaly_flag_events`, TTL {retention_days}); write path
in the UpdateBaselines tick (shared detection helper, detected_at = tick
time, synchronous inserts, nil flagStore = no-op); hysteresis warm-up from
the store at start; `QueryFlagHistory` + separate `FlagHistoryQuerier`
interface + setter; handler routes on ?from/?to presence; keyset cursor;
registry: 2 known-violations → probes (recording double + non-overlapping
window differential), minProbes 33→35. Full §8 gates; contract untouched
(params already declared).

## WO-B [S] — TC-REC-01 in the routine suite + BUG-002 follow-ups

Consider adding TC-REC-01 to a P2/post-fix Makefile list (it auto-discovers
under validate-all and SKIPs without the env — decide whether the realams
stack, now on the S23 build, should run it with PULSE_HAS_VOD_POLL=1
routinely). Optional cheap follow-ups if touching the area: recording_method
field on usage response (design-note §5.3 enhancement, needs contract CR).

## WO-C [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff
proof; web-e2e earliest ~07-25; else skip carry ×13.

## WO-D [XS] — standing re-checks

Protection drift, dependabot, prod health read-only. Prod untouched unless a
rollout is operator-approved (it now carries D-082..D-085).

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 unexpected
  SKIP — the 3 env-gated infra skips (npx ×2, poppler) are pre-existing and
  characterized in D-085, coverage ≥ floor 70.2 (S23 actual 76.0), gofmt-on-
  emptiness, vet, contract-drift clean).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- Never trust a tree a dead workflow left (D-082) — re-derive RED proofs in a
  pristine copy.
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-086** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-25; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-25.md`.
