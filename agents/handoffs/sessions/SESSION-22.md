# SESSION-22 — post-expiry sweep (⚠️ OPEN AFTER 2026-07-12T12:10Z) + conformance-debt fixes (ROADMAP-V2 S22, planned at S21 close)

> Written by SESSION-21 close (D-083, 2026-07-12). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §3 S22 + `RESUME-PROMPT.md` §7/§8/§12 AND
> `docs/assessment/bugs/BUG-006..010` (the S21 conformance-sweep yield) before
> dispatching.

## Mission

Exit = (a) the **post-expiry AMS delta recorded (D-084)** — the operator chose
(D-083) to run it in this fresh session instead of holding S21 open; everything
is staged, just run + diff; (b) operator intake (caddy-vhost decision +
final-assessment review) applied or re-surfaced; (c) the S21-pinned
conformance debt fixed TDD: BUG-006 + BUG-007 + BUG-009 (+ BUG-010 contract CR);
each fixed param's registry entry flips known-violation → probe; (d) CI
promotions if run date ≥2026-07-23 (else skip carry ×11); (e) standing
re-checks. PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **THE CLOCK.** This session must run AFTER 2026-07-12T12:10Z (the AMS trial
   lapsed 12:09Z). `date -u` FIRST. If earlier: **WAIT — do not re-gate a 4th
   time** (S19, S20, S21 all opened pre-expiry; S21's re-gate was
   operator-directed and zero-cost, a 4th would not be).
2. **`docs/operator-expected.md` — two live decisions** (caddy-vhost merge:
   branch `caddy-bedirhan-vhost` ready, main is BEHIND live prod Caddy until it
   lands; final-assessment DRAFT review). If answered → act; else re-surface.
   Do NOT revert `deploy/config/Caddyfile.prod` on disk (prod mounts it); the
   untracked `.bak` is the operator's.
3. **Concurrent-session hazard (D-062, recurrent).** If HEAD moved or the tree
   is dirty with work you did not do: inspect → preserve on a branch → reset —
   never revert working files, never absorb into your PR.

## WO-A [S, FIRST] — post-expiry sweep → D-084

```
bash qa/realams/harness/expiry-sweep.sh postexpiry
diff qa/realams/evidence/S21-sweep-preexpiry-20260712T014135Z/stable.txt \
     qa/realams/evidence/S21-sweep-postexpiry-*/stable.txt
```

Read-only; lockout-safe auth (cookie reuse, ONE login attempt max — `admin@` is
prod's polling account, 2 fails = 5-min email-keyed lock). The tool is validated
(S21: output byte-identical to the baseline). Record in **D-084**: the diff, the
interpretation (what lapsed → which enterprise features 403/shrink), the
blocked-scenario list from `qa/realams/scenarios/`, and whether prod's own
polling still works (`pulse-prod.poll-errlines-15m` line + healthz). **A null
delta is a real result — say so explicitly.** If versionType flips or apps
vanish, update the validation docs where reality changed.

## WO-C [M, PRIMARY] — conformance-debt fixes (TDD, registry flips prove each fix)

The S21 gate (`server/internal/api/param_conformance_test.go`) pins 27
known-violations. Fix order (each: red→green, then flip its registry entry
known-violation → probe — the flip IS the proof):

1. **BUG-006** (16 entries): thread `limit`+`cursor` through the 8 list
   handlers + `meta.Store` methods. Store-layer signature changes — keep them
   additive-compatible; check every store-method caller.
2. **BUG-007** (2 entries): cursor threading for `alerts/history` +
   `probes/{id}/results` (limit already works there).
3. **BUG-009** (3 entries): `tenant` filter + cursor decode inside
   `query.LiveOverview`/`LiveStreams` (tenant→stream assignment may be thin —
   if there is genuinely no tenant data model on live snapshots yet, fix
   cursor, document tenant as still-pinned with the reason, and say so).
4. **BUG-010**: CONTRACT CR (INT-01 scope, contract-first): declare `format`
   (enum [json, csv], default json) + `text/csv` response on
   `/analytics/audience`; then `npm run gen:api` and commit the generated
   diff. This is the ONE deliberate contract change; everything else keeps the
   contract byte-identical.
5. **BUG-008** (6 entries): needs `ComputeFlags` signature redesign
   (time-range/entity/pagination args). ASSESS first; if > M effort, split to
   S23 with a written triage — do not rush an API redesign.

Full §8 gates (repo-root mount `-race`, 0 FAIL / 0 SKIP asserted, coverage
≥ floor 70.2 — currently 74.9%, gofmt-on-emptiness, vet, contract-drift clean
EXCEPT the deliberate BUG-010 CR).

## WO-D [S–M, backlog-if-light] — BUG-002 VoD REST-poll build

Design note + two INT-01 migration CRs are written
(`bugs/BUG-002-design-note-vod-rest-poll.md`). Build = migrations
(`mv_recording_1d` + `vod_poll_state`) + poller + tests. Only if WO-A..C land
with room to spare.

## WO-E [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff proof;
web-e2e earliest ~07-25; else skip carry ×11.

## WO-F [XS] — standing re-checks

Protection drift, dependabot, prod health read-only, prod untouched unless a
rollout is operator-approved.

## Gates (ORCH, before any commit)

- Any Go change: FULL §8. Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- **Never trust a tree a dead workflow left** (D-082) — ORCH re-runs gates and
  re-derives missing RED proofs in a pristine copy.
- final-assessment stays DRAFT until the operator OKs it. Nothing external.
- Prod untouched by default; S20+S21 fixes reach prod only with an
  operator-approved rollout.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-084** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-23; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/session-plan.md`
   (bugs table) + PushNotification.
5. Write `sessions/SESSION-23.md`.
