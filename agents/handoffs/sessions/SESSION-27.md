# SESSION-27 — next backlog batch (F10 tail / §2.17 candidates) + CI promotions window (ROADMAP-V2, planned at S26 close)

> Written by SESSION-26 close (D-088, 2026-07-13). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` §2/§3 + `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (incl. NEW §2.17) + the
final-assessment §5 roadmap and REVISE this plan if a higher-leverage move
exists. This file is a starting point, not a contract — "we always find a
spot to shine." (S26's example: the backlog review surfaced that §2.4 was
already delivered and never marked, and the scouts widened the FleetNodes
fix from one missing arm to a three-copy predicate unification.) Record any
revision in the D-089 open block. Carry this header into SESSION-28.md.

## ★ REVISED AT OPEN (2026-07-13, D-089) — OPERATOR DIRECTIVE SUPERSEDES THE MISSION BELOW

Operator (this session's prompt): **"rollout quick — i want app to be ready
for marketplace asap … installation easy and ready for uploading to the
marketplace with trial license key. i will provide ams license again today."**

Revised exit criteria (the original WO-A candidates are DEFERRED → S28+):
- (R1) **Prod rollout of D-082..D-088 executed** (the standing offer,
  triggered by "rollout quick"): runbook path, `pre-d089` rollback tag,
  fresh backup, smoke. **DONE = prod healthz ok on the new build.**
- (R2) **Trial-license lifecycle honest end-to-end:** expiry at runtime
  degrades to free-tier entitlements gracefully (never a dead product),
  surfaced in API/UI; mint→install→expire flow documented; tested with
  dev-key-signed short-expiry licenses; mutation-pinned.
- (R3) **Installation easy:** one-command install path (quickstart with
  trial key slot), install.md brought current, clean-install verified.
- (R4) **Marketplace package:** checklist rows 4/16/17 (integration docs /
  AMS compat disclosure / known limitations) brought from PARTIAL toward
  PASS; row 10 listing copy DRAFTED (stays internal — final-assessment
  review still gates anything external).
- (R5) Operator-action ledger recorded (D-089 + operator-expected.md ⚡):
  AMS license (promised today), official trial-key mint (vault privkey),
  final-assessment review (now gates upload), Ant Media contact.
- Standing: (b) operator intake ✅ (the directive IS the intake), (c) CI
  promotions skip carry ×16 (07-13 < 07-23), (d) AMS observation at open.

## Mission (original, superseded by the block above)

Exit = (a) the highest-leverage backlog batch built — candidates:
**F10 tail [M]** (probe-stats UI surface + RTMP AMF0 connect phase —
§2.11 remainder, the largest unstarted non-gated item); **§2.17.1
viewer_count zero-mean product ruling [S]** (0-viewer streams have
mean=0/stddev=0 baselines n>800 live; first viewer ⇒ z≫4 flag — decide
"audience appeared" signal vs noise, write the ruling, implement if
noise); **§2.17.2 parity-test map-derivation [XS]**; **§2.17.3
FleetNodes status="down" reachability [XS–S, may need contract CR]**;
**§2.5 O(N²) rebuildSnapshot [M]** (profiling + algorithmic fix — becomes
relevant for marketplace scale claims); (b) operator intake applied or
re-surfaced; (c) CI promotions if run date ≥ 2026-07-23 (else skip carry
×16 — csp-e2e candidate opens 07-23, web-e2e ~07-25); (d) standing
re-checks + AMS observation at open. PR-first, ≤2 pushes.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` — standing decisions** (caddy-vhost merge;
   final-assessment DRAFT review; optional prod rollout — now carries
   D-082..D-088: BUG-001..011 all fixed + recording billing + anomaly
   history + early-warning ladder + degraded-display/zero-mean-guard).
   If answered → act; else re-surface. Do NOT revert
   `deploy/config/Caddyfile.prod` on disk.
2. **Concurrent-session hazard (D-062, recurrent).** Foreign work → inspect
   → preserve on a branch → reset; never revert working files.
3. **AMS post-expiry state.** FIVE byte-identical sweeps post-lapse; still
   no antmedia restart (StartedAt 2026-07-12T06:52Z). At open:
   `bash qa/realams/harness/expiry-sweep.sh s27open` + diff vs
   `S21-sweep-preexpiry-20260712T014135Z/stable.txt`; check
   `sg docker -c 'docker inspect antmedia --format "{{.State.StartedAt}}"'`.
   If restarted AND features 403: observe + report, never restart/fix AMS.
   **⚠ TOKEN GOTCHA:** the S26 realams rebuild reset container logs — the
   harness env.sh `plt_` log-extraction WILL FAIL (orphaned-auth state,
   memory `realams-token-log-extract`). The sweep's
   `pulse-realams.overview` line needs auth: either `down -v` the realams
   stack FIRST (sanctioned, realams ONLY — wipes the live S26 sweep-proof
   baselines) and rebuild fresh, or accept that one line failing and note
   it. Never `down -v` prod.
4. **pulse-realams runs the S26 build** (loopback :18090, rebuilt at S26
   close WITHOUT down -v — meta volume preserved so the boot sweep proof
   is live: `purged zero-mean baselines on startup count=3` at
   2026-07-13T11:37:40Z; post-boot zero-mean node rows = 0 while
   ams_api_latency_ms n kept growing).
5. **S26 merge evidence:** if the S26 PR merged post-push, append the
   PR/merge line to decisions.md D-088 (S27's PR carries it).

## WO-A — the chosen backlog batch (per the standing directive review)

Scope per candidate list above; TDD + mutation-verified per §8; scouts
first (CodeGraph before grep); single-writer file scopes; no agent ever
runs git restore/checkout in the shared tree.

## WO-B [S, gate ≥2026-07-23] — CI promotions

JOB-streak re-measure FIRST; if open → csp-e2e FULL-LIST PUT + GET-diff
proof; web-e2e earliest ~07-25; else skip carry ×16.

## WO-C [XS] — standing re-checks

Protection drift, dependabot, prod health read-only. Prod untouched unless
a rollout is operator-approved.

## Gates (ORCH, before any commit)

- Any Go change: FULL §8 (repo-root mount `-race`, 0 FAIL / 0 unexpected
  SKIP — 3 env-gated infra skips are characterized in D-085..D-088,
  coverage ≥ floor 70.2 (S26 actual 76.0), gofmt-on-emptiness, vet,
  contract-drift clean). Integration suite when store/query/api change
  (S26 ran it CI-faithful: CH 24.8 + postgres:16 service containers +
  /tmp/clickhouse mounted).
- Never trust a tree a dead/stalled agent left (D-082/D-086) — re-derive
  RED proofs in pristine worktrees. A pin that can t.Skip is not a pin
  (D-086). A replacement pin must have ITS red re-derived too (D-087 M4).
- Harness/bash edits: `bash -n` + shellcheck + memory
  `shell-harness-false-green-patterns` (BINDING).
- final-assessment stays DRAFT until the operator OKs it. Nothing external.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-089** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-28; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-28.md` (carry the standing directive header).
