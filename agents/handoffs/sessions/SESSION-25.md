# SESSION-25 — next ROADMAP item (F9 beacon-QoE anomaly candidate) + CI promotions window (ROADMAP-V2, planned at S24 close)

> Written by SESSION-24 close (D-086, 2026-07-12). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2.14/§3 + `RESUME-PROMPT.md` §7/§8/§12
> before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (esp. the NEW §2.16) + the
final-assessment §5 roadmap and REVISE this plan if a higher-leverage move
exists. This file is a starting point, not a contract — the operator's words:
"we always find a spot to shine." S24's example: two upstream AMS issues
(#3122, #7926) turned into a demand-driven backlog item (§2.16 / WO-D below)
worth more than mechanical list-execution. Spend a few read-only minutes
looking for that kind of leverage (upstream issues, operator pain, assessment
gaps) before locking the session shape; record any revision in the D-087 open
block.

## Mission

Exit = (a) the next ROADMAP-V2 item built or explicitly re-gated — **top
candidate: F9 beacon-QoE anomaly metrics** (`rebuffer_ratio`, `error_rate`;
the §2.14 exclusion was "U3 gate / sparsity" and U3 is RESOLVED since D-076 —
prod runs an enterprise license with the beacon chain live; ASSESS FIRST:
does prod/realams have enough beacon_events data to Welford-baseline these
metrics honestly? If data is too sparse, document the honest gate and pick
the next item — candidates: BUG-001 dead-code cleanup [low], F10 tail
[RTMP AMF0 connect + probe-stats UI surface, D-075 note], 2.5 O(N²)
rebuildSnapshot if fleet growth warrants); (b) operator intake applied or
re-surfaced; (c) CI promotions if run date ≥ 2026-07-23 (else skip carry ×14
— csp-e2e candidate opens 07-23, web-e2e ~07-25); (d) standing re-checks +
AMS post-expiry observation at open. PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — standing decisions** (caddy-vhost merge;
   final-assessment DRAFT review; optional prod rollout — now carries
   D-082..D-086: all BUG-002..010 fixes + recording_gb billing + persistent
   anomaly history). If answered → act; else re-surface. Do NOT revert
   `deploy/config/Caddyfile.prod` on disk.
2. **Concurrent-session hazard (D-062, recurrent).** HEAD moved / dirty tree
   with foreign work → inspect → preserve on a branch → reset; never revert
   working files.
3. **AMS post-expiry state (D-084/D-085/D-086).** THREE byte-identical
   sweeps post-lapse; NO post-lapse antmedia restart yet (StartedAt
   2026-07-12T06:52Z). At open: `bash qa/realams/harness/expiry-sweep.sh
   s25open` + diff vs `S21-sweep-preexpiry-20260712T014135Z/stable.txt`;
   check `sg docker -c 'docker inspect antmedia --format
   "{{.State.StartedAt}}"'`. If restarted AND features 403: observe +
   report, do NOT restart/fix AMS.
4. **pulse-realams stack runs the S23 build** (loopback :18090) — it does
   NOT have S24's flag-event store. If S25 needs to live-check
   `/anomalies?from`, rebuild the stack from the merged tree first
   (`down -v` is sanctioned for realams ONLY, never prod).
5. **S24 merge evidence:** if the S24 PR merged post-push, append the
   PR/merge line to decisions.md D-086 (S25's PR carries it).

## WO-A [M, PRIMARY — assess-then-build] — F9 beacon-QoE anomaly metrics

Scout first: beacon_events volume/shape in prod CH (read-only) and realams;
the 5-copy metric whitelist (D-074 landed `ingest_bitrate_kbps`+`disk_pct` —
follow that PR's shape); LiveProvider surface for viewer-QoE aggregates.
Build = extend the anomaly whitelist to `rebuffer_ratio` + `error_rate`
(stream scope, beacon-sourced), all whitelist copies atomic, e2e A5-class
coverage, FalseAlarmRate bound re-derived (ADR-0007 budget ≤1/node-week must
still hold with 7 metrics). The S24 flag-event store persists whatever
fires — no schema change expected (verify: LowCardinality(metric) accepts
new values transparently). If beacon data is too sparse to baseline honestly
→ document the gate in ROADMAP §2.14 and fall back to the next candidate.

## WO-D [S, OPERATOR-APPROVED 2026-07-12] — AMS early-warning metrics (ROADMAP §2.16)

Demand-driven by upstream **ant-media/Ant-Media-Server#7926** (open: AMS
freezes after ~24 h; OS metrics normal → cpu/mem/disk anomaly blind by
construction; Pulse's node_down already detects the freeze — the gap is the
lead time). Build, riding WO-A's whitelist plumbing:
1. `ams_api_latency_ms` (node scope): measure poll round-trip in restpoller
   (restpoller.go — today poll errors are ONLY logged, nothing measured) →
   live snapshot → anomaly whitelist (all 5 copies atomic, D-074 pattern).
2. API error-streak → `node_degraded` (rule type already in
   evaluator.go:379's scope-match; feed it a consecutive-failure counter
   BEFORE absence eviction fires node_down).
3. Stretch only if light: probe TTFB trend anomaly (TTFB already stored).
Assessment docs: add the two demand-evidence citations (#3122 — Pulse's
/metrics already solves it natively; #7926 — detect-in-30s + early-warn
story). FalseAlarmRate budget must be re-derived with the metric count.

## WO-B [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff
proof; web-e2e earliest ~07-25; else skip carry ×14.

## WO-C [XS] — standing re-checks

Protection drift, dependabot, prod health read-only. Prod untouched unless a
rollout is operator-approved (it now carries D-082..D-086).

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 unexpected
  SKIP — the 3 env-gated infra skips (npx ×2, poppler) are pre-existing and
  characterized in D-085/D-086, coverage ≥ floor 70.2 (S24 actual 75.5 —
  integration-covered store code dilutes the unit number; that is
  characterized, not a regression), gofmt-on-emptiness, vet, contract-drift
  clean).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- Never trust a tree a dead/stalled workflow agent left (D-082/D-086) —
  re-derive RED proofs in a pristine copy. A pin that can t.Skip is not a
  pin (D-086).
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-087** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-26; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-26.md`.
