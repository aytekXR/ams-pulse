# SESSION-23 — BUG-002 VoD REST-poll build + BUG-008 phase-2 design (ROADMAP-V2 S23, planned at S22 close)

> Written by SESSION-22 close (D-084, 2026-07-12). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/bugs/BUG-002-design-note-vod-rest-poll.md` +
> `docs/assessment/bugs/BUG-008-triage-s22.md` before dispatching.

## Mission

Exit = (a) **BUG-002 built TDD** (the recording/billing gap — the last FAIL row
in the marketplace checklist): the two additive INT-01 migrations
(`mv_recording_1d` + `vod_poll_state`) + the VoD REST poller + tests, per the
S20 design note; (b) **BUG-008 phase-2 DESIGNED** (not necessarily built): ADR
for the persistent anomaly flag-event store that makes `/anomalies?from&to`
honest (storage backend, write path in the detector tick, dedup, retention,
interface change); if the ADR lands early and is Small to build, building is a
stretch goal; (c) operator intake applied or re-surfaced; (d) CI promotions if
run date ≥2026-07-23 (else skip carry ×12); (e) standing re-checks + AMS
post-expiry observation. PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — two live decisions** (caddy-vhost merge:
   branch `caddy-bedirhan-vhost` ready, main is BEHIND live prod Caddy until it
   lands; final-assessment DRAFT review). If answered → act; else re-surface.
   Do NOT revert `deploy/config/Caddyfile.prod` on disk (prod mounts it); the
   untracked `.bak` is the operator's.
2. **Concurrent-session hazard (D-062, recurrent).** If HEAD moved or the tree
   is dirty with work you did not do: inspect → preserve on a branch → reset —
   never revert working files, never absorb into your PR.
3. **AMS post-expiry state (D-084).** The trial lapsed 2026-07-12T12:09Z with a
   NULL observable delta (byte-identical sweep; publish still accepted). The
   standing hypothesis: enforcement may bite at AMS **process restart**
   (boot-time license check). At open, re-run
   `bash qa/realams/harness/expiry-sweep.sh s23open` + diff vs
   `S21-sweep-preexpiry-20260712T014135Z/stable.txt`; check
   `sg docker -c 'docker ps --filter name=antmedia --format "{{.Status}}"'`
   for a restart since 2026-07-12. If AMS restarted AND features now 403:
   observe + report (operator said "handled") — do NOT restart the antmedia
   container yourself, do NOT fix AMS.

## WO-A [M–L, PRIMARY] — BUG-002 VoD REST-poll build (TDD)

Design note: `docs/assessment/bugs/BUG-002-design-note-vod-rest-poll.md`
(S20-authored; corrected final-assessment §5 — TWO additive migrations, not
zero). Scope: INT-01 (migrations, contract untouched) + BE-01 collector.
1. Migrations first (contract-before-code): `mv_recording_1d` ClickHouse view +
   `vod_poll_state` meta table, additive-only, both with migration tests.
2. Poller: periodic VoD REST poll per app (AMS 3.0.3 `getVodList` per the
   design note), dedupe via `vod_poll_state`, emit recording events →
   `recording_gb` finally non-zero. FakeClock-drivable (D-082 prober
   precedent); jitter + backoff; poll errors surfaced in logtail metrics.
3. Tests: unit (poller state machine, dedupe, restart-resume) + fixture-replay
   from a real `getVodList` capture (capture live at open — one read-only REST
   call; the S17 test VoD on pulse-test is ground truth) + e2e if cheap.
4. Live-verify read-only against the real AMS (poll sees the S17 VoD;
   recording_gb computes) — do NOT enable recording on any app without
   operator say-so; the existing S17 VoD suffices.

## WO-B [M, DESIGN] — BUG-008 phase 2: anomaly flag-event persistence ADR

Triage: `docs/assessment/bugs/BUG-008-triage-s22.md` (§S23). Deliverable: ADR
in `docs/adr/` — table choice (ClickHouse `anomaly_flag_events` vs meta),
write path (detector tick persists emitted flags), dedup key, retention/prune,
`ComputeFlags`/api interface change, migration CR list, probe plan (how the
`?from`/`?to` conformance entries flip known-violation → probe). The 2
remaining `/anomalies` known-violations + 2 `?tenant` (F6) stay pinned until
built. Build only if the ADR is approved-by-plan and Small.

## WO-C [S] — assessment refresh (post S20–S22 fixes)

`docs/assessment/prd-validation-matrix.md` + `final-assessment.md` still cite
BUG-003/004/005/006/007/008/009/010 as open in several rows. Refresh ONLY the
rows those fixes change (matrix stays evidence-cited; assessment stays DRAFT —
operator review still gates external use). Update
`docs/assessment/session-plan.md` bugs table.

## WO-D [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff proof;
web-e2e earliest ~07-25; else skip carry ×12.

## WO-E [XS] — standing re-checks

Protection drift, dependabot, prod health read-only, prod untouched unless a
rollout is operator-approved. If the operator approves a rollout: it now
carries D-082 (BUG-003/004) + D-083 (BUG-005) + D-084 (BUG-006..010 partial +
2 panic fixes) — a meaningful API-correctness release.

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 SKIP asserted,
  coverage ≥ floor 70.2, gofmt-on-emptiness, vet, contract-drift clean —
  migrations are additive INT-01 scope, NOT a pulse-api.yaml change).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- **Never trust a tree a dead workflow left** (D-082) — ORCH re-runs gates and
  re-derives missing RED proofs in a pristine copy.
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-085** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-24; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/session-plan.md`
   (bugs table) + PushNotification.
5. Write `sessions/SESSION-24.md`.
